package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alkmc/restClean/internal/cache"
	"github.com/alkmc/restClean/internal/config"
	"github.com/alkmc/restClean/internal/entity"
	"github.com/alkmc/restClean/internal/service"
	"github.com/google/uuid"
)

var testHTTPConfig = config.HTTP{
	MaxBodyBytes:     1 << 20, // 1 MiB
	CompressMinBytes: 1024,
}

func decodeJSON[T any](t *testing.T, r io.Reader) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(r).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

type mockRepo struct {
	save     func(ctx context.Context, p entity.Product) (entity.Product, error)
	findByID func(ctx context.Context, id uuid.UUID) (entity.Product, error)
	findAll  func(ctx context.Context, limit, offset int) ([]entity.Product, error)
	update   func(ctx context.Context, p entity.Product) error
	delete   func(ctx context.Context, id uuid.UUID) error
}

func (m *mockRepo) Save(ctx context.Context, p entity.Product) (entity.Product, error) {
	return m.save(ctx, p)
}

func (m *mockRepo) FindByID(ctx context.Context, id uuid.UUID) (entity.Product, error) {
	if m.findByID == nil {
		return entity.Product{}, entity.ErrNotFound
	}
	return m.findByID(ctx, id)
}

func (m *mockRepo) FindAll(ctx context.Context, limit, offset int) ([]entity.Product, error) {
	return m.findAll(ctx, limit, offset)
}

func (m *mockRepo) Update(ctx context.Context, p entity.Product) error {
	return m.update(ctx, p)
}

func (m *mockRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return m.delete(ctx, id)
}

type mockCache struct{}

func (m *mockCache) Set(_ context.Context, _ string, _ entity.Product) error {
	return nil
}

func (m *mockCache) Get(_ context.Context, _ string) (entity.Product, error) {
	return entity.Product{}, cache.ErrCacheMiss
}

func (m *mockCache) Invalidate(_ context.Context, _ string) error {
	return nil
}

func setupTest(t *testing.T, cfg config.HTTP) (http.Handler, *mockRepo) {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	repo := new(mockRepo{})

	srv := service.NewService(logger, repo, &mockCache{})
	h := NewHandler(logger, srv, 2*time.Second)
	return bodyLimit(cfg.MaxBodyBytes)(NewMux(h)), repo
}

func TestGetProductByID(t *testing.T) {
	mux, repo := setupTest(t, testHTTPConfig)

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
				repo.findByID = func(_ context.Context, id uuid.UUID) (entity.Product, error) {
					return entity.Product{ID: id, Name: "Car", Price: 1.23}, nil
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
				repo.findByID = func(_ context.Context, _ uuid.UUID) (entity.Product, error) {
					return entity.Product{}, entity.ErrNotFound
				}
			},
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "product not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/product/"+tt.id, nil)
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				p := decodeJSON[entity.Product](t, resp.Body)
				if p.ID != uuid.MustParse(tt.id) {
					t.Errorf("got id %v, want %v", p.ID, tt.id)
				}
				if p.Name != "Car" {
					t.Errorf("got name %v, want %v", p.Name, "Car")
				}
			} else {
				e := decodeJSON[messageResponse](t, resp.Body)
				if e.Message != tt.expectedMsg {
					t.Errorf("got msg %q, want %q", e.Message, tt.expectedMsg)
				}
			}
		})
	}
}

