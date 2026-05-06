package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/alkmc/restClean/internal/cache"
	"github.com/alkmc/restClean/internal/entity"
	"github.com/google/uuid"
)

const timeout = 2 * time.Second

// productCache is what the handler needs from a cache implementation
type productCache interface {
	Set(ctx context.Context, key string, value entity.Product) error
	Get(ctx context.Context, key string) (entity.Product, error)
	Invalidate(ctx context.Context, key string) error
}

type productService interface {
	Create(context.Context, *entity.Product) (*entity.Product, error)
	FindByID(context.Context, uuid.UUID) (*entity.Product, error)
	FindAll(context.Context) ([]entity.Product, error)
	Update(context.Context, *entity.Product) error
	Delete(context.Context, uuid.UUID) error
}

type productValidator interface {
	Product(*entity.Product) error
	Body(error) error
	UUID(string) (uuid.UUID, error)
}

type Handler struct {
	logger           *slog.Logger
	productService   productService
	productCache     productCache
	productValidator productValidator
}

// NewHandler returns Product Handler
func NewHandler(l *slog.Logger, s productService, c productCache, v productValidator,
) *Handler {
	return new(Handler{
		logger:           l,
		productService:   s,
		productCache:     c,
		productValidator: v,
	})
}

func (c *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := c.validID(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	cached, err := c.productCache.Get(ctx, idStr)
	if err == nil {
		respond(w, http.StatusOK, cached)
		return
	}
	if !errors.Is(err, cache.ErrCacheMiss) {
		c.logger.Warn("cache get failed", slog.Any("error", err), slog.String("key", idStr))
	}

	p, err := c.findProduct(ctx, id)
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
	if err := c.productCache.Set(ctx, idStr, *p); err != nil {
		c.logger.Warn("cache set failed", slog.Any("error", err), slog.String("key", idStr))
	}
	respond(w, http.StatusOK, p)
}

func (c *Handler) Get(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	products, err := c.productService.FindAll(ctx)
	if err != nil {
		c.logger.Error("failed to find all products", slog.Any("error", err))
		respondError(w, http.StatusInternalServerError, "failed to fetch products")
		return
	}
	if len(products) == 0 {
		respond(w, http.StatusOK, map[string]string{"message": "no products found"})
		return
	}
	respond(w, http.StatusOK, products)
}

func (c *Handler) Add(w http.ResponseWriter, r *http.Request) {
	var p entity.Product
	if err := c.decodeBody(r, &p); err != nil {
		respondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if err := c.productValidator.Product(&p); err != nil {
		respondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	result, err := c.productService.Create(ctx, &p)
	if err != nil {
		c.logger.Error("failed to create product", slog.Any("error", err))
		respondError(w, http.StatusInternalServerError, "error saving the product")
		return
	}

	key := result.ID.String()
	if err := c.productCache.Set(ctx, key, *result); err != nil {
		c.logger.Warn("cache set failed", slog.Any("error", err), slog.String("key", key))
	}

	respond(w, http.StatusCreated, result)
}

func (c *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := c.validID(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	if _, err := c.findProduct(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondError(w, http.StatusNotFound, "unable to delete product, which does not exist")
		} else {
			c.logger.Error("failed to find product before delete",
				slog.Any("error", err), slog.String("id", id.String()))
			respondError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	if err := c.productService.Delete(ctx, id); err != nil {
		c.logger.Error("failed to delete product",
			slog.Any("error", err), slog.String("id", id.String()))
		respondError(w, http.StatusInternalServerError, "error deleting product")
		return
	}

	if err := c.productCache.Invalidate(ctx, idStr); err != nil {
		c.logger.Warn("cache invalidate failed", slog.Any("error", err), slog.String("key", idStr))
	}
	respond(w, http.StatusOK, map[string]string{"message": "product deleted"})
}

func (c *Handler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := c.validID(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	if _, err := c.findProduct(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondError(w, http.StatusNotFound, "unable to update product, which does not exist")
		} else {
			c.logger.Error("failed to find product before update",
				slog.Any("error", err), slog.String("id", id.String()))
			respondError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	var p entity.Product
	if err := c.decodeBody(r, &p); err != nil {
		respondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if p.ID == uuid.Nil {
		p.ID = id
	}

	if p.ID != id {
		respondError(w, http.StatusBadRequest, "uuid is guaranteed to be unique and shall be not changed")
		return
	}

	if err := c.productService.Update(ctx, &p); err != nil {
		c.logger.Error("failed to update product",
			slog.Any("error", err), slog.String("id", id.String()))
		respondError(w, http.StatusInternalServerError, "error updating product")
		return
	}

	if err := c.productCache.Invalidate(ctx, idStr); err != nil {
		c.logger.Warn("cache invalidate failed", slog.Any("error", err), slog.String("key", idStr))
	}
	respond(w, http.StatusOK, p)
}

func (c *Handler) findProduct(ctx context.Context, id uuid.UUID) (*entity.Product, error) {
	return c.productService.FindByID(ctx, id)
}

func (c *Handler) validID(id string) (uuid.UUID, error) {
	uid, err := c.productValidator.UUID(id)
	if err != nil {
		return uuid.Nil, err
	}
	return uid, nil
}

func (c *Handler) decodeBody(r *http.Request, p *entity.Product) error {
	if err := decodeBody(r.Body, p); err != nil {
		valErr := c.productValidator.Body(err)
		return valErr
	}
	return nil
}
