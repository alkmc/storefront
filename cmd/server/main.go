package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/alkmc/restClean/internal/cache"
	"github.com/alkmc/restClean/internal/config"
	"github.com/alkmc/restClean/internal/httpapi"
	"github.com/alkmc/restClean/internal/migrate"
	"github.com/alkmc/restClean/internal/repository"
	"github.com/alkmc/restClean/internal/service"
	"golang.org/x/sync/errgroup"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "load config: %v\n", err)
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
	srv := service.NewService(logger, repo, rCache, cfg.Service.LoadTimeout)
	h := httpapi.NewHandler(logger, srv, cfg.HTTP.RequestTimeout)
	ih := httpapi.NewInternalHandler(repo, rCache)

	mw, err := httpapi.NewMiddleware(httpapi.MiddlewareCfg{
		MaxBodyBytes:       cfg.HTTP.MaxBodyBytes,
		CompressMinBytes:   cfg.HTTP.CompressMinBytes,
		CORSAllowedOrigins: cfg.HTTP.CORSAllowedOrigins,
		CORSMaxAge:         cfg.HTTP.CORSMaxAge,
		HSTSEnabled:        cfg.HTTP.HSTSEnabled,
		HSTSMaxAge:         cfg.HTTP.HSTSMaxAge,
	})
	if err != nil {
		return err
	}
	apiServer := httpapi.NewAPIServer(cfg.HTTP, mw(httpapi.NewMux(h)))
	internalServer := httpapi.NewInternalServer(cfg.HTTP, httpapi.NewInternalMux(ih))

	eg, ctx := errgroup.WithContext(ctx)
	serve := func(s *http.Server) {
		eg.Go(func() error {
			logger.Info("starting server", slog.String("address", s.Addr))
			if err := s.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("server %s listen failed: %w", s.Addr, err)
			}
			return nil
		})
		eg.Go(func() error {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.HTTP.ShutdownTimeout)
			defer cancel()
			logger.Info("shutting down server", slog.String("address", s.Addr))
			if err := s.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("server %s shutdown failed: %w", s.Addr, err)
			}
			return nil
		})
	}
	serve(apiServer)
	serve(internalServer)

	if err := eg.Wait(); err != nil {
		return err
	}
	logger.Info("server shutdown completed")
	return nil
}