func TestGetProducts(t *testing.T) {
	mux, repo := setupTest(t, testHTTPConfig)

	tests := []struct {
		name           string
		url            string
		setupMock      func()
		expectedStatus int
		expectedMsg    string
		expectedNames  []string
	}{
		{
			name: "empty",
			setupMock: func() {
				repo.findAll = func(_ context.Context, _, _ int) ([]entity.Product, error) {
					return nil, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{},
		},
		{
			name: "success with default pagination",
			setupMock: func() {
				repo.findAll = func(_ context.Context, limit, offset int) ([]entity.Product, error) {
					if limit != 50 || offset != 0 {
						t.Errorf("got limit=%d offset=%d, want 50/0", limit, offset)
					}
					return []entity.Product{{Name: "Car", Price: 1.23}}, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{"Car"},
		},
		{
			name: "explicit limit and offset",
			url:  "/product?limit=10&offset=5",
			setupMock: func() {
				repo.findAll = func(_ context.Context, limit, offset int) ([]entity.Product, error) {
					if limit != 10 || offset != 5 {
						t.Errorf("got limit=%d offset=%d, want 10/5", limit, offset)
					}
					return []entity.Product{{Name: "Car", Price: 1.23}}, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{"Car"},
		},
		{
			name: "limit clamped to max",
			url:  "/product?limit=500",
			setupMock: func() {
				repo.findAll = func(_ context.Context, limit, _ int) ([]entity.Product, error) {
					if limit != 200 {
						t.Errorf("got limit=%d, want 200", limit)
					}
					return nil, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{},
		},
		{
			name: "negative limit falls back to default",
			url:  "/product?limit=-5",
			setupMock: func() {
				repo.findAll = func(_ context.Context, limit, _ int) ([]entity.Product, error) {
					if limit != 50 {
						t.Errorf("got limit=%d, want 50", limit)
					}
					return nil, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{},
		},
		{
			name: "negative offset clamped to zero",
			url:  "/product?offset=-1",
			setupMock: func() {
				repo.findAll = func(_ context.Context, _, offset int) ([]entity.Product, error) {
					if offset != 0 {
						t.Errorf("got offset=%d, want 0", offset)
					}
					return nil, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{},
		},
		{
			name:           "invalid limit",
			url:            "/product?limit=abc",
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "invalid limit: \"abc\"",
		},
		{
			name:           "invalid offset",
			url:            "/product?offset=xyz",
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "invalid offset: \"xyz\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			url := tt.url
			if url == "" {
				url = "/product"
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
			} else {
				products := decodeJSON[[]entity.Product](t, resp.Body)
				if len(products) != len(tt.expectedNames) {
					t.Fatalf("got len %d, want %d", len(products), len(tt.expectedNames))
				}
				for i, name := range tt.expectedNames {
					if products[i].Name != name {
						t.Errorf("got name %q, want %q", products[i].Name, name)
					}
				}
			}
		})
	}
}

func TestAddProduct(t *testing.T) {
	mux, repo := setupTest(t, testHTTPConfig)

	tests := []struct {
		name           string
		body           any
		setupMock      func()
		expectedStatus int
		expectedMsg    string
	}{
		{
			name: "success",
			body: productInput{Name: "Car", Price: 1.23},
			setupMock: func() {
				repo.save = func(_ context.Context, p entity.Product) (entity.Product, error) {
					return p, nil
				}
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "extra field",
			body:           map[string]any{"name": "Car", "price": 1.23, "email": "a@a.com"},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "unknown field \"email\"",
		},
		{
			name:           "client supplied id rejected",
			body:           map[string]any{"id": uuid.Must(uuid.NewV7()).String(), "name": "Car", "price": 1.23},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "unknown field \"id\"",
		},
		{
			name:           "negative price",
			body:           productInput{Name: "Car", Price: -1.0},
			setupMock:      func() {},
			expectedStatus: http.StatusUnprocessableEntity,
			expectedMsg:    "the product price must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			b, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/product", bytes.NewReader(b))
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

func TestAddProductBodyTooLarge(t *testing.T) {
	const limit = 16 // bytes
	cfg := testHTTPConfig
	cfg.MaxBodyBytes = limit
	mux, _ := setupTest(t, cfg)

	body := []byte(`{"name":"a long enough name","price":1.0}`)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/product", bytes.NewReader(body))
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
	mux, repo := setupTest(t, testHTTPConfig)

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
				repo.delete = func(_ context.Context, _ uuid.UUID) error {
					return entity.ErrNotFound
				}
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "success",
			id:   uuid.Must(uuid.NewV7()).String(),
			setupMock: func() {
				repo.delete = func(_ context.Context, _ uuid.UUID) error {
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
			req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/product/"+tt.id, nil)
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
	mux, repo := setupTest(t, testHTTPConfig)

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
			body: productInput{Name: "Updated", Price: 99.9},
			setupMock: func() {
				repo.update = func(_ context.Context, _ entity.Product) error {
					return nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedName:   "Updated",
		},
		{
			name:           "client supplied id rejected",
			id:             uuid.Must(uuid.NewV7()).String(),
			body:           map[string]any{"id": uuid.Must(uuid.NewV7()).String(), "name": "Updated", "price": 99.9},
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

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/product/"+tt.id, bytes.NewReader(b))
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				p := decodeJSON[entity.Product](t, resp.Body)
				if p.Name != tt.expectedName {
					t.Errorf("got name %q, want %q", p.Name, tt.expectedName)
				}
			}
		})
	}
}
