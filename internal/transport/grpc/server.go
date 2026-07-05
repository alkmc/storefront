package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	catalogv1 "github.com/alkmc/storefront/api/gen/catalog/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

// Run serves the gRPC API until ctx is cancelled, then stops gracefully.
func Run(
	ctx context.Context,
	addr string,
	shutdownTimeout time.Duration,
	maxRecvBytes int,
	requestTimeout time.Duration,
	svc processor,
	log *slog.Logger,
) error {
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(logging(log), recovery(log), timeout(requestTimeout)),
		grpc.MaxRecvMsgSize(maxRecvBytes),
	)
	catalogv1.RegisterProductServiceServer(srv, NewHandler(log, svc))
	healthgrpc.RegisterHealthServer(srv, health.NewServer())

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		log.Info("starting grpc server", slog.String("address", addr))
		if err := srv.Serve(lis); err != nil {
			return fmt.Errorf("grpc serve failed: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		<-ctx.Done()
		log.Info("shutting down grpc server", slog.String("address", addr))
		force := time.AfterFunc(shutdownTimeout, srv.Stop)
		defer force.Stop()
		srv.GracefulStop()
		return nil
	})

	return eg.Wait()
}
