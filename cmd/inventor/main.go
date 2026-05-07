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

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if err := migrate.Verify(ctx, cfg.Postgres.DSN()); err != nil {
		return err
	}

	repo, err := repository.NewPG(ctx, logger, cfg.Postgres.DSN())
	if err != nil {
		return err
	}
	defer repo.CloseDB()

	srv := service.NewService(repo)
	rCache, err := cache.NewRedis(ctx, cfg.Redis.Address(), cfg.Redis.DB, cfg.Redis.TTL)
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
	h := httpapi.NewHandler(logger, srv, rCache, cfg.HTTP.RequestTimeout)

	s := new(http.Server{
		Addr:         cfg.HTTP.Address(),
		Handler:      httpapi.NewMux(logger, h),
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	})

	var wg sync.WaitGroup
	wg.Go(func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()

		logger.Info("signal closing server received")
		if err := s.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", slog.Any("error", err))
		}
	})

	logger.Info("starting http server", slog.String("address", cfg.HTTP.Address()))
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
