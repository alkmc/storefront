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

// Server serves the gRPC API over the product service.
type Server struct {
	addr            string
	requestTimeout  time.Duration
	maxRequestBytes int
	shutdownTimeout time.Duration
	svc             processor
	log             *slog.Logger
}

// NewServer configures a gRPC server for svc.
func NewServer(
	addr string,
	requestTimeout time.Duration,
	maxRequestBytes int,
	shutdownTimeout time.Duration,
	svc processor,
	log *slog.Logger,
) *Server {
	return new(Server{
		addr:            addr,
		requestTimeout:  requestTimeout,
		maxRequestBytes: maxRequestBytes,
		shutdownTimeout: shutdownTimeout,
		svc:             svc,
		log:             log,
	})
}

// Run serves until ctx is cancelled, then stops gracefully.
func (s *Server) Run(ctx context.Context) error {
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", s.addr)
	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(logging(s.log), recovery(s.log), timeout(s.requestTimeout)),
		grpc.MaxRecvMsgSize(s.maxRequestBytes),
	)
	catalogv1.RegisterProductServiceServer(srv, NewHandler(s.svc, s.log))
	healthgrpc.RegisterHealthServer(srv, health.NewServer())

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		s.log.Info("starting grpc server", slog.String("address", s.addr))
		if err := srv.Serve(lis); err != nil {
			return fmt.Errorf("grpc serve failed: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		<-ctx.Done()
		s.log.Info("shutting down grpc server", slog.String("address", s.addr))
		force := time.AfterFunc(s.shutdownTimeout, srv.Stop)
		defer force.Stop()
		srv.GracefulStop()
		return nil
	})

	return eg.Wait()
}
