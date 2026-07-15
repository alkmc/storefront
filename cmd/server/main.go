package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alkmc/storefront/internal/bus"
	"github.com/alkmc/storefront/internal/cache"
	"github.com/alkmc/storefront/internal/config"
	"github.com/alkmc/storefront/internal/outbox"
	"github.com/alkmc/storefront/internal/service"
	"github.com/alkmc/storefront/internal/store"
	grpcsrv "github.com/alkmc/storefront/internal/transport/grpc"
	httpsrv "github.com/alkmc/storefront/internal/transport/http"
	"github.com/alkmc/storefront/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/rabbitmq-amqp-go-client/pkg/rabbitmqamqp"
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
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
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

	amqp, err := openRabbitMQ(ctx, cfg.RabbitMQ.URL())
	if err != nil {
		return err
	}
	logger.Info("successfully connected to rabbitmq")
	defer closeWithTimeout(ctx, cfg.ShutdownTimeout, "rabbitmq connection", amqp.Close, logger)

	pub, err := bus.NewPublisher(ctx, amqp)
	if err != nil {
		return err
	}
	defer closeWithTimeout(ctx, cfg.ShutdownTimeout, "event publisher", pub.Close, logger)

	repo := store.NewPostgres(pool)

	rCache := cache.New(client, cfg.Redis.TTL, cfg.Redis.NegTTL)
	srv := service.NewService(repo, rCache, cfg.Service.LoadTimeout, logger)

	apiSrv, err := newAPIServer(cfg, srv, logger)
	if err != nil {
		return err
	}

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
		grpcsrv.ServerCfg{
			Addr:            cfg.GRPC.Address(),
			RequestTimeout:  cfg.GRPC.RequestTimeout,
			MaxRequestBytes: int(cfg.GRPC.MaxRequestBytes),
			ShutdownTimeout: cfg.ShutdownTimeout,
			Reflection:      cfg.GRPC.Reflection,
		},
		srv, logger,
	)

	listener := store.NewListener(pool, store.OutboxChannel)
	relay := outbox.New(repo, listener, pub, outbox.Config{
		BatchSize:      cfg.Outbox.BatchSize,
		PollInterval:   cfg.Outbox.PollInterval,
		PublishTimeout: cfg.Outbox.PublishTimeout,
		MaxAttempts:    cfg.Outbox.MaxAttempts,
	}, logger)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return apiSrv.Run(ctx) })
	eg.Go(func() error { return internalSrv.Run(ctx) })
	eg.Go(func() error { return grpcSrv.Run(ctx) })
	eg.Go(func() error { return relay.Run(ctx) })

	if err := eg.Wait(); err != nil {
		return err
	}
	logger.Info("server shutdown completed")
	return nil
}

func newAPIServer(cfg config.Config, svc *service.Service, l *slog.Logger,
) (*httpsrv.Server, error) {
	mw, err := httpsrv.NewMiddleware(httpsrv.MiddlewareCfg{
		MaxBodyBytes:       cfg.HTTP.MaxBodyBytes,
		CompressMinBytes:   cfg.HTTP.CompressMinBytes,
		CORSAllowedOrigins: cfg.HTTP.CORSAllowedOrigins,
		CORSMaxAge:         cfg.HTTP.CORSMaxAge,
		HSTSEnabled:        cfg.HTTP.HSTSEnabled,
		HSTSMaxAge:         cfg.HTTP.HSTSMaxAge,
	})
	if err != nil {
		return nil, err
	}

	h := httpsrv.NewHandler(svc, cfg.HTTP.RequestTimeout, l)
	return httpsrv.NewAPIServer(
		mw(httpsrv.NewMux(h)),
		httpsrv.ServerCfg{
			Addr:            cfg.HTTP.Address(),
			ReadTimeout:     cfg.HTTP.ReadTimeout,
			WriteTimeout:    cfg.HTTP.WriteTimeout,
			IdleTimeout:     cfg.HTTP.IdleTimeout,
			ShutdownTimeout: cfg.ShutdownTimeout,
		},
		l,
	), nil
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

// openRabbitMQ connects to the broker through the client's auto-recovering environment.
func openRabbitMQ(ctx context.Context, url string) (*rabbitmqamqp.AmqpConnection, error) {
	env := rabbitmqamqp.NewEnvironment(url, nil)
	conn, err := env.NewConnection(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect rabbitmq: %w", err)
	}
	return conn, nil
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

// closeWithTimeout closes a resource with a fresh timeout detached from the canceled run context.
func closeWithTimeout(
	ctx context.Context, d time.Duration, name string, closeFn func(context.Context) error, l *slog.Logger,
) {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), d)
	defer cancel()
	if err := closeFn(closeCtx); err != nil {
		l.Warn("close failed", slog.String("resource", name), slog.Any("error", err))
		return
	}
	l.Info("closed", slog.String("resource", name))
}
