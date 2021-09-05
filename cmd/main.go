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
	httpRouter        = router.NewChiRouter()
)

func main() {
	mapUrls()
	defer productRepository.CloseDB()
	httpRouter.SERVE(port)
}

func mapUrls() {
	httpRouter.POST("/product", productController.Add)
	httpRouter.GET("/product", productController.Get)
	httpRouter.GET("/product/{id}", productController.GetByID)
	httpRouter.PUT("/product/{id}", productController.Update)
	httpRouter.DELETE("/product/{id}", productController.Delete)
}
