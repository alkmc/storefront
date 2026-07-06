package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alkmc/storefront/internal/cache"
	"github.com/alkmc/storefront/internal/config"
	"github.com/alkmc/storefront/internal/service"
	"github.com/alkmc/storefront/internal/store"
	grpcsrv "github.com/alkmc/storefront/internal/transport/grpc"
	httpsrv "github.com/alkmc/storefront/internal/transport/http"
	"github.com/alkmc/storefront/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
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

	pool, err := openPostgres(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	logger.Info("successfully connected to db")
	defer func() {
		pool.Close()
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

	repo := store.NewPostgres(pool)
	rCache := cache.New(client, cfg.Redis.TTL)
	srv := service.NewService(repo, rCache, cfg.Service.LoadTimeout, logger)

	mw, err := httpsrv.NewMiddleware(httpsrv.MiddlewareCfg{
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

	h := httpsrv.NewHandler(srv, cfg.HTTP.RequestTimeout, logger)
	apiSrv := httpsrv.NewAPIServer(
		mw(httpsrv.NewMux(h)),
		httpsrv.ServerCfg{
			Addr:            cfg.HTTP.Address(),
			ReadTimeout:     cfg.HTTP.ReadTimeout,
			WriteTimeout:    cfg.HTTP.WriteTimeout,
			IdleTimeout:     cfg.HTTP.IdleTimeout,
			ShutdownTimeout: cfg.ShutdownTimeout,
		},
		logger,
	)

	ih := httpsrv.NewInternalHandler(repo, rCache)
	internalSrv := httpsrv.NewInternalServer(
		httpsrv.NewInternalMux(ih),
		httpsrv.ServerCfg{
			Addr:            cfg.HTTP.InternalAddress(),
			ReadTimeout:     cfg.HTTP.ReadTimeout,
			ShutdownTimeout: cfg.ShutdownTimeout,
		},
		logger,
	)

	grpcSrv := grpcsrv.NewServer(
		cfg.GRPC.Address(), cfg.GRPC.RequestTimeout,
		int(cfg.GRPC.MaxRequestBytes), cfg.ShutdownTimeout, srv, logger,
	)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return apiSrv.Run(ctx) })
	eg.Go(func() error { return internalSrv.Run(ctx) })
	eg.Go(func() error { return grpcSrv.Run(ctx) })

	if err := eg.Wait(); err != nil {
		return err
	}
	logger.Info("server shutdown completed")
	return nil
}

// openPostgres opens a pgx connection pool and verifies it with a ping.
func openPostgres(ctx context.Context, cfg config.Postgres) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse pg config: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxOpenConns
	poolCfg.MinConns = cfg.MaxIdleConns
	poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pg pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping pg database: %w", err)
	}
	return pool, nil
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
