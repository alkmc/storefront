package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alkmc/restClean/internal/serviceerr"
	"github.com/alkmc/restClean/pkg/cache"
	"github.com/alkmc/restClean/pkg/entity"
	"github.com/alkmc/restClean/pkg/repository"
	"github.com/alkmc/restClean/pkg/service"
	"github.com/alkmc/restClean/pkg/validator"

	"github.com/go-chi/chi"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

const (
	NAME  string  = "Car"
	PRICE float64 = 1.23
)

var (
	pRepo       = repository.NewSQLite()
	pSrv        = service.NewService(pRepo)
	pCacheSrv   = cache.NewRedis("localhost:6379", 0, 10)
	pValid      = validator.NewValidator()
	pController = NewController(pSrv, pCacheSrv, pValid)
)

func TestGetProductByID(t *testing.T) {
	uid := uuid.New()
	setupUUID(uid)

	const path = "/product/%v"

	// create a http GET request
	req := httptest.NewRequest("GET", fmt.Sprintf(path, uid), nil)

	// record http Response
	resp := httptest.NewRecorder()

	// assign http Handler function
	r := chi.NewRouter()
	r.Get("/product/{id}", pController.GetProductByID)

	// dispatch the http request
	r.ServeHTTP(resp, req)

	// assert http status code
	checkResponseCode(t, http.StatusOK, resp.Code)

	// decode the http response
	var p entity.Product
	if err := json.NewDecoder(io.Reader(resp.Body)).Decode(&p); err != nil {
		log.Fatal(err)
	}

	// assert http response
	assert.Equal(t, uid, p.ID)
	assert.Equal(t, NAME, p.Name)
	assert.Equal(t, PRICE, p.Price)

	// clean up database
	tearDown(p.ID)
}

func TestGetProductByIncorrectID(t *testing.T) {
	const (
		errCode  = "invalid input error"
		fakeUUID = "incorrect"
		path     = "/product/%v"
	)
	errMsg := fmt.Sprintf("invalid UUID length: %d", len(fakeUUID))

	// create a http GET request
	req := httptest.NewRequest("GET", fmt.Sprintf(path, fakeUUID), nil)

	// record http response
	resp := httptest.NewRecorder()

	// assign http handler function
	r := chi.NewRouter()
	r.Get("/product/{id}", pController.GetProductByID)

	// dispatch the http request
	r.ServeHTTP(resp, req)

	// assert http status code
	checkResponseCode(t, http.StatusBadRequest, resp.Code)

	// decode the http response
	var e serviceerr.ServiceError
	if err := json.NewDecoder(io.Reader(resp.Body)).Decode(&e); err != nil {
		log.Fatal(err)
	}

	// assert http response
	assert.Equal(t, errCode, e.Code)
	assert.Equal(t, errMsg, e.Message)
}

func TestGetNotExistingProduct(t *testing.T) {
	const (
		errCode = "invalid input error"
		errMsg  = "No product found!"
		path    = "/product/%v"
	)
	uid := uuid.New()

	// create a http GET request
	req := httptest.NewRequest("GET", fmt.Sprintf(path, uid), nil)

	// record http response
	resp := httptest.NewRecorder()

	// assign http handler function
	r := chi.NewRouter()
	r.Get("/product/{id}", pController.GetProductByID)

	// dispatch the http request
	r.ServeHTTP(resp, req)

	// assert http status code
	checkResponseCode(t, http.StatusBadRequest, resp.Code)

	// decode the http response
	var e serviceerr.ServiceError
	if err := json.NewDecoder(io.Reader(resp.Body)).Decode(&e); err != nil {
		log.Fatal(err)
	}

	// assert http response
	assert.Equal(t, errCode, e.Code)
	assert.Equal(t, errMsg, e.Message)
}

func TestGetProducts(t *testing.T) {
	// insert new post
	setup()

	// create a http GET request
	req := httptest.NewRequest("GET", "/product", nil)

	// record http response
	resp := httptest.NewRecorder()

	// assign http handler function
	r := chi.NewRouter()
	r.Get("/product", pController.GetProducts)

	// dispatch the http request
	r.ServeHTTP(resp, req)

	// assert http status code
	checkResponseCode(t, http.StatusOK, resp.Code)

	// decode the http response
	var products []entity.Product
	if err := json.NewDecoder(io.Reader(resp.Body)).Decode(&products); err != nil {
		log.Fatal(err)
	}

	// assert http response
	assert.NotNil(t, products[0].ID)
	assert.Equal(t, NAME, products[0].Name)
	assert.Equal(t, PRICE, products[0].Price)

	// clean up db
	tearDown(products[0].ID)
}

