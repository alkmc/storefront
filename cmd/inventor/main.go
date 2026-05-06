package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/alkmc/restClean/internal/cache"
	"github.com/alkmc/restClean/internal/httpapi"
	"github.com/alkmc/restClean/internal/repository"
	"github.com/alkmc/restClean/internal/service"
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
	logger := setupLogger()
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("application failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	repo, err := repository.NewPG(ctx, logger)
	if err != nil {
		return err
	}
	defer repo.CloseDB()

	srv := service.NewService(repo)
	rCache, err := cache.NewRedis(ctx, redisHost, redisDB, cacheExpiration)
	if err != nil {
		return err
	}
	logger.Info("successfully connected to redis")
	defer func() {
		if err := rCache.Close(); err != nil {
			logger.Error("failed to close redis cache", slog.Any("error", err))
			return
		}
		logger.Info("connection to redis closed")
	}()
	h := httpapi.NewHandler(logger, srv, rCache)

	s := new(http.Server{
		Addr:         port,
		Handler:      httpapi.NewMux(logger, h),
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  keepAlive,
	})

	var wg sync.WaitGroup
	wg.Go(func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		logger.Info("signal closing server received")
		if err := s.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", slog.Any("error", err))
		}
	})

	logger.Info("starting http server", slog.String("port", port))
	if err := s.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server listen failed: %w", err)
	}

	wg.Wait()
	logger.Info("server shutdown completed")
	return nil
}

func setupLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Value.Kind() == slog.KindDuration {
				return slog.String(a.Key, fmt.Sprintf("%dms", a.Value.Duration().Milliseconds()))
			}
			return a
		},
	}))
}
