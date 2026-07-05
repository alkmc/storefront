package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/alkmc/storefront/internal/cache"
	"github.com/alkmc/storefront/internal/config"
	"github.com/alkmc/storefront/internal/httpapi"
	"github.com/alkmc/storefront/internal/migrate"
	"github.com/alkmc/storefront/internal/repository"
	"github.com/alkmc/storefront/internal/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/rueidis"
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

	db, err := openPostgres(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	logger.Info("successfully connected to db")
	defer func() {
		_ = db.Close()
		logger.Info("connection to db closed")
	}()

	client, err := openRedis(ctx, cfg.Redis)
	if err != nil {
		return err
	}
	logger.Info("successfully connected to redis")
	defer func() {
		client.Close()
		logger.Info("connection to redis closed")
	}()

	repo := repository.New(db)
	rCache := cache.New(client, cfg.Redis.TTL)
	srv := service.NewService(logger, repo, rCache, cfg.Service.LoadTimeout)

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

	h := httpapi.NewHandler(logger, srv, cfg.HTTP.RequestTimeout)
	apiServer := httpapi.NewAPIServer(
		mw(httpapi.NewMux(h)),
		httpapi.ServerCfg{
			Addr:         cfg.HTTP.Address(),
			ReadTimeout:  cfg.HTTP.ReadTimeout,
			WriteTimeout: cfg.HTTP.WriteTimeout,
			IdleTimeout:  cfg.HTTP.IdleTimeout,
		},
	)

	ih := httpapi.NewInternalHandler(repo, rCache)
	internalServer := httpapi.NewInternalServer(
		httpapi.NewInternalMux(ih),
		httpapi.ServerCfg{
			Addr:        cfg.HTTP.InternalAddress(),
			ReadTimeout: cfg.HTTP.ReadTimeout,
		},
	)

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

// openPostgres opens a database/sql pool over pgx and verifies it with a ping.
func openPostgres(ctx context.Context, cfg config.Postgres) (*sql.DB, error) {
	pgCfg, err := pgx.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse pg config: %w", err)
	}
	db := stdlib.OpenDB(*pgCfg)
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping pg database: %w", err)
	}
	return db, nil
}

// openRedis creates a rueidis client and verifies it with a ping.
func openRedis(ctx context.Context, cfg config.Redis) (rueidis.Client, error) {
	client, err := rueidis.NewClient(rueidis.ClientOption{
		InitAddress: []string{cfg.Address()},
		Password:    cfg.Password.Reveal(),
		SelectDB:    cfg.DB,
	})
	if err != nil {
		return nil, fmt.Errorf("create redis client: %w", err)
	}
	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