func TestGetNotExistingProducts(t *testing.T) {
	const (
		statusOK = "OK"
		infoMsg  = "No products found"
	)

	// create a http GET request
	req := httptest.NewRequest("GET", "/product", nil)

	// record http response
	resp := httptest.NewRecorder()

	// assign http handler function
	r := chi.NewRouter()
	r.Get("/product", pController.GetProducts)

	// dispatch the http request
	r.ServeHTTP(resp, req)

	// assert http status code
	checkResponseCode(t, http.StatusOK, resp.Code)

	// decode the http response
	var e serviceerr.ServiceError
	if err := json.NewDecoder(io.Reader(resp.Body)).Decode(&e); err != nil {
		log.Fatal(err)
	}

	// assert http response
	assert.Equal(t, statusOK, e.Code)
	assert.Equal(t, infoMsg, e.Message)
}

func TestAddProduct(t *testing.T) {
	uid := uuid.New()
	data := entity.Product{
		ID:    uid,
		Name:  NAME,
		Price: PRICE,
	}
	jsonReq, err := json.Marshal(data)
	if err != nil {
		log.Println(err)
	}
	// create a new http POST request
	req := httptest.NewRequest("POST", "/product", bytes.NewBuffer(jsonReq))

	// record http response
	resp := httptest.NewRecorder()

	// assign http handler function
	r := chi.NewRouter()
	r.Post("/product", pController.AddProduct)

	// dispatch the http request
	r.ServeHTTP(resp, req)

	// assert http status code
	checkResponseCode(t, http.StatusCreated, resp.Code)

	// decode the http response
	var p entity.Product
	if err := json.NewDecoder(io.Reader(resp.Body)).Decode(&p); err != nil {
		log.Fatal(err)
	}

	// assert http response
	assert.Equal(t, uid, p.ID)
	assert.Equal(t, NAME, p.Name)
	assert.Equal(t, PRICE, p.Price)

	// clean up db
	tearDown(p.ID)
}

func TestDeleteProduct(t *testing.T) {
	uid := uuid.New()
	setupUUID(uid)

	const (
		path     = "/product/%v"
		statusOK = "OK"
		pDeleted = "Product deleted"
	)

	// create a new http DELETE request
	req := httptest.NewRequest("DELETE", fmt.Sprintf(path, uid), nil)

	// record http response
	resp := httptest.NewRecorder()

	// assign http handler function
	r := chi.NewRouter()
	r.Delete("/product/{id}", pController.DeleteProduct)

	// dispatch the http request
	r.ServeHTTP(resp, req)

	// assert http status code
	checkResponseCode(t, http.StatusOK, resp.Code)

	// decode the http response
	var e serviceerr.ServiceError
	if err := json.NewDecoder(io.Reader(resp.Body)).Decode(&e); err != nil {
		log.Fatal(err)
	}

	assert.Equal(t, statusOK, e.Code)
	assert.Equal(t, pDeleted, e.Message)

	// clean up db
	tearDown(uid)
}

func TestUpdateProduct(t *testing.T) {
	uid := uuid.New()
	setupUUID(uid)

	const (
		path     = "/product/%v"
		newName  = "auto"
		newPrice = 999.9
	)

	data := entity.Product{
		ID:    uid,
		Name:  newName,
		Price: newPrice,
	}

	jsonReq, err := json.Marshal(data)
	if err != nil {
		log.Println(err)
	}

	// create a new http PUT request
	req := httptest.NewRequest("PUT", fmt.Sprintf(path, uid), bytes.NewBuffer(jsonReq))

	// record http response
	resp := httptest.NewRecorder()

	// assign http handler function
	r := chi.NewRouter()
	r.Put("/product/{id}", pController.UpdateProduct)

	r.ServeHTTP(resp, req)
	checkResponseCode(t, http.StatusOK, resp.Code)

	var p entity.Product
	if err := json.NewDecoder(io.Reader(resp.Body)).Decode(&p); err != nil {
		log.Fatal(err)
	}

	// assert http response
	assert.Equal(t, uid, p.ID)
	assert.Equal(t, newName, p.Name)
	assert.Equal(t, newPrice, p.Price)

	// clean up db
	tearDown(uid)
}

func checkResponseCode(t *testing.T, expected, actual int) {
	if expected != actual {
		t.Errorf("Expected response code %d. Got %d\n", expected, actual)
	}
}

func setupUUID(id uuid.UUID) {
	var p = entity.Product{
		ID:    id,
		Name:  NAME,
		Price: PRICE,
	}
	addProd(p)
}

func setup() {
	var p = entity.Product{
		Name:  NAME,
		Price: PRICE,
	}
	addProd(p)
}

func addProd(p entity.Product) {
	if _, err := pRepo.Save(&p); err != nil {
		log.Println(err)
	}
}

func tearDown(ID uuid.UUID) {
	if err := pRepo.Delete(ID); err != nil {
		log.Fatal(err)
	}
}
