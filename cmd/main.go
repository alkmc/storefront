package main

import (
	"time"

	"github.com/alkmc/restClean/product/cache"
	"github.com/alkmc/restClean/product/controller"
	"github.com/alkmc/restClean/product/repository"
	"github.com/alkmc/restClean/product/router"
	"github.com/alkmc/restClean/product/service"
)

const (
	redisDB         = 1
	cacheExpiration = 10 * time.Second
	redisHost       = "localhost:6379"
)

var (
	productRepository = repository.NewPG() //can be set to repository.NewSQLite()
	productService    = service.NewService(productRepository)
	productCache      = cache.NewRedis(redisHost, redisDB, cacheExpiration)
	productController = controller.NewController(productService, productCache)
	productRouter     = router.NewChiRouter()
)

func main() {
	mapUrls()
	productRouter.SERVE(":7000")
}

func mapUrls() {
	productRouter.POST("/product", productController.AddProduct)
	productRouter.GET("/product", productController.GetProducts)
	productRouter.GET("/product/{id}", productController.GetProductByID)
	productRouter.PUT("/product/{id}", productController.UpdateProduct)
	productRouter.DELETE("/product/{id}", productController.DeleteProduct)
}
