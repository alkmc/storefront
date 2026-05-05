package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/alkmc/restClean/pkg/cache"
	"github.com/alkmc/restClean/pkg/httpapi"
	"github.com/alkmc/restClean/pkg/repository"
	"github.com/alkmc/restClean/pkg/service"
	"github.com/alkmc/restClean/pkg/validator"
)

const (
	redisDB         = 1
	cacheExpiration = 10 * time.Second
	redisHost       = "localhost:6379"
	port            = ":7000"
	readTimeout     = 5 * time.Second   // max time to read request from the client
	writeTimeout    = 10 * time.Second  // max time to write response to the client
	keepAlive       = 120 * time.Second // max time for connections using TCP Keep-Alive
	shutdownTimeout = 10 * time.Second  // max time to complete tasks before shutdown
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("application failed: %v", err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	productRepository := repository.NewPG() // can be set to repository.NewSQLite()
	defer productRepository.CloseDB()

	productService := service.NewService(productRepository)
	productCache := cache.NewRedis(redisHost, redisDB, cacheExpiration)
	productValidator := validator.NewValidator()
	productHandler := httpapi.NewHandler(productService, productCache, productValidator)

	s := &http.Server{
		Addr:         port,
		Handler:      httpapi.NewMux(productHandler),
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  keepAlive,
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		log.Print("signal closing server received")
		if err := s.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown failed: %v", err)
		}
	})

	log.Printf("starting http server on port %s", port)
	if err := s.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server listen failed: %w", err)
	}

	wg.Wait()
	log.Print("server shutdown completed")
	return nil
}
