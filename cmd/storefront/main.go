package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alkmc/storefront/internal/auth"
	"github.com/alkmc/storefront/internal/bus"
	"github.com/alkmc/storefront/internal/cache"
	"github.com/alkmc/storefront/internal/config"
	"github.com/alkmc/storefront/internal/grpcapi"
	"github.com/alkmc/storefront/internal/httpapi"
	"github.com/alkmc/storefront/internal/outbox"
	"github.com/alkmc/storefront/internal/pg"
	"github.com/alkmc/storefront/internal/service"
	"github.com/alkmc/storefront/internal/store"
	"github.com/alkmc/storefront/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/rabbitmq-amqp-go-client/pkg/rabbitmqamqp"
	"github.com/valkey-io/valkey-go"
	"golang.org/x/sync/errgroup"
)

// brokerCloseTimeout keeps RabbitMQ closes within the stop grace period.
const brokerCloseTimeout = 5 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("application failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	slog.SetDefault(config.Log{}.NewLogger(os.Stdout))

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := cfg.Log.NewLogger(os.Stdout)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := migrations.Verify(ctx, cfg.Postgres.DSN()); err != nil {
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

	client, err := openValkey(ctx, cfg.Valkey)
	if err != nil {
		return err
	}
	logger.Info("successfully connected to valkey")
	defer func() {
		client.Close()
		logger.Info("connection to valkey closed")
	}()

	amqp, err := openRabbitMQ(ctx, cfg.RabbitMQ.URL())
	if err != nil {
		return err
	}
	logger.Info("successfully connected to rabbitmq")
	defer closeWithTimeout(ctx, brokerCloseTimeout, "rabbitmq connection", amqp.Close, logger)

	pub, err := bus.NewPublisher(ctx, amqp)
	if err != nil {
		return err
	}
	defer closeWithTimeout(ctx, brokerCloseTimeout, "event publisher", pub.Close, logger)

	repo := store.NewPostgres(pool, cfg.Idempotency.TTL)

	vCache := cache.New(client, cfg.Valkey.TTL, cfg.Valkey.NegTTL)
	svc := service.NewService(repo, vCache, cfg.Service.LoadTimeout, logger)

	ih := httpapi.NewInternalHandler(repo, vCache)

	listener := pg.NewListener(pool, store.OutboxChannel)
	relay := outbox.New(repo, listener, pub, outbox.Config{
		BatchSize:      cfg.Outbox.BatchSize,
		PollInterval:   cfg.Outbox.PollInterval,
		PublishTimeout: cfg.Outbox.PublishTimeout,
		MaxAttempts:    cfg.Outbox.MaxAttempts,
	}, logger)
	janitor := pg.NewJanitor(repo, cfg.Idempotency.PurgeInterval, logger)

	if err := serve(ctx, cfg, svc, ih, relay, janitor, logger); err != nil {
		return err
	}
	logger.Info("server shutdown completed")
	return nil
}

// serve runs the servers, the outbox relay and the janitor until the first failure or shutdown signal.
func serve(
	ctx context.Context,
	cfg config.Config,
	svc *service.Service,
	ih *httpapi.InternalHandler,
	relay *outbox.Relay,
	janitor *pg.Janitor,
	logger *slog.Logger,
) error {
	verifier := auth.NewVerifier(cfg.Auth.JWTSecret.Reveal())

	apiSrv, err := newAPIServer(cfg, svc, verifier, logger)
	if err != nil {
		return err
	}

	internalSrv := httpapi.NewInternalServer(
		httpapi.NewInternalMux(ih),
		httpapi.ServerCfg{
			Addr:            cfg.HTTP.InternalAddress(),
			ReadTimeout:     cfg.HTTP.ReadTimeout,
			ShutdownTimeout: cfg.ShutdownTimeout,
		},
		logger,
	)

	grpcSrv := grpcapi.NewServer(
		grpcapi.ServerCfg{
			Addr:            cfg.GRPC.Address(),
			RequestTimeout:  cfg.GRPC.RequestTimeout,
			MaxRequestBytes: int(cfg.GRPC.MaxRequestBytes),
			ShutdownTimeout: cfg.ShutdownTimeout,
			Reflection:      cfg.GRPC.Reflection,
		},
		svc, verifier, logger,
	)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return apiSrv.Run(ctx) })
	eg.Go(func() error { return internalSrv.Run(ctx) })
	eg.Go(func() error { return grpcSrv.Run(ctx) })
	eg.Go(func() error { return relay.Run(ctx) })
	eg.Go(func() error { return janitor.Run(ctx) })
	return eg.Wait()
}

func newAPIServer(
	cfg config.Config, svc *service.Service, v *auth.Verifier, l *slog.Logger,
) (*httpapi.Server, error) {
	mw, err := httpapi.NewMiddleware(httpapi.MiddlewareCfg{
		MaxBodyBytes:       cfg.HTTP.MaxBodyBytes,
		CompressMinBytes:   cfg.HTTP.CompressMinBytes,
		CORSAllowedOrigins: cfg.HTTP.CORSAllowedOrigins,
		CORSMaxAge:         cfg.HTTP.CORSMaxAge,
		HSTSEnabled:        cfg.HTTP.HSTSEnabled,
		HSTSMaxAge:         cfg.HTTP.HSTSMaxAge,
	})
	if err != nil {
		return nil, fmt.Errorf("create http middleware: %w", err)
	}

	h := httpapi.NewHandler(svc, cfg.HTTP.RequestTimeout, l)
	return httpapi.NewPublicServer(
		mw(httpapi.NewMux(h, httpapi.Auth(v))),
		httpapi.ServerCfg{
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
	var defaultConnOptions *rabbitmqamqp.AmqpConnOptions
	env := rabbitmqamqp.NewEnvironment(url, defaultConnOptions)
	conn, err := env.NewConnection(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect rabbitmq: %w", err)
	}
	return conn, nil
}

// openValkey creates a valkey-go client and verifies it with a ping.
func openValkey(ctx context.Context, cfg config.Valkey) (valkey.Client, error) {
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{cfg.Address()},
		Password:    cfg.Password.Reveal(),
		SelectDB:    cfg.DB,
	})
	if err != nil {
		return nil, fmt.Errorf("create valkey client: %w", err)
	}
	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping valkey: %w", err)
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
