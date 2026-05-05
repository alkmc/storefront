package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alkmc/restClean/internal/entity"
	"github.com/alkmc/restClean/internal/repository"
	"github.com/alkmc/restClean/internal/service"
	"github.com/alkmc/restClean/internal/validator"
	"github.com/google/uuid"
)

const (
	NAME  = "Car"
	PRICE = 1.23
)

type responseMessage struct {
	Message string `json:"message"`
}

type mockRepo struct {
	repository.Repository
	save     func(p *entity.Product) (*entity.Product, error)
	findByID func(id uuid.UUID) (*entity.Product, error)
	findAll  func() ([]entity.Product, error)
	update   func(p *entity.Product) error
	delete   func(id uuid.UUID) error
}

func (m mockRepo) Save(ctx context.Context, p *entity.Product) (*entity.Product, error) {
	return m.save(p)
}
func (m mockRepo) FindByID(ctx context.Context, id uuid.UUID) (*entity.Product, error) {
	if m.findByID == nil {
		return nil, sql.ErrNoRows
	}
	return m.findByID(id)
}
func (m mockRepo) FindAll(ctx context.Context) ([]entity.Product, error) {
	return m.findAll()
}
func (m mockRepo) Update(ctx context.Context, p *entity.Product) error {
	return m.update(p)
}
func (m mockRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return m.delete(id)
}
func (m mockRepo) CloseDB() {}

type mockCache struct{}

func (m *mockCache) Set(ctx context.Context, key string, value *entity.Product) {}
func (m *mockCache) Get(ctx context.Context, key string) *entity.Product        { return nil }
func (m *mockCache) Expire(ctx context.Context, key string)                     {}

func setupTest(t *testing.T) (http.Handler, *mockRepo) {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	repo := new(mockRepo{})

	srv := service.NewService(repo)
	cacheSrv := new(mockCache)
	valid := validator.NewValidator()
	h := NewHandler(logger, srv, cacheSrv, valid)
	mux := NewMux(logger, h)

	return mux, repo
}

func TestGetProductByID(t *testing.T) {
	mux, repo := setupTest(t)
	uid := uuid.New()

	tests := []struct {
		name           string
		id             string
		setupMock      func()
		expectedStatus int
		expectedMsg    string // for error cases
	}{
		{
			name: "success",
			id:   uid.String(),
			setupMock: func() {
				repo.findByID = func(id uuid.UUID) (*entity.Product, error) {
					return new(entity.Product{ID: uid, Name: NAME, Price: PRICE}), nil
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
			id:   uuid.New().String(),
			setupMock: func() {
				repo.findByID = func(id uuid.UUID) (*entity.Product, error) {
					return nil, sql.ErrNoRows
				}
			},
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "product not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			req := httptest.NewRequestWithContext(t.Context(), "GET", "/product/"+tt.id, nil)
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				var p entity.Product
				err := json.NewDecoder(resp.Body).Decode(&p)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if p.ID != uid {
					t.Errorf("got id %v, want %v", p.ID, uid)
				}
				if p.Name != NAME {
					t.Errorf("got name %v, want %v", p.Name, NAME)
				}
			} else {
				var e responseMessage
				err := json.NewDecoder(resp.Body).Decode(&e)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if e.Message != tt.expectedMsg {
					t.Errorf("got msg %q, want %q", e.Message, tt.expectedMsg)
				}
			}
		})
	}
}

func TestGetProducts(t *testing.T) {
	mux, repo := setupTest(t)

	tests := []struct {
		name           string
		setupMock      func()
		expectedStatus int
		expectedMsg    string
		expectedNames  []string
	}{
		{
			name: "empty",
			setupMock: func() {
				repo.findAll = func() ([]entity.Product, error) {
					return nil, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedMsg:    "no products found",
		},
		{
			name: "success",
			setupMock: func() {
				repo.findAll = func() ([]entity.Product, error) {
					return []entity.Product{{Name: NAME, Price: PRICE}}, nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{NAME},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			req := httptest.NewRequestWithContext(t.Context(), "GET", "/product", nil)
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedMsg != "" {
				var e responseMessage
				err := json.NewDecoder(resp.Body).Decode(&e)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if e.Message != tt.expectedMsg {
					t.Errorf("got msg %q, want %q", e.Message, tt.expectedMsg)
				}
			} else {
				var products []entity.Product
				err := json.NewDecoder(resp.Body).Decode(&products)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
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
	mux, repo := setupTest(t)

	tests := []struct {
		name           string
		body           any
		setupMock      func()
		expectedStatus int
		expectedMsg    string
	}{
		{
			name: "success",
			body: entity.Product{Name: NAME, Price: PRICE},
			setupMock: func() {
				repo.save = func(p *entity.Product) (*entity.Product, error) {
					p.ID = uuid.New()
					return p, nil
				}
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "extra field",
			body:           map[string]any{"Name": NAME, "Price": PRICE, "Email": "a@a.com"},
			setupMock:      func() {},
			expectedStatus: http.StatusUnprocessableEntity,
			expectedMsg:    "unknown field \"Email\"",
		},
		{
			name:           "negative price",
			body:           entity.Product{Name: NAME, Price: -1.0},
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

			req := httptest.NewRequestWithContext(t.Context(), "POST", "/product", bytes.NewBuffer(b))
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedMsg != "" {
				var e responseMessage
				err := json.NewDecoder(resp.Body).Decode(&e)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if e.Message != tt.expectedMsg {
					t.Errorf("got msg %q, want %q", e.Message, tt.expectedMsg)
				}
			}
		})
	}
}

func TestDeleteProduct(t *testing.T) {
	mux, repo := setupTest(t)
	uid := uuid.New()

	tests := []struct {
		name           string
		id             string
		setupMock      func()
		expectedStatus int
		expectedMsg    string
	}{
		{
			name: "not existing",
			id:   uuid.New().String(),
			setupMock: func() {
				repo.findByID = func(id uuid.UUID) (*entity.Product, error) {
					return nil, sql.ErrNoRows
				}
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "success",
			id:   uid.String(),
			setupMock: func() {
				repo.findByID = func(id uuid.UUID) (*entity.Product, error) {
					return new(entity.Product{ID: uid, Name: NAME, Price: PRICE}), nil
				}
				repo.delete = func(id uuid.UUID) error {
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
			req := httptest.NewRequestWithContext(t.Context(), "DELETE", "/product/"+tt.id, nil)
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedMsg != "" {
				var e responseMessage
				err := json.NewDecoder(resp.Body).Decode(&e)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if e.Message != tt.expectedMsg {
					t.Errorf("got msg %q, want %q", e.Message, tt.expectedMsg)
				}
			}
		})
	}
}

func TestUpdateProduct(t *testing.T) {
	mux, repo := setupTest(t)
	uid := uuid.New()

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
			id:   uid.String(),
			body: entity.Product{ID: uid, Name: "Updated", Price: 99.9},
			setupMock: func() {
				repo.findByID = func(id uuid.UUID) (*entity.Product, error) {
					return new(entity.Product{ID: uid, Name: NAME, Price: PRICE}), nil
				}
				repo.update = func(p *entity.Product) error {
					return nil
				}
			},
			expectedStatus: http.StatusOK,
			expectedName:   "Updated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			b, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			req := httptest.NewRequestWithContext(t.Context(), "PUT", "/product/"+tt.id, bytes.NewBuffer(b))
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				var p entity.Product
				err = json.NewDecoder(resp.Body).Decode(&p)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if p.Name != tt.expectedName {
					t.Errorf("got name %q, want %q", p.Name, tt.expectedName)
				}
			}
		})
	}
}
