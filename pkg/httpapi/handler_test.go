package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alkmc/restClean/internal/serviceerr"
	"github.com/alkmc/restClean/pkg/cache"
	"github.com/alkmc/restClean/pkg/entity"
	"github.com/alkmc/restClean/pkg/repository"
	"github.com/alkmc/restClean/pkg/service"
	"github.com/alkmc/restClean/pkg/validator"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	NAME  = "Car"
	PRICE = 1.23
)

func setupTest(t *testing.T) (*Handler, http.Handler) {
	logger := slog.New(slog.DiscardHandler)
	repo, err := repository.NewSQLite(logger)
	require.NoError(t, err)

	t.Cleanup(func() {
		repo.CloseDB()
	})

	srv := service.NewService(repo)
	cacheSrv := cache.NewRedis(logger, "localhost:6379", 0, 10)
	valid := validator.NewValidator()
	h := NewHandler(logger, srv, cacheSrv, valid)
	mux := NewMux(logger, h)

	return h, mux
}

func TestGetProductByID(t *testing.T) {
	h, mux := setupTest(t)
	uid := uuid.New()

	// Seed data
	_, err := h.productService.Create(t.Context(), &entity.Product{ID: uid, Name: NAME, Price: PRICE})
	require.NoError(t, err)

	tests := []struct {
		name           string
		id             string
		expectedStatus int
		expectedMsg    string // for error cases
	}{
		{
			name:           "success",
			id:             uid.String(),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "incorrect uuid",
			id:             "incorrect",
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "invalid UUID length: 9",
		},
		{
			name:           "non-existing product",
			id:             uuid.New().String(),
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "no product found!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/product/"+tt.id, nil)
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			assert.Equal(t, tt.expectedStatus, resp.Code)

			if tt.expectedStatus == http.StatusOK {
				var p entity.Product
				err := json.NewDecoder(resp.Body).Decode(&p)
				require.NoError(t, err)
				assert.Equal(t, uid, p.ID)
				assert.Equal(t, NAME, p.Name)
			} else {
				var e serviceerr.ServiceError
				err := json.NewDecoder(resp.Body).Decode(&e)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedMsg, e.Message)
			}
		})
	}
}

func TestGetProducts(t *testing.T) {
	h, mux := setupTest(t)

	t.Run("empty", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/product", nil)
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		var e serviceerr.ServiceError
		err := json.NewDecoder(resp.Body).Decode(&e)
		require.NoError(t, err)
		assert.Equal(t, "no products found", e.Message)
	})

	t.Run("success", func(t *testing.T) {
		_, err := h.productService.Create(t.Context(), &entity.Product{Name: NAME, Price: PRICE})
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/product", nil)
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		var products []entity.Product
		err = json.NewDecoder(resp.Body).Decode(&products)
		require.NoError(t, err)
		assert.NotEmpty(t, products)
		assert.Equal(t, NAME, products[0].Name)
	})
}

func TestAddProduct(t *testing.T) {
	_, mux := setupTest(t)

	tests := []struct {
		name           string
		body           any
		expectedStatus int
		expectedMsg    string
	}{
		{
			name:           "success",
			body:           entity.Product{Name: NAME, Price: PRICE},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "extra field",
			body:           map[string]any{"Name": NAME, "Price": PRICE, "Email": "a@a.com"},
			expectedStatus: http.StatusUnprocessableEntity,
			expectedMsg:    "unknown field \"Email\"",
		},
		{
			name:           "negative price",
			body:           entity.Product{Name: NAME, Price: -1.0},
			expectedStatus: http.StatusUnprocessableEntity,
			expectedMsg:    "the product price must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.body)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/product", bytes.NewBuffer(b))
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			assert.Equal(t, tt.expectedStatus, resp.Code)

			if tt.expectedMsg != "" {
				var e serviceerr.ServiceError
				err := json.NewDecoder(resp.Body).Decode(&e)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedMsg, e.Message)
			}
		})
	}
}

func TestDeleteProduct(t *testing.T) {
	h, mux := setupTest(t)
	uid := uuid.New()

	t.Run("not existing", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/product/"+uuid.New().String(), nil)
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("success", func(t *testing.T) {
		_, err := h.productService.Create(t.Context(), &entity.Product{ID: uid, Name: NAME, Price: PRICE})
		require.NoError(t, err)

		req := httptest.NewRequest("DELETE", "/product/"+uid.String(), nil)
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		var e serviceerr.ServiceError
		err = json.NewDecoder(resp.Body).Decode(&e)
		require.NoError(t, err)
		assert.Equal(t, "product deleted", e.Message)
	})
}

func TestUpdateProduct(t *testing.T) {
	h, mux := setupTest(t)
	uid := uuid.New()

	_, err := h.productService.Create(t.Context(), &entity.Product{ID: uid, Name: NAME, Price: PRICE})
	require.NoError(t, err)

	update := entity.Product{ID: uid, Name: "Updated", Price: 99.9}
	b, err := json.Marshal(update)
	require.NoError(t, err)

	req := httptest.NewRequest("PUT", "/product/"+uid.String(), bytes.NewBuffer(b))
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var p entity.Product
	err = json.NewDecoder(resp.Body).Decode(&p)
	require.NoError(t, err)
	assert.Equal(t, "Updated", p.Name)
}
