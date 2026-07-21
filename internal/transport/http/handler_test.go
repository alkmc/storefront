package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alkmc/storefront/internal/auth"
	"github.com/alkmc/storefront/internal/auth/authtest"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

const (
	testMaxBodyBytes = 1 << 20 // 1 MiB
	testJWTSecret    = "test-secret"
)

func setupTest(t *testing.T) (http.Handler, *stubProcessor) {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	proc := new(stubProcessor{})

	h := NewHandler(proc, 2*time.Second, logger)
	return bodyLimit(testMaxBodyBytes)(NewMux(h, Auth(auth.NewVerifier(testJWTSecret)))), proc
}

func TestGetProductByID(t *testing.T) {
	mux, proc := setupTest(t)

	tests := []struct {
		name           string
		id             string
		setupMock      func()
		expectedStatus int
		expectedMsg    string // for error cases
	}{
		{
			name: "success",
			id:   uuid.Must(uuid.NewV7()).String(),
			setupMock: func() {
				proc.findByID = func(_ context.Context, id uuid.UUID) (domain.Product, error) {
					return domain.Product{ID: id, Name: "Car", Price: testMoney()}, nil
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "incorrect uuid",
			id:             "incorrect",
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "invalid UUID length: 9",
		},
		{
			name: "non-existing product",
			id:   uuid.Must(uuid.NewV7()).String(),
			setupMock: func() {
				proc.findByID = func(_ context.Context, _ uuid.UUID) (domain.Product, error) {
					return domain.Product{}, domain.ErrNotFound
				}
			},
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "product not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/products/"+tt.id, nil)
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				p := decodeJSON[productResponse](t, resp.Body)
				if p.ID != uuid.MustParse(tt.id) {
					t.Errorf("got id %v, want %v", p.ID, tt.id)
				}
				if p.Name != "Car" {
					t.Errorf("got name %v, want %v", p.Name, "Car")
				}
				if p.Price.MinorAmount != 123 || p.Price.Currency != domain.CurrencyPLN {
					t.Errorf("got price %+v, want 123 PLN", p.Price)
				}
				return
			}
			e := decodeJSON[messageResponse](t, resp.Body)
			if e.Message != tt.expectedMsg {
				t.Errorf("got msg %q, want %q", e.Message, tt.expectedMsg)
			}
		})
	}
}

