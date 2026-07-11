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
	"google.golang.org/grpc/reflection"
)

type (
	// ServerCfg carries the gRPC server settings.
	ServerCfg struct {
		Addr            string
		RequestTimeout  time.Duration
		MaxRequestBytes int
		ShutdownTimeout time.Duration
		// Reflection enables server reflection for CLI tooling; dev only.
		Reflection bool
	}
	// Server serves the gRPC API over the product service.
	Server struct {
		cfg ServerCfg
		svc processor
		log *slog.Logger
	}
)

// NewServer configures a gRPC server for svc.
func NewServer(cfg ServerCfg, svc processor, log *slog.Logger) *Server {
	return new(Server{cfg: cfg, svc: svc, log: log})
}

// Run serves until ctx is cancelled, then stops gracefully.
func (s *Server) Run(ctx context.Context) error {
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}

	srv := s.newServer()

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		s.log.Info("starting grpc server", slog.String("address", s.cfg.Addr))
		if err := srv.Serve(lis); err != nil {
			return fmt.Errorf("grpc serve failed: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		<-ctx.Done()
		s.log.Info("shutting down grpc server", slog.String("address", s.cfg.Addr))
		force := time.AfterFunc(s.cfg.ShutdownTimeout, srv.Stop)
		defer force.Stop()
		srv.GracefulStop()
		return nil
	})

	return eg.Wait()
}

// newServer assembles the grpc.Server with interceptors and registered services.
func (s *Server) newServer() *grpc.Server {
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(logging(s.log), recovery(s.log), timeout(s.cfg.RequestTimeout)),
		grpc.MaxRecvMsgSize(s.cfg.MaxRequestBytes),
	)
	catalogv1.RegisterProductServiceServer(srv, NewHandler(s.svc, s.log))
	healthgrpc.RegisterHealthServer(srv, health.NewServer())
	if s.cfg.Reflection {
		reflection.Register(srv)
		s.log.Info("grpc reflection enabled")
	}
	return srv
}
