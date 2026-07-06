package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// NewAPIServer builds the public HTTP server from cfg.
func NewAPIServer(h http.Handler, cfg ServerCfg, log *slog.Logger) *Server {
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

// Run serves until ctx is cancelled, then shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		s.log.Info("starting server", slog.String("address", s.srv.Addr))
		if err := s.srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server %s listen failed: %w", s.srv.Addr, err)
		}
		return nil
	})
	eg.Go(func() error {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.shutdownTimeout)
		defer cancel()
		s.log.Info("shutting down server", slog.String("address", s.srv.Addr))
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server %s shutdown failed: %w", s.srv.Addr, err)
		}
		return nil
	})
	return eg.Wait()
}