func TestGetProducts(t *testing.T) {
	mux, proc := setupTest(t)

	cursorID := uuid.Must(uuid.NewV7())
	lastID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name           string
		url            string
		setupMock      func()
		expectedStatus int
		expectedMsg    string
		expectedNames  []string
		wantNextCursor string
	}{
		{
			name: "empty",
			setupMock: func() {
				proc.findAll = func(_ context.Context, _ uuid.NullUUID, _ int) (domain.ProductPage, error) {
					return domain.ProductPage{}, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{},
		},
		{
			name: "success with default pagination",
			setupMock: func() {
				proc.findAll = func(_ context.Context, cursor uuid.NullUUID, limit int) (domain.ProductPage, error) {
					if limit != 50 || cursor.Valid {
						t.Errorf("got limit=%d cursor=%v, want 50/first-page", limit, cursor)
					}
					return domain.ProductPage{Items: []domain.Product{{Name: "Car", Price: testMoney()}}}, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{"Car"},
		},
		{
			name: "explicit limit and cursor",
			url:  "/v1/products?limit=10&cursor=" + cursorID.String(),
			setupMock: func() {
				proc.findAll = func(_ context.Context, cursor uuid.NullUUID, limit int) (domain.ProductPage, error) {
					if limit != 10 || !cursor.Valid || cursor.UUID != cursorID {
						t.Errorf("got limit=%d cursor=%v, want 10/%s", limit, cursor, cursorID)
					}
					return domain.ProductPage{Items: []domain.Product{{Name: "Car", Price: testMoney()}}}, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{"Car"},
		},
		{
			name: "limit clamped to max",
			url:  "/v1/products?limit=500",
			setupMock: func() {
				proc.findAll = func(_ context.Context, _ uuid.NullUUID, limit int) (domain.ProductPage, error) {
					if limit != 200 {
						t.Errorf("got limit=%d, want 200", limit)
					}
					return domain.ProductPage{}, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{},
		},
		{
			name: "negative limit falls back to default",
			url:  "/v1/products?limit=-5",
			setupMock: func() {
				proc.findAll = func(_ context.Context, _ uuid.NullUUID, limit int) (domain.ProductPage, error) {
					if limit != 50 {
						t.Errorf("got limit=%d, want 50", limit)
					}
					return domain.ProductPage{}, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{},
		},
		{
			name: "more pages set next cursor",
			setupMock: func() {
				proc.findAll = func(_ context.Context, _ uuid.NullUUID, _ int) (domain.ProductPage, error) {
					return domain.ProductPage{
						Items:   []domain.Product{{ID: lastID, Name: "Car", Price: testMoney()}},
						HasMore: true,
					}, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{"Car"},
			wantNextCursor: lastID.String(),
		},
		{
			name:           "invalid limit",
			url:            "/v1/products?limit=abc",
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "invalid limit: \"abc\"",
		},
		{
			name:           "invalid cursor",
			url:            "/v1/products?cursor=xyz",
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "invalid cursor: \"xyz\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			url := tt.url
			if url == "" {
				url = "/v1/products"
			}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedMsg != "" {
				e := decodeJSON[messageResponse](t, resp.Body)
				if e.Message != tt.expectedMsg {
					t.Errorf("got msg %q, want %q", e.Message, tt.expectedMsg)
				}
				return
			}
			page := decodeJSON[productsPage](t, resp.Body)
			if len(page.Items) != len(tt.expectedNames) {
				t.Fatalf("got len %d, want %d", len(page.Items), len(tt.expectedNames))
			}
			for i, name := range tt.expectedNames {
				if page.Items[i].Name != name {
					t.Errorf("got name %q, want %q", page.Items[i].Name, name)
				}
			}
			if page.NextCursor != tt.wantNextCursor {
				t.Errorf("got nextCursor %q, want %q", page.NextCursor, tt.wantNextCursor)
			}
		})
	}
}

func TestAddProduct(t *testing.T) {
	mux, proc := setupTest(t)

	tests := []struct {
		name           string
		body           any
		setupMock      func()
		expectedStatus int
		expectedMsg    string
	}{
		{
			name: "success",
			body: addInput{Name: "Car", Stock: 7, Price: testMoneyInput(123)},
			setupMock: func() {
				proc.create = func(_ context.Context, p domain.Product) (domain.Product, error) {
					return p, nil
				}
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "extra field",
			body:           map[string]any{"name": "Car", "price": testMoneyJSON(123), "email": "a@a.com"},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "unknown field \"email\"",
		},
		{
			name: "client supplied id rejected",
			body: map[string]any{
				"id":    uuid.Must(uuid.NewV7()).String(),
				"name":  "Car",
				"price": testMoneyJSON(123),
			},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "unknown field \"id\"",
		},
		{
			name:           "negative price",
			body:           addInput{Name: "Car", Price: testMoneyInput(-1)},
			setupMock:      func() {},
			expectedStatus: http.StatusUnprocessableEntity,
			expectedMsg:    "the product price must be positive",
		},
		{
			name: "invalid currency",
			body: addInput{
				Name:  "Car",
				Price: moneyInput{MinorAmount: 123, Currency: domain.Currency("XXX")},
			},
			setupMock:      func() {},
			expectedStatus: http.StatusUnprocessableEntity,
			expectedMsg:    "the product currency is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			b, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/products", bytes.NewReader(b))
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusCreated {
				if p := decodeJSON[productResponse](t, resp.Body); p.Stock != 7 {
					t.Errorf("got stock %d, want 7", p.Stock)
				}
				return
			}
			if tt.expectedMsg != "" {
				e := decodeJSON[messageResponse](t, resp.Body)
				if e.Message != tt.expectedMsg {
					t.Errorf("got msg %q, want %q", e.Message, tt.expectedMsg)
				}
			}
		})
	}
}

func TestServiceUnavailable(t *testing.T) {
	mux, proc := setupTest(t)
	proc.findByID = func(_ context.Context, _ uuid.UUID) (domain.Product, error) {
		return domain.Product{}, fmt.Errorf("query failed: %w", domain.ErrUnavailable)
	}

	id := uuid.Must(uuid.NewV7()).String()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/products/"+id, nil)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Errorf("got status %d, want %d", resp.Code, http.StatusServiceUnavailable)
	}
	if e := decodeJSON[messageResponse](t, resp.Body); e.Message != msgUnavailable {
		t.Errorf("got msg %q, want %q", e.Message, msgUnavailable)
	}
}

func TestAddProductBodyTooLarge(t *testing.T) {
	const limit = 16 // bytes
	h := NewHandler(new(stubProcessor{}), 2*time.Second, slog.New(slog.DiscardHandler))
	mux := bodyLimit(limit)(NewMux(h, Auth(auth.NewVerifier(testJWTSecret))))

	body := []byte(`{"name":"a long enough name","price":{"minorAmount":100,"currency":"PLN"}}`)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/products", bytes.NewReader(body))
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("got status %d, want %d", resp.Code, http.StatusRequestEntityTooLarge)
	}
	e := decodeJSON[messageResponse](t, resp.Body)
	if !strings.Contains(e.Message, "request body too large") {
		t.Errorf("got msg %q, want it to contain %q", e.Message, "request body too large")
	}
}

func TestDeleteProduct(t *testing.T) {
	mux, proc := setupTest(t)

	tests := []struct {
		name           string
		id             string
		setupMock      func()
		expectedStatus int
		expectedMsg    string
	}{
		{
			name: "not existing",
			id:   uuid.Must(uuid.NewV7()).String(),
			setupMock: func() {
				proc.delete = func(_ context.Context, _ uuid.UUID) error {
					return domain.ErrNotFound
				}
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "success",
			id:   uuid.Must(uuid.NewV7()).String(),
			setupMock: func() {
				proc.delete = func(_ context.Context, _ uuid.UUID) error {
					return nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedMsg:    "product deleted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/v1/products/"+tt.id, nil)
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedMsg != "" {
				e := decodeJSON[messageResponse](t, resp.Body)
				if e.Message != tt.expectedMsg {
					t.Errorf("got msg %q, want %q", e.Message, tt.expectedMsg)
				}
			}
		})
	}
}

func TestUpdateProduct(t *testing.T) {
	mux, proc := setupTest(t)

	tests := []struct {
		name           string
		id             string
		body           any
		setupMock      func()
		expectedStatus int
		expectedName   string
	}{
		{
			name: "success",
			id:   uuid.Must(uuid.NewV7()).String(),
			body: updateInput{Name: "Updated", Price: testMoneyInput(9990)},
			setupMock: func() {
				proc.update = func(_ context.Context, p domain.Product) (domain.Product, error) {
					p.Stock = 7 // stock comes from the store, not the request body
					return p, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedName:   "Updated",
		},
		{
			name: "client supplied id rejected",
			id:   uuid.Must(uuid.NewV7()).String(),
			body: map[string]any{
				"id":    uuid.Must(uuid.NewV7()).String(),
				"name":  "Updated",
				"price": testMoneyJSON(9990),
			},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			b, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			req := httptest.NewRequestWithContext(
				t.Context(), http.MethodPut, "/v1/products/"+tt.id, bytes.NewReader(b),
			)
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				p := decodeJSON[productResponse](t, resp.Body)
				if p.Name != tt.expectedName {
					t.Errorf("got name %q, want %q", p.Name, tt.expectedName)
				}
				if p.Stock != 7 {
					t.Errorf("got stock %d, want 7 (from store, not request)", p.Stock)
				}
			}
		})
	}
}

func TestPurchaseProduct(t *testing.T) {
	mux, proc := setupTest(t)

	id := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())
	orderID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name           string
		quantity       int64
		setupMock      func()
		expectedStatus int
		expectedMsg    string
	}{
		{
			name:     "success",
			quantity: 2,
			setupMock: func() {
				proc.purchase = func(_ context.Context, gotUser domain.UserID, pid uuid.UUID, qty int64,
				) (domain.Product, domain.Order, error) {
					if gotUser != domain.UserID(userID) {
						t.Errorf("got user %v, want %v", gotUser, userID)
					}
					p := domain.Product{ID: pid, Name: "Car", Price: testMoney(), Stock: 5}
					o := domain.Order{ID: domain.OrderID(orderID), UserID: gotUser, ProductID: pid, Quantity: qty}
					return p, o, nil
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "quantity below min",
			quantity:       0,
			setupMock:      func() {},
			expectedStatus: http.StatusUnprocessableEntity,
			expectedMsg:    msgInvalidQuantity,
		},
		{
			name:           "quantity above max",
			quantity:       10001,
			setupMock:      func() {},
			expectedStatus: http.StatusUnprocessableEntity,
			expectedMsg:    msgInvalidQuantity,
		},
		{
			name:     "product not found",
			quantity: 2,
			setupMock: func() {
				proc.purchase = func(_ context.Context, _ domain.UserID, _ uuid.UUID, _ int64,
				) (domain.Product, domain.Order, error) {
					return domain.Product{}, domain.Order{}, domain.ErrNotFound
				}
			},
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "product not found",
		},
		{
			name:     "insufficient stock",
			quantity: 2,
			setupMock: func() {
				proc.purchase = func(_ context.Context, _ domain.UserID, _ uuid.UUID, _ int64,
				) (domain.Product, domain.Order, error) {
					return domain.Product{}, domain.Order{}, domain.ErrInsufficientStock
				}
			},
			expectedStatus: http.StatusConflict,
			expectedMsg:    "insufficient stock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			b, err := json.Marshal(purchaseInput{Quantity: tt.quantity})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			req := httptest.NewRequestWithContext(
				t.Context(), http.MethodPost, "/v1/products/"+id.String()+"/purchase", bytes.NewReader(b),
			)
			req.Header.Set("Authorization", bearer(t, userID))
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				pr := decodeJSON[purchaseResponse](t, resp.Body)
				if pr.ProductID != id || pr.Quantity != 2 || pr.RemainingStock != 5 {
					t.Errorf("got %+v, want productId=%v quantity=2 remainingStock=5", pr, id)
				}
				if pr.OrderID != orderID {
					t.Errorf("got orderId %v, want %v", pr.OrderID, orderID)
				}
				return
			}
			e := decodeJSON[messageResponse](t, resp.Body)
			if e.Message != tt.expectedMsg {
				t.Errorf("got msg %q, want %q", e.Message, tt.expectedMsg)
			}
		})
	}
}

func TestProtectedRoutesRequireToken(t *testing.T) {
	mux, _ := setupTest(t)

	tests := []struct {
		name   string
		method string
		url    string
		body   string
	}{
		{
			name:   "purchase",
			method: http.MethodPost,
			url:    "/v1/products/" + uuid.NewString() + "/purchase",
			body:   `{"quantity":1}`,
		},
		{
			name:   "list orders",
			method: http.MethodGet,
			url:    "/v1/orders",
		},
		{
			name:   "get order",
			method: http.MethodGet,
			url:    "/v1/orders/" + uuid.NewString(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(
				t.Context(), tt.method, tt.url, strings.NewReader(tt.body),
			)
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != http.StatusUnauthorized {
				t.Errorf("got status %d, want %d", resp.Code, http.StatusUnauthorized)
			}
			if got := resp.Header().Get("WWW-Authenticate"); got != "Bearer" {
				t.Errorf("got WWW-Authenticate %q, want %q", got, "Bearer")
			}
		})
	}
}

func TestGetOrder(t *testing.T) {
	mux, proc := setupTest(t)

	userID := uuid.Must(uuid.NewV7())
	orderID := uuid.Must(uuid.NewV7())
	productID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name           string
		id             string
		setupMock      func()
		expectedStatus int
		expectedMsg    string
	}{
		{
			name: "success",
			id:   orderID.String(),
			setupMock: func() {
				proc.findOrder = func(_ context.Context, gotUser domain.UserID, id domain.OrderID,
				) (domain.Order, error) {
					if gotUser != domain.UserID(userID) {
						t.Errorf("got user %v, want %v", gotUser, userID)
					}
					return domain.Order{
						ID: id, UserID: gotUser, ProductID: productID, Quantity: 2, UnitPrice: testMoney(),
					}, nil
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "foreign or missing order",
			id:   orderID.String(),
			setupMock: func() {
				proc.findOrder = func(_ context.Context, _ domain.UserID, _ domain.OrderID) (domain.Order, error) {
					return domain.Order{}, domain.ErrNotFound
				}
			},
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "order not found",
		},
		{
			name:           "invalid uuid",
			id:             "nope",
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/orders/"+tt.id, nil)
			req.Header.Set("Authorization", bearer(t, userID))
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				o := decodeJSON[orderResponse](t, resp.Body)
				if o.ID != orderID || o.ProductID != productID || o.Quantity != 2 {
					t.Errorf("got %+v, want id=%v productId=%v quantity=2", o, orderID, productID)
				}
				return
			}
			if tt.expectedMsg != "" {
				e := decodeJSON[messageResponse](t, resp.Body)
				if e.Message != tt.expectedMsg {
					t.Errorf("got msg %q, want %q", e.Message, tt.expectedMsg)
				}
			}
		})
	}
}

func TestListOrders(t *testing.T) {
	mux, proc := setupTest(t)

	userID := uuid.Must(uuid.NewV7())
	cursorID := uuid.Must(uuid.NewV7())
	lastID := uuid.Must(uuid.NewV7())

	proc.findOrders = func(_ context.Context, gotUser domain.UserID, cursor uuid.NullUUID, limit int,
	) (domain.OrderPage, error) {
		if gotUser != domain.UserID(userID) {
			t.Errorf("got user %v, want %v", gotUser, userID)
		}
		if limit != 10 || !cursor.Valid || cursor.UUID != cursorID {
			t.Errorf("got limit %d cursor %v, want 10 %s", limit, cursor, cursorID)
		}
		return domain.OrderPage{
			Items: []domain.Order{
				{
					ID:        domain.OrderID(lastID),
					UserID:    gotUser,
					ProductID: uuid.Must(uuid.NewV7()),
					Quantity:  1,
					UnitPrice: testMoney(),
				},
			},
			HasMore: true,
		}, nil
	}

	url := "/v1/orders?limit=10&cursor=" + cursorID.String()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	req.Header.Set("Authorization", bearer(t, userID))
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", resp.Code, http.StatusOK)
	}
	page := decodeJSON[ordersPage](t, resp.Body)
	if len(page.Items) != 1 || page.Items[0].ID != lastID {
		t.Errorf("got items %+v, want single order %v", page.Items, lastID)
	}
	if page.NextCursor != lastID.String() {
		t.Errorf("got nextCursor %q, want %q", page.NextCursor, lastID.String())
	}
}

func bearer(t *testing.T, sub uuid.UUID) string {
	t.Helper()
	return "Bearer " + authtest.Token(testJWTSecret, sub, time.Now().Add(time.Hour))
}

func testMoney() domain.Money {
	return domain.Money{MinorAmount: 123, Currency: domain.CurrencyPLN}
}

func testMoneyInput(amount int64) moneyInput {
	return moneyInput{MinorAmount: amount, Currency: domain.CurrencyPLN}
}

func testMoneyJSON(amount int64) map[string]any {
	return map[string]any{"minorAmount": amount, "currency": string(domain.CurrencyPLN)}
}

func decodeJSON[T any](t *testing.T, r io.Reader) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(r).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}
