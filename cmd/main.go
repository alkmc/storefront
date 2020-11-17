package main

import (
	"time"

	"github.com/alkmc/restClean/pkg/cache"
	"github.com/alkmc/restClean/pkg/controller"
	"github.com/alkmc/restClean/pkg/repository"
	"github.com/alkmc/restClean/pkg/router"
	"github.com/alkmc/restClean/pkg/service"
	"github.com/alkmc/restClean/pkg/validator"
)

const (
	redisDB         = 1
	cacheExpiration = 10 * time.Second
	redisHost       = "localhost:6379"
	port            = ":7000"
)

var (
	productRepository = repository.NewPG() //can be set to repository.NewSQLite()
	productService    = service.NewService(productRepository)
	productCache      = cache.NewRedis(redisHost, redisDB, cacheExpiration)
	productValidator  = validator.NewValidator()
	productController = controller.NewController(productService, productCache, productValidator)
	productRouter     = router.NewChiRouter()
)

func main() {
	mapUrls()
	defer productRepository.CloseDB()
	productRouter.SERVE(port)
}

func mapUrls() {
	productRouter.POST("/product", productController.AddProduct)
	productRouter.GET("/product", productController.GetProducts)
	productRouter.GET("/product/{id}", productController.GetProductByID)
	productRouter.PUT("/product/{id}", productController.UpdateProduct)
	productRouter.DELETE("/product/{id}", productController.DeleteProduct)
}
