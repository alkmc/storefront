package controller

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/alkmc/restClean/product/cache"
	"github.com/alkmc/restClean/product/entity"
	"github.com/alkmc/restClean/product/service"
	"github.com/alkmc/restClean/product/validator"
	"github.com/alkmc/restClean/renderer"
	"github.com/alkmc/restClean/serviceerr"

	"github.com/go-chi/chi"
	"github.com/google/uuid"
)

type productController struct {
	productService   service.Service
	productCache     cache.Cache
	productValidator validator.Validator
}

//NewController returns Product Controller
func NewController(s service.Service, c cache.Cache, v validator.Validator) Controller {
	return &productController{
		productService:   s,
		productCache:     c,
		productValidator: v,
	}
}

func (c *productController) GetProductByID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		log.Println(err.Error())
		errs := serviceerr.Input("Invalid uuid")
		errs.JSON(w)
		return
	}

	p := c.productCache.Get(idStr)
	if p == nil {
		p, err := c.productService.FindByID(id)
		if err != nil {
			errs := serviceerr.Input("No product found!")
			errs.JSON(w)
			return
		}
		c.productCache.Set(idStr, p)
		p.JSON(w)
	} else {
		p.JSON(w)
	}
}

func (c *productController) GetProducts(w http.ResponseWriter, r *http.Request) {
	products, err := c.productService.FindAll()
	if err != nil {
		log.Println(err.Error())
		errs := serviceerr.Codec("decoding error")
		errs.JSON(w)
		return
	}
	if len(products) == 0 {
		confirmation := &serviceerr.ServiceError{
			Code: "OK", Message: "No products found",
		}
		renderer.JSON(w, http.StatusOK, confirmation)
		return
	}
	renderer.JSON(w, http.StatusOK, products)
}

func (c *productController) AddProduct(w http.ResponseWriter, r *http.Request) {
	var p entity.Product
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		err := serviceerr.Codec("decoding error")
		err.JSON(w)
		return
	}

	if err := c.productValidator.Product(&p); err != nil {
		errs := serviceerr.Valid(err.Error())
		errs.JSON(w)
		return
	}

	result, err := c.productService.Create(&p)
	if err != nil {
		errs := serviceerr.Internal("Error saving the product")
		errs.JSON(w)
		return
	}
	c.productCache.Set(p.ID.String(), &p)

	renderer.JSON(w, http.StatusCreated, result)
}

func (c *productController) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := c.validID(idStr)
	if err != nil {
		err.JSON(w)
		return
	}
	if err := c.productService.Delete(id); err != nil {
		log.Println(err.Error())
		err := serviceerr.Internal("error deleting product")
		err.JSON(w)
		return
	}
	c.productCache.Expire(idStr)
	confirmation := &serviceerr.ServiceError{
		Code: "OK", Message: "Product deleted",
	}
	renderer.JSON(w, http.StatusOK, confirmation)
}

func (c *productController) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := c.validID(idStr)
	if err != nil {
		err.JSON(w)
		return
	}

	var p entity.Product
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		errs := serviceerr.Input("Invalid request payload")
		errs.JSON(w)
		return
	}

	if p.ID == uuid.Nil {
		p.ID = id
	}

	if p.ID != id {
		errs := serviceerr.Input("UUID is guaranteed to be unique and shall be not changed")
		errs.JSON(w)
		return
	}

	if err := c.productService.Update(&p); err != nil {
		log.Println(err.Error())
		err := serviceerr.Internal("error updating product")
		err.JSON(w)
		return
	}
	p.JSON(w)
}

func (c *productController) validID(id string) (uuid.UUID, *serviceerr.ServiceError) {
	uid, err := c.productValidator.UUID(id)
	if err != nil {
		return uuid.Nil, serviceerr.Input(err.Error())
	}
	return uid, nil
}
