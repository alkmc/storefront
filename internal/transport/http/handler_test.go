package http

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/alkmc/storefront/internal/auth"
	"github.com/alkmc/storefront/internal/auth/authtest"
	"github.com/alkmc/storefront/internal/domain"
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
			id:   uuid.NewV7().String(),
			setupMock: func() {
				proc.findByID = func(_ context.Context, id domain.ProductID) (domain.Product, error) {
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
			expectedMsg:    "invalid uuid",
		},
		{
			name: "non-existing product",
			id:   uuid.NewV7().String(),
			setupMock: func() {
				proc.findByID = func(_ context.Context, _ domain.ProductID) (domain.Product, error) {
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

	cursorID := uuid.NewV7()
	lastID := uuid.NewV7()

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
				proc.findAll = func(_ context.Context, _ domain.Cursor, _ int) (domain.ProductPage, error) {
					return domain.ProductPage{}, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{},
		},
		{
			name: "success with default pagination",
			setupMock: func() {
				proc.findAll = func(_ context.Context, cursor domain.Cursor, limit int) (domain.ProductPage, error) {
					if _, ok := cursor.After(); limit != 50 || ok {
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
				proc.findAll = func(_ context.Context, cursor domain.Cursor, limit int) (domain.ProductPage, error) {
					if id, ok := cursor.After(); limit != 10 || !ok || id != cursorID {
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
				proc.findAll = func(_ context.Context, _ domain.Cursor, limit int) (domain.ProductPage, error) {
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
				proc.findAll = func(_ context.Context, _ domain.Cursor, limit int) (domain.ProductPage, error) {
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
				proc.findAll = func(_ context.Context, _ domain.Cursor, _ int) (domain.ProductPage, error) {
					return domain.ProductPage{
						Items:   []domain.Product{{ID: domain.ProductID(lastID), Name: "Car", Price: testMoney()}},
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
				"id":    uuid.NewV7().String(),
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
	proc.findByID = func(_ context.Context, _ domain.ProductID) (domain.Product, error) {
		return domain.Product{}, fmt.Errorf("query failed: %w", domain.ErrUnavailable)
	}

	id := uuid.NewV7().String()
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
			id:   uuid.NewV7().String(),
			setupMock: func() {
				proc.delete = func(_ context.Context, _ domain.ProductID) error {
					return domain.ErrNotFound
				}
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "success",
			id:   uuid.NewV7().String(),
			setupMock: func() {
				proc.delete = func(_ context.Context, _ domain.ProductID) error {
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
			id:   uuid.NewV7().String(),
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
			id:   uuid.NewV7().String(),
			body: map[string]any{
				"id":    uuid.NewV7().String(),
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

func TestCreateOrder(t *testing.T) {
	mux, proc := setupTest(t)

	id := uuid.NewV7()
	userID := uuid.NewV7()
	orderID := uuid.NewV7()

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
				proc.createOrder = func(
					_ context.Context, gotUser domain.UserID, pid domain.ProductID, qty int64, _ domain.IdempotencyKey,
				) (domain.Order, bool, error) {
					if gotUser != domain.UserID(userID) {
						t.Errorf("got user %v, want %v", gotUser, userID)
					}
					o := domain.Order{ID: domain.OrderID(orderID), UserID: gotUser, ProductID: pid, Quantity: qty}
					return o, false, nil
				}
			},
			expectedStatus: http.StatusCreated,
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
				proc.createOrder = func(
					_ context.Context, _ domain.UserID, _ domain.ProductID, _ int64, _ domain.IdempotencyKey,
				) (domain.Order, bool, error) {
					return domain.Order{}, false, domain.ErrNotFound
				}
			},
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "product not found",
		},
		{
			name:     "insufficient stock",
			quantity: 2,
			setupMock: func() {
				proc.createOrder = func(
					_ context.Context, _ domain.UserID, _ domain.ProductID, _ int64, _ domain.IdempotencyKey,
				) (domain.Order, bool, error) {
					return domain.Order{}, false, domain.ErrInsufficientStock
				}
			},
			expectedStatus: http.StatusConflict,
			expectedMsg:    "insufficient stock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			b, err := json.Marshal(createOrderInput{ProductID: id.String(), Quantity: tt.quantity})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			req := httptest.NewRequestWithContext(
				t.Context(), http.MethodPost, "/v1/orders", bytes.NewReader(b),
			)
			req.Header.Set("Authorization", bearer(t, userID))
			req.Header.Set(headerIdempotencyKey, uuid.NewV7().String())
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusCreated {
				pr := decodeJSON[createOrderResponse](t, resp.Body)
				if pr.ProductID != id || pr.Quantity != 2 {
					t.Errorf("got %+v, want productId=%v quantity=2", pr, id)
				}
				if pr.ID != orderID {
					t.Errorf("got id %v, want %v", pr.ID, orderID)
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

func TestCreateOrder_Idempotency(t *testing.T) {
	id := uuid.NewV7()
	userID := uuid.NewV7()
	orderID := uuid.NewV7()
	idemKey := uuid.NewV7().String()

	send := func(t *testing.T, mux http.Handler, key string, qty int64) *httptest.ResponseRecorder {
		t.Helper()
		b, err := json.Marshal(createOrderInput{ProductID: id.String(), Quantity: qty})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/orders", bytes.NewReader(b))
		req.Header.Set("Authorization", bearer(t, userID))
		if key != "" {
			req.Header.Set(headerIdempotencyKey, key)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	t.Run("replay sets the replayed header", func(t *testing.T) {
		mux, proc := setupTest(t)
		proc.createOrder = func(
			_ context.Context, u domain.UserID, pid domain.ProductID, qty int64, idem domain.IdempotencyKey,
		) (domain.Order, bool, error) {
			if idem == "" {
				t.Error("idempotency key not propagated to processor")
			}
			o := domain.Order{ID: domain.OrderID(orderID), UserID: u, ProductID: pid, Quantity: qty}
			return o, true, nil
		}
		rec := send(t, mux, idemKey, 2)
		if rec.Code != http.StatusCreated {
			t.Fatalf("got %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
		}
		if got := rec.Header().Get(headerIdempotencyReplayed); got != "true" {
			t.Errorf("got %s %q, want %q", headerIdempotencyReplayed, got, "true")
		}
	})

	t.Run("mismatch is 422", func(t *testing.T) {
		mux, proc := setupTest(t)
		proc.createOrder = func(
			context.Context, domain.UserID, domain.ProductID, int64, domain.IdempotencyKey,
		) (domain.Order, bool, error) {
			return domain.Order{}, false, domain.ErrIdempotencyMismatch
		}
		rec := send(t, mux, idemKey, 2)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("got %d, want %d", rec.Code, http.StatusUnprocessableEntity)
		}
		if msg := decodeJSON[messageResponse](t, rec.Body); msg.Message != msgIdempotencyMismatch {
			t.Errorf("got msg %q, want %q", msg.Message, msgIdempotencyMismatch)
		}
	})

	t.Run("missing key is 400 and never calls the processor", func(t *testing.T) {
		mux, proc := setupTest(t)
		called := false
		proc.createOrder = func(
			context.Context, domain.UserID, domain.ProductID, int64, domain.IdempotencyKey,
		) (domain.Order, bool, error) {
			called = true
			return domain.Order{}, false, nil
		}
		rec := send(t, mux, "", 2)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want %d", rec.Code, http.StatusBadRequest)
		}
		if msg := decodeJSON[messageResponse](t, rec.Body); msg.Message != msgIdempotencyRequired {
			t.Errorf("got msg %q, want %q", msg.Message, msgIdempotencyRequired)
		}
		if called {
			t.Error("processor called despite missing key")
		}
	})

	t.Run("over-long key is 400 and never calls the processor", func(t *testing.T) {
		mux, proc := setupTest(t)
		called := false
		proc.createOrder = func(
			context.Context, domain.UserID, domain.ProductID, int64, domain.IdempotencyKey,
		) (domain.Order, bool, error) {
			called = true
			return domain.Order{}, false, nil
		}
		rec := send(t, mux, strings.Repeat("k", domain.MaxIdempotencyKeyLen+1), 2)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want %d", rec.Code, http.StatusBadRequest)
		}
		if called {
			t.Error("processor called despite over-long key")
		}
	})
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
			name:   "create order",
			method: http.MethodPost,
			url:    "/v1/orders",
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
			url:    "/v1/orders/" + uuid.NewV7().String(),
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

	userID := uuid.NewV7()
	orderID := uuid.NewV7()
	productID := uuid.NewV7()

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
						ID: id, UserID: gotUser, ProductID: domain.ProductID(productID), Quantity: 2, UnitPrice: testMoney(),
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

	userID := uuid.NewV7()
	cursorID := uuid.NewV7()
	lastID := uuid.NewV7()

	proc.findOrders = func(_ context.Context, gotUser domain.UserID, cursor domain.Cursor, limit int,
	) (domain.OrderPage, error) {
		if gotUser != domain.UserID(userID) {
			t.Errorf("got user %v, want %v", gotUser, userID)
		}
		if id, ok := cursor.After(); limit != 10 || !ok || id != cursorID {
			t.Errorf("got limit %d cursor %v, want 10 %s", limit, cursor, cursorID)
		}
		return domain.OrderPage{
			Items: []domain.Order{
				{
					ID:        domain.OrderID(lastID),
					UserID:    gotUser,
					ProductID: domain.ProductID(uuid.NewV7()),
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
	if err := json.UnmarshalRead(r, &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}
