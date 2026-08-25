//go:build integration

package http

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/alkmc/storefront/internal/auth"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/alkmc/storefront/internal/pg/pgtest"
	"github.com/alkmc/storefront/internal/service"
	"github.com/alkmc/storefront/internal/store"
	"github.com/google/go-cmp/cmp"
)

// TestIntegration_OrderIdempotency drives the full request path against real Postgres: the same key
// replays the stored result without a second decrement, a different payload on that key is a mismatch.
func TestIntegration_OrderIdempotency(t *testing.T) {
	mux := setupIntegrationMux(t)
	user := uuid.NewV7()
	productID := createProduct(t, mux, 5, 999)
	key := uuid.NewV7().String()

	// first call runs the purchase and carries no replay marker
	first := postOrder(t, mux, user, productID, 2, key)
	if first.Code != http.StatusCreated {
		t.Fatalf("first: got %d: %s", first.Code, first.Body)
	}
	if got := first.Header().Get(headerIdempotencyReplayed); got != "" {
		t.Errorf("first call set %s = %q, want unset", headerIdempotencyReplayed, got)
	}
	firstOrder := decodeJSON[createOrderResponse](t, first.Body)

	// same key and body replays the stored result verbatim
	replay := postOrder(t, mux, user, productID, 2, key)
	if replay.Code != http.StatusCreated {
		t.Fatalf("replay: got %d: %s", replay.Code, replay.Body)
	}
	if got := replay.Header().Get(headerIdempotencyReplayed); got != "true" {
		t.Errorf("replay %s = %q, want %q", headerIdempotencyReplayed, got, "true")
	}
	replayOrder := decodeJSON[createOrderResponse](t, replay.Body)
	if diff := cmp.Diff(firstOrder, replayOrder); diff != "" {
		t.Errorf("replay diverged (-first +replay):\n%s", diff)
	}

	// the replay decremented nothing: exactly one order exists end to end
	page := doGet[ordersPage](t, mux, user, "/v1/orders", http.StatusOK)
	if len(page.Items) != 1 {
		t.Errorf("got %d orders, want 1 (single decrement)", len(page.Items))
	}

	// same key with a different payload is rejected as a mismatch
	mismatch := postOrder(t, mux, user, productID, 3, key)
	if mismatch.Code != http.StatusUnprocessableEntity {
		t.Fatalf("mismatch: got %d, want %d: %s", mismatch.Code, http.StatusUnprocessableEntity, mismatch.Body)
	}
	if msg := decodeJSON[messageResponse](t, mismatch.Body); msg.Message != msgIdempotencyMismatch {
		t.Errorf("mismatch msg %q, want %q", msg.Message, msgIdempotencyMismatch)
	}
}

func TestIntegration_PurchaseCreatesOwnedOrder(t *testing.T) {
	mux := setupIntegrationMux(t)
	userA := uuid.NewV7()
	userB := uuid.NewV7()

	productID := createProduct(t, mux, 5, 999)

	pr := doPurchase(t, mux, userA, productID, 2)
	if pr.ID == uuid.Nil() {
		t.Fatal("create order response has no id")
	}

	// the owner reads the order back with the price snapshot
	o := doGet[orderResponse](t, mux, userA, "/v1/orders/"+pr.ID.String(), http.StatusOK)
	if o.ProductID != productID || o.Quantity != 2 {
		t.Errorf("got %+v, want productId %v quantity 2", o, productID)
	}
	if o.UnitPrice.MinorAmount != 999 || o.UnitPrice.Currency != domain.CurrencyPLN {
		t.Errorf("got unit price %+v, want 999 PLN", o.UnitPrice)
	}
	if o.CreatedAt.IsZero() {
		t.Error("order createdAt is zero")
	}

	// a foreign order is a 404, not a 403, so its existence stays hidden
	msg := doGet[messageResponse](t, mux, userB, "/v1/orders/"+pr.ID.String(), http.StatusNotFound)
	if msg.Message != "order not found" {
		t.Errorf("got msg %q, want %q", msg.Message, "order not found")
	}
}

func TestIntegration_ListOrdersIsOwnerScoped(t *testing.T) {
	mux := setupIntegrationMux(t)
	userA := uuid.NewV7()
	userB := uuid.NewV7()

	productID := createProduct(t, mux, 10, 500)

	aOrders := make([]uuid.UUID, 0, 3)
	for range 3 {
		aOrders = append(aOrders, doPurchase(t, mux, userA, productID, 1).ID)
	}
	bOrder := doPurchase(t, mux, userB, productID, 1).ID

	// first page: newest first, only A's orders
	first := doGet[ordersPage](t, mux, userA, "/v1/orders?limit=2", http.StatusOK)
	if len(first.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(first.Items))
	}
	if first.Items[0].ID != aOrders[2] || first.Items[1].ID != aOrders[1] {
		t.Errorf("expected newest first, got %+v", first.Items)
	}
	if first.NextCursor == "" {
		t.Fatal("expected nextCursor on a full page")
	}

	// second page ends the stream with the oldest order and no foreign rows
	second := doGet[ordersPage](
		t, mux, userA, "/v1/orders?limit=2&cursor="+first.NextCursor, http.StatusOK,
	)
	if len(second.Items) != 1 || second.Items[0].ID != aOrders[0] {
		t.Fatalf("got %+v, want the oldest order %v", second.Items, aOrders[0])
	}
	if second.NextCursor != "" {
		t.Errorf("got nextCursor %q on the last page", second.NextCursor)
	}
	for _, o := range append(first.Items, second.Items...) {
		if o.ID == bOrder {
			t.Errorf("foreign order leaked into listing: %v", o.ID)
		}
	}

	// B sees only their own single order
	bPage := doGet[ordersPage](t, mux, userB, "/v1/orders", http.StatusOK)
	if len(bPage.Items) != 1 || bPage.Items[0].ID != bOrder {
		t.Errorf("got %+v, want only %v", bPage.Items, bOrder)
	}
}

