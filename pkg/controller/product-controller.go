package controller

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/alkmc/restClean/internal/renderer"
	"github.com/alkmc/restClean/internal/serviceerr"
	"github.com/alkmc/restClean/pkg/cache"
	"github.com/alkmc/restClean/pkg/entity"
	"github.com/alkmc/restClean/pkg/service"
	"github.com/alkmc/restClean/pkg/validator"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const timeout = 20 * time.Millisecond

type productController struct {
	productService   service.Service
	productCache     cache.Cache
	productValidator validator.Validator
}

// NewController returns Product Controller
func NewController(s service.Service, c cache.Cache, v validator.Validator) Controller {
	return &productController{
		productService:   s,
		productCache:     c,
		productValidator: v,
	}
}

func (c *productController) GetByID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := c.validID(idStr)
	if err != nil {
		err.Encode(w)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	p := c.productCache.Get(ctx, idStr)
	if p == nil {
		p, err := c.findProduct(id)
		if err != nil {
			errs := serviceerr.Input("no product found!")
			errs.Encode(w)
			return
		}
		c.productCache.Set(ctx, idStr, p)
		p.JSON(w)
	} else {
		p.JSON(w)
	}
}

func (c *productController) Get(w http.ResponseWriter, r *http.Request) {
	products, err := c.productService.FindAll()
	if err != nil {
		log.Println(err.Error())
		errs := serviceerr.Codec("decoding error")
		errs.Encode(w)
		return
	}
	if len(products) == 0 {
		confirmation := &serviceerr.ServiceError{
			Code: "ok", Message: "no products found",
		}
		renderer.JSON(w, http.StatusOK, confirmation)
		return
	}
	renderer.JSON(w, http.StatusOK, products)
}

func (c *productController) Add(w http.ResponseWriter, r *http.Request) {
	var p entity.Product
	if err := c.decode(r, &p); err != nil {
		err.Encode(w)
		return
	}

	if err := c.productValidator.Product(&p); err != nil {
		errs := serviceerr.Valid(err.Error())
		errs.Encode(w)
		return
	}

	result, err := c.productService.Create(&p)
	if err != nil {
		errs := serviceerr.Internal("error saving the product")
		errs.Encode(w)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	c.productCache.Set(ctx, p.ID.String(), &p)

	renderer.JSON(w, http.StatusCreated, result)
}

func (c *productController) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := c.validID(idStr)
	if err != nil {
		err.Encode(w)
		return
	}

	if _, err := c.findProduct(id); err != nil {
		errs := serviceerr.Input("unable to delete product, which already does not exist")
		errs.Encode(w)
		return
	}

	if err := c.productService.Delete(id); err != nil {
		log.Println(err.Error())
		err := serviceerr.Internal("error deleting product")
		err.Encode(w)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	c.productCache.Expire(ctx, idStr)
	confirmation := &serviceerr.ServiceError{
		Code: "ok", Message: "product deleted",
	}
	renderer.JSON(w, http.StatusOK, confirmation)
}

func (c *productController) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := c.validID(idStr)
	if err != nil {
		err.Encode(w)
		return
	}

	if _, err := c.findProduct(id); err != nil {
		errs := serviceerr.Input("unable to update product, which does not exist")
		errs.Encode(w)
		return
	}

	var p entity.Product
	if err := c.decode(r, &p); err != nil {
		err.Encode(w)
		return
	}

	if p.ID == uuid.Nil {
		p.ID = id
	}

	if p.ID != id {
		errs := serviceerr.Input("uuid is guaranteed to be unique and shall be not changed")
		errs.Encode(w)
		return
	}

	if err := c.productService.Update(&p); err != nil {
		log.Println(err.Error())
		err := serviceerr.Internal("error updating product")
		err.Encode(w)
		return
	}
	p.JSON(w)
}

func (c *productController) findProduct(id uuid.UUID) (*entity.Product, error) {
	return c.productService.FindByID(id)
}

func (c *productController) validID(id string) (uuid.UUID, *serviceerr.ServiceError) {
	uid, err := c.productValidator.UUID(id)
	if err != nil {
		return uuid.Nil, serviceerr.Input(err.Error())
	}
	return uid, nil
}

func (c *productController) decode(r *http.Request, p *entity.Product) *serviceerr.ServiceError {
	if err := renderer.Decode(r.Body, &p); err != nil {
		valErr := c.productValidator.Body(err)
		return serviceerr.Body(valErr.Error())
	}
	return nil
}
