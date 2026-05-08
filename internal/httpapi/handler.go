package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/alkmc/restClean/internal/cache"
	"github.com/alkmc/restClean/internal/entity"
	"github.com/google/uuid"
)

const (
	defaultLimit = 50
	maxLimit     = 200
)

type (
	cacher interface {
		Set(ctx context.Context, key string, value entity.Product) error
		Get(ctx context.Context, key string) (entity.Product, error)
		Invalidate(ctx context.Context, key string) error
	}

	processor interface {
		Create(context.Context, *entity.Product) (*entity.Product, error)
		FindByID(context.Context, uuid.UUID) (*entity.Product, error)
		FindAll(ctx context.Context, limit, offset int) ([]entity.Product, error)
		Update(context.Context, *entity.Product) error
		Delete(context.Context, uuid.UUID) error
	}

	Handler struct {
		logger         *slog.Logger
		processor      processor
		cache          cacher
		requestTimeout time.Duration
	}

	productInput struct {
		Name  string  `json:"name"`
		Price float64 `json:"price"`
	}
)

// NewHandler initializes a product API handler with its required dependencies
func NewHandler(l *slog.Logger, p processor, c cacher, requestTimeout time.Duration) *Handler {
	return &Handler{
		logger:         l,
		processor:      p,
		cache:          c,
		requestTimeout: requestTimeout,
	}
}

func (c *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), c.requestTimeout)
	defer cancel()

	cached, err := c.cache.Get(ctx, idStr)
	if err == nil {
		respond(w, http.StatusOK, cached)
		return
	}
	if !errors.Is(err, cache.ErrCacheMiss) {
		c.logger.Warn("cache get failed", slog.Any("error", err), slog.String("key", idStr))
	}

	p, err := c.processor.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondError(w, http.StatusNotFound, "product not found")
		} else {
			c.logger.Error("failed to find product by id",
				slog.Any("error", err), slog.String("id", id.String()))
			respondError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	if err := c.cache.Set(ctx, idStr, *p); err != nil {
		c.logger.Warn("cache set failed", slog.Any("error", err), slog.String("key", idStr))
	}
	respond(w, http.StatusOK, p)
}

func (c *Handler) Get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := parseLimit(q.Get("limit"))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := parseOffset(q.Get("offset"))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), c.requestTimeout)
	defer cancel()

	products, err := c.processor.FindAll(ctx, limit, offset)
	if err != nil {
		c.logger.Error("failed to find all products", slog.Any("error", err))
		respondError(w, http.StatusInternalServerError, "failed to fetch products")
		return
	}
	if products == nil {
		products = []entity.Product{}
	}
	respond(w, http.StatusOK, products)
}

func (c *Handler) Add(w http.ResponseWriter, r *http.Request) {
	var in productInput
	if err := decodeBody(r.Body, &in); err != nil {
		respondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	p := entity.Product{Name: in.Name, Price: in.Price}
	if err := p.Validate(); err != nil {
		respondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), c.requestTimeout)
	defer cancel()

	result, err := c.processor.Create(ctx, &p)
	if err != nil {
		c.logger.Error("failed to create product", slog.Any("error", err))
		respondError(w, http.StatusInternalServerError, "error saving the product")
		return
	}

	key := result.ID.String()
	if err := c.cache.Set(ctx, key, *result); err != nil {
		c.logger.Warn("cache set failed", slog.Any("error", err), slog.String("key", key))
	}

	respond(w, http.StatusCreated, result)
}

func (c *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), c.requestTimeout)
	defer cancel()

	if _, err := c.processor.FindByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondError(w, http.StatusNotFound, "unable to delete product, which does not exist")
		} else {
			c.logger.Error("failed to find product before delete",
				slog.Any("error", err), slog.String("id", id.String()))
			respondError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	if err := c.processor.Delete(ctx, id); err != nil {
		c.logger.Error("failed to delete product",
			slog.Any("error", err), slog.String("id", id.String()))
		respondError(w, http.StatusInternalServerError, "error deleting product")
		return
	}

	if err := c.cache.Invalidate(ctx, idStr); err != nil {
		c.logger.Warn("cache invalidate failed", slog.Any("error", err), slog.String("key", idStr))
	}
	respond(w, http.StatusOK, map[string]string{"message": "product deleted"})
}

func (c *Handler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), c.requestTimeout)
	defer cancel()

	if _, err := c.processor.FindByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondError(w, http.StatusNotFound, "unable to update product, which does not exist")
		} else {
			c.logger.Error("failed to find product before update",
				slog.Any("error", err), slog.String("id", id.String()))
			respondError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	var in productInput
	if err := decodeBody(r.Body, &in); err != nil {
		respondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	p := entity.Product{ID: id, Name: in.Name, Price: in.Price}
	if err := p.Validate(); err != nil {
		respondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if err := c.processor.Update(ctx, &p); err != nil {
		c.logger.Error("failed to update product",
			slog.Any("error", err), slog.String("id", id.String()))
		respondError(w, http.StatusInternalServerError, "error updating product")
		return
	}

	if err := c.cache.Invalidate(ctx, idStr); err != nil {
		c.logger.Warn("cache invalidate failed", slog.Any("error", err), slog.String("key", idStr))
	}
	respond(w, http.StatusOK, p)
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return defaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid limit: %q", raw)
	}
	if n <= 0 {
		return defaultLimit, nil
	}
	return min(n, maxLimit), nil
}

func parseOffset(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid offset: %q", raw)
	}
	return max(n, 0), nil
}