func TestIntegration_DeleteBlockedByOrders(t *testing.T) {
	mux := setupIntegrationMux(t)
	user := uuid.NewV7()

	soldID := createProduct(t, mux, 5, 100)
	freshID := createProduct(t, mux, 5, 100)
	doPurchase(t, mux, user, soldID, 1)

	del := func(id uuid.UUID) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(
			t.Context(), http.MethodDelete, "/v1/products/"+id.String(), nil,
		)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	// a purchased product must not be deletable, the FK and its mapping do the work end to end
	rec := del(soldID)
	if rec.Code != http.StatusConflict {
		t.Fatalf("got %d, want %d: %s", rec.Code, http.StatusConflict, rec.Body)
	}
	if msg := decodeJSON[messageResponse](t, rec.Body); msg.Message != "product has existing orders" {
		t.Errorf("got msg %q, want %q", msg.Message, "product has existing orders")
	}

	// a product without orders still deletes as before
	if rec := del(freshID); rec.Code != http.StatusOK {
		t.Errorf("got %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}
}

func TestIntegration_PurchaseRequiresToken(t *testing.T) {
	mux := setupIntegrationMux(t)
	productID := createProduct(t, mux, 5, 100)

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/v1/orders",
		strings.NewReader(fmt.Sprintf(`{"productId":%q,"quantity":1}`, productID.String())),
	)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("got WWW-Authenticate %q, want %q", got, "Bearer")
	}
}

// setupIntegrationMux boots PG in a container and wires the real service into the real mux.
func setupIntegrationMux(t *testing.T) http.Handler {
	t.Helper()

	pool := pgtest.MigratedPool(t)

	logger := slog.New(slog.DiscardHandler)
	svc := service.NewService(store.NewPostgres(pool, time.Hour), nopCache{}, time.Second, logger)
	h := NewHandler(svc, 2*time.Second, logger)
	mw, err := NewMiddleware(MiddlewareCfg{
		MaxBodyBytes:     testMaxBodyBytes,
		CompressMinBytes: 1024,
	})
	if err != nil {
		t.Fatalf("failed to build middleware: %v", err)
	}
	return mw(NewMux(h, Auth(auth.NewVerifier(testJWTSecret))))
}

func createProduct(t *testing.T, mux http.Handler, stock, price int64) uuid.UUID {
	t.Helper()
	body := fmt.Sprintf(`{"name":"Widget","stock":%d,"price":{"minorAmount":%d,"currency":"PLN"}}`,
		stock, price)
	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/v1/products", strings.NewReader(body),
	)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create product: got %d: %s", rec.Code, rec.Body)
	}
	return decodeJSON[productResponse](t, rec.Body).ID
}

func doPurchase(t *testing.T, mux http.Handler, sub, productID uuid.UUID, qty int64) createOrderResponse {
	t.Helper()
	body := fmt.Sprintf(`{"productId":%q,"quantity":%d}`, productID.String(), qty)
	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/v1/orders", strings.NewReader(body),
	)
	req.Header.Set("Authorization", bearer(t, sub))
	req.Header.Set(headerIdempotencyKey, uuid.NewV7().String())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create order: got %d: %s", rec.Code, rec.Body)
	}
	return decodeJSON[createOrderResponse](t, rec.Body)
}

func doGet[T any](t *testing.T, mux http.Handler, sub uuid.UUID, url string, wantStatus int) T {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	req.Header.Set("Authorization", bearer(t, sub))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("GET %s: got %d, want %d: %s", url, rec.Code, wantStatus, rec.Body)
	}
	return decodeJSON[T](t, rec.Body)
}

// postOrder issues a create-order request with a caller-chosen Idempotency-Key (empty omits the
// header) and returns the raw recorder so the test can inspect status, headers, and body.
func postOrder(
	t *testing.T, mux http.Handler, sub, productID uuid.UUID, qty int64, key string,
) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"productId":%q,"quantity":%d}`, productID.String(), qty)
	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/v1/orders", strings.NewReader(body),
	)
	req.Header.Set("Authorization", bearer(t, sub))
	if key != "" {
		req.Header.Set(headerIdempotencyKey, key)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}
