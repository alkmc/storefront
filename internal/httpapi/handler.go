package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/alkmc/restClean/internal/cache"
	"github.com/alkmc/restClean/internal/entity"
	"github.com/alkmc/restClean/internal/service"
	"github.com/alkmc/restClean/internal/serviceerr"
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
		respondError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	p := c.productCache.Get(ctx, idStr)
	if p == nil {
		p, err := c.findProduct(ctx, id)
		if err != nil {
			errs := serviceerr.Input("no product found!")
			respondError(w, errs)
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
		errs := serviceerr.Codec("decoding error")
		respondError(w, errs)
		return
	}
	if len(products) == 0 {
		confirmation := &serviceerr.ServiceError{
			Code: "ok", Message: "no products found",
		}
		respond(w, http.StatusOK, confirmation)
		return
	}
	respond(w, http.StatusOK, products)
}

func (c *Handler) Add(w http.ResponseWriter, r *http.Request) {
	var p entity.Product
	if err := c.decodeBody(r, &p); err != nil {
		respondError(w, err)
		return
	}

	if err := c.productValidator.Product(&p); err != nil {
		errs := serviceerr.Valid(err.Error())
		respondError(w, errs)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	result, err := c.productService.Create(ctx, &p)
	if err != nil {
		c.logger.Error("failed to create product", slog.Any("error", err))
		errs := serviceerr.Internal("error saving the product")
		respondError(w, errs)
		return
	}

	c.productCache.Set(ctx, p.ID.String(), &p)

	respond(w, http.StatusCreated, result)
}

func (c *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := c.validID(idStr)
	if err != nil {
		respondError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	if _, err := c.findProduct(ctx, id); err != nil {
		errs := serviceerr.Input("unable to delete product, which already does not exist")
		respondError(w, errs)
		return
	}

	if err := c.productService.Delete(ctx, id); err != nil {
		c.logger.Error("failed to delete product",
			slog.Any("error", err), slog.String("id", id.String()))
		err := serviceerr.Internal("error deleting product")
		respondError(w, err)
		return
	}

	c.productCache.Expire(ctx, idStr)
	confirmation := &serviceerr.ServiceError{
		Code: "ok", Message: "product deleted",
	}
	respond(w, http.StatusOK, confirmation)
}

func (c *Handler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := c.validID(idStr)
	if err != nil {
		respondError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	if _, err := c.findProduct(ctx, id); err != nil {
		errs := serviceerr.Input("unable to update product, which does not exist")
		respondError(w, errs)
		return
	}

	var p entity.Product
	if err := c.decodeBody(r, &p); err != nil {
		respondError(w, err)
		return
	}

	if p.ID == uuid.Nil {
		p.ID = id
	}

	if p.ID != id {
		errs := serviceerr.Input("uuid is guaranteed to be unique and shall be not changed")
		respondError(w, errs)
		return
	}

	if err := c.productService.Update(ctx, &p); err != nil {
		c.logger.Error("failed to update product",
			slog.Any("error", err), slog.String("id", id.String()))
		err := serviceerr.Internal("error updating product")
		respondError(w, err)
		return
	}
	respond(w, http.StatusOK, p)
}

func (c *Handler) findProduct(ctx context.Context, id uuid.UUID) (*entity.Product, error) {
	return c.productService.FindByID(ctx, id)
}

func (c *Handler) validID(id string) (uuid.UUID, *serviceerr.ServiceError) {
	uid, err := c.productValidator.UUID(id)
	if err != nil {
		return uuid.Nil, serviceerr.Input(err.Error())
	}
	return uid, nil
}

func (c *Handler) decodeBody(r *http.Request, p *entity.Product) *serviceerr.ServiceError {
	if err := decodeBody(r.Body, &p); err != nil {
		valErr := c.productValidator.Body(err)
		return serviceerr.Body(valErr.Error())
	}
	return nil
}
