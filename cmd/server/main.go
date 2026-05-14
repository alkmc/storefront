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

	"github.com/alkmc/restClean/internal/cache"
	"github.com/alkmc/restClean/internal/config"
	"github.com/alkmc/restClean/internal/httpapi"
	"github.com/alkmc/restClean/internal/migrate"
	"github.com/alkmc/restClean/internal/repository"
	"github.com/alkmc/restClean/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger := cfg.Log.NewLogger(os.Stdout)
	slog.SetDefault(logger)

	if err := run(logger, cfg); err != nil {
		logger.Error("application failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(logger *slog.Logger, cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := migrate.Verify(ctx, cfg.Postgres.DSN()); err != nil {
		return err
	}

	repo, err := repository.NewPG(ctx, logger, cfg.Postgres)
	if err != nil {
		return err
	}
	defer repo.Close()

	rCache, err := cache.NewRedis(ctx, cfg.Redis)
	if err != nil {
		return err
	}
	logger.Info("successfully connected to redis")
	defer func() {
		rCache.Close()
		logger.Info("connection to redis closed")
	}()
	srv := service.NewService(logger, repo, rCache)
	h := httpapi.NewHandler(logger, srv, cfg.HTTP.RequestTimeout)
	ih := httpapi.NewInternalHandler(repo, rCache)

	apiServer := httpapi.NewAPIServer(cfg.HTTP, httpapi.NewMux(logger, h))
	internalServer := httpapi.NewInternalServer(cfg.HTTP, httpapi.NewInternalMux(ih))

	var wg sync.WaitGroup
	wg.Go(func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.HTTP.ShutdownTimeout)
		defer cancel()
		logger.Info("signal closing server received")
		if err := apiServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("api server shutdown failed", slog.Any("error", err))
		}
		if err := internalServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("internal server shutdown failed", slog.Any("error", err))
		}
	})
	wg.Go(func() {
		logger.Info("starting internal server", slog.String("address", cfg.HTTP.InternalAddress()))
		if err := internalServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("internal server failed", slog.Any("error", err))
		}
	})

	logger.Info("starting http server", slog.String("address", cfg.HTTP.Address()))
	if err := apiServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server listen failed: %w", err)
	}

	wg.Wait()
	logger.Info("server shutdown completed")
	return nil
}
