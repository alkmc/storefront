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

	"github.com/alkmc/restClean/product/cache"
	"github.com/alkmc/restClean/product/entity"
	"github.com/alkmc/restClean/product/repository"
	"github.com/alkmc/restClean/product/service"
	"github.com/alkmc/restClean/product/validator"
	"github.com/alkmc/restClean/serviceerr"

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

	//Create a GET HTTP request
	req := httptest.NewRequest("GET", fmt.Sprintf(path, uid), nil)

	//Record HTTP Response (httptest)
	resp := httptest.NewRecorder()

	//Assign HTTP Handler function
	r := chi.NewRouter()
	r.Get("/product/{id}", pController.GetProductByID)

	//Dispatch the HTTP request
	r.ServeHTTP(resp, req)

	//Assert HTTP status code
	checkResponseCode(t, http.StatusOK, resp.Code)

	//Decode the HTTP response
	var product entity.Product
	if err := json.NewDecoder(io.Reader(resp.Body)).Decode(&product); err != nil {
		log.Fatal(err)
	}

	//Assert HTTP response
	assert.Equal(t, uid, product.ID)
	assert.Equal(t, NAME, product.Name)
	assert.Equal(t, PRICE, product.Price)

	//Clean up database
	tearDown(product.ID)
}
func TestGetProducts(t *testing.T) {
	//Insert new post
	setup()

	//Create a GET HTTP request
	req := httptest.NewRequest("GET", "/product", nil)

	//Record HTTP Response
	resp := httptest.NewRecorder()

	//Assign HTTP Handler function
	r := chi.NewRouter()
	r.Get("/product", pController.GetProducts)

	//Dispatch the HTTP request
	r.ServeHTTP(resp, req)

	//Assert HTTP status code
	checkResponseCode(t, http.StatusOK, resp.Code)

	//Decode the HTTP response
	var products []entity.Product
	if err := json.NewDecoder(io.Reader(resp.Body)).Decode(&products); err != nil {
		log.Fatal(err)
	}

	//Assert HTTP response
	assert.NotNil(t, products[0].ID)
	assert.Equal(t, NAME, products[0].Name)
	assert.Equal(t, PRICE, products[0].Price)

	//Clean up database
	tearDown(products[0].ID)

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
	//Create a new HTTP POST request
	req := httptest.NewRequest("POST", "/product", bytes.NewBuffer(jsonReq))

	//Record HTTP Response (httptest)
	resp := httptest.NewRecorder()

	//Assign HTTP Handler function
	r := chi.NewRouter()
	r.Post("/product", pController.AddProduct)

	//Dispatch the HTTP request
	r.ServeHTTP(resp, req)

	//Assert HTTP status code
	checkResponseCode(t, http.StatusCreated, resp.Code)

	//Decode the HTTP response
	var p entity.Product
	if err := json.NewDecoder(io.Reader(resp.Body)).Decode(&p); err != nil {
		log.Fatal(err)
	}

	//Assert HTTP response
	assert.Equal(t, uid, p.ID)
	assert.Equal(t, NAME, p.Name)
	assert.Equal(t, PRICE, p.Price)

	//Clean up database
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

	//Create a new HTTP DELETE request
	req := httptest.NewRequest("DELETE", fmt.Sprintf(path, uid), nil)

	//Record HTTP Response
	resp := httptest.NewRecorder()

	// Assign HTTP Handler function
	r := chi.NewRouter()
	r.Delete("/product/{id}", pController.DeleteProduct)

	//Dispatch the HTTP request
	r.ServeHTTP(resp, req)

	//Assert HTTP status code
	checkResponseCode(t, http.StatusOK, resp.Code)

	//Decode the HTTP response
	var err serviceerr.ServiceError
	if err := json.NewDecoder(io.Reader(resp.Body)).Decode(&err); err != nil {
		log.Fatal(err)
	}

	assert.Equal(t, statusOK, err.Code)
	assert.Equal(t, pDeleted, err.Message)

	//Clean up database
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
	req := httptest.NewRequest("PUT", fmt.Sprintf(path, uid), bytes.NewBuffer(jsonReq))

	//Record HTTP Response
	resp := httptest.NewRecorder()

	//Assign HTTP Handler function
	r := chi.NewRouter()
	r.Put("/product/{id}", pController.UpdateProduct)

	r.ServeHTTP(resp, req)
	checkResponseCode(t, http.StatusOK, resp.Code)

	var p entity.Product
	if err := json.NewDecoder(io.Reader(resp.Body)).Decode(&p); err != nil {
		log.Fatal(err)
	}

	//Assert HTTP response
	assert.Equal(t, uid, p.ID)
	assert.Equal(t, newName, p.Name)
	assert.Equal(t, newPrice, p.Price)

	// Clean up database
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
