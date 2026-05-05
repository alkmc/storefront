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
	"github.com/alkmc/restClean/internal/service"
	"github.com/alkmc/restClean/internal/validator"
	"github.com/google/uuid"
)

const timeout = 2 * time.Second

type Handler struct {
	logger           *slog.Logger
	productService   service.Service
	productCache     cache.Cache
	productValidator validator.Validator
}

// NewHandler returns Product Handler
func NewHandler(l *slog.Logger, s service.Service, c cache.Cache, v validator.Validator) *Handler {
	return &Handler{
		logger:           l,
		productService:   s,
		productCache:     c,
		productValidator: v,
	}
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
	p := c.productCache.Get(ctx, idStr)
	if p == nil {
		p, err := c.findProduct(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondError(w, http.StatusNotFound, "product not found")
			} else {
				c.logger.Error("failed to find product by id", slog.Any("error", err), slog.String("id", id.String()))
				respondError(w, http.StatusInternalServerError, "internal server error")
			}
			return
		}
		c.productCache.Set(ctx, idStr, p)
		respond(w, http.StatusOK, p)
	} else {
		respond(w, http.StatusOK, p)
	}
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

	c.productCache.Set(ctx, p.ID.String(), &p)

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
			c.logger.Error("failed to find product before delete", slog.Any("error", err), slog.String("id", id.String()))
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

	c.productCache.Expire(ctx, idStr)
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
			c.logger.Error("failed to find product before update", slog.Any("error", err), slog.String("id", id.String()))
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
	if err := decodeBody(r.Body, &p); err != nil {
		valErr := c.productValidator.Body(err)
		return valErr
	}
	return nil
}
