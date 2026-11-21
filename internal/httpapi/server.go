package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
)

// ServerCfg carries the settings an HTTP server needs.
type ServerCfg struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// Server runs an HTTP server and shuts it down gracefully on context cancellation.
type Server struct {
	srv             *http.Server
	shutdownTimeout time.Duration
	log             *slog.Logger
}

// NewPublicServer builds the public HTTP server from cfg.
func NewPublicServer(h http.Handler, cfg ServerCfg, log *slog.Logger) *Server {
	return new(Server{
		srv: &http.Server{
			Addr:         cfg.Addr,
			Handler:      h,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
		shutdownTimeout: cfg.ShutdownTimeout,
		log:             log,
	})
}

// NewInternalServer builds the internal probes server from cfg.
func NewInternalServer(h http.Handler, cfg ServerCfg, log *slog.Logger) *Server {
	return new(Server{
		srv: &http.Server{
			Addr:        cfg.Addr,
			Handler:     h,
			ReadTimeout: cfg.ReadTimeout,
		},
		shutdownTimeout: cfg.ShutdownTimeout,
		log:             log,
	})
}

// Run binds the configured address and serves until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", s.srv.Addr)
	if err != nil {
		return fmt.Errorf("server %s listen: %w", s.srv.Addr, err)
	}
	return s.Serve(ctx, lis)
}

// Serve takes ownership of lis and serves until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, lis net.Listener) error {
	addr := lis.Addr().String()
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		s.log.Info("starting server", slog.String("address", addr))
		if err := s.srv.Serve(lis); !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server %s serve: %w", addr, err)
		}
		return nil
	})
	eg.Go(func() error {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.shutdownTimeout)
		defer cancel()
		s.log.Info("shutting down server", slog.String("address", addr))
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			s.log.Warn("forcing server close", slog.String("address", addr), slog.Any("error", err))
			_ = s.srv.Close()
		}
		return nil
	})
	return eg.Wait()
}
