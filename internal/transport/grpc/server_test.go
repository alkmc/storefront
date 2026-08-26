package grpc

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/alkmc/storefront/internal/auth"
)

const (
	reflectionService = "grpc.reflection.v1.ServerReflection"
	// Serve and GracefulStop race roughly evenly, so one run proves nothing.
	cancelRaceRuns = 200
)

func TestServer_Reflection(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{
			name:    "enabled registers reflection service",
			enabled: true,
		},
		{
			name:    "disabled omits reflection service",
			enabled: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer(
				ServerCfg{Reflection: tc.enabled},
				stubProcessor{},
				auth.NewVerifier(testJWTSecret),
				slog.New(slog.DiscardHandler),
			)
			_, got := s.newServer().GetServiceInfo()[reflectionService]
			if got != tc.enabled {
				t.Errorf("reflection service registered = %v, want %v", got, tc.enabled)
			}
		})
	}
}

func TestServer_RunCancelledBeforeServe(t *testing.T) {
	cfg := ServerCfg{
		Addr:            "127.0.0.1:0",
		RequestTimeout:  time.Second,
		MaxRequestBytes: 1 << 20,
		ShutdownTimeout: time.Second,
	}
	log := slog.New(slog.DiscardHandler)

	for range cancelRaceRuns {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		srv := NewServer(cfg, stubProcessor{}, auth.NewVerifier(testJWTSecret), log)
		if err := srv.Run(ctx); err != nil {
			t.Fatalf("Run() = %v, want nil on shutdown", err)
		}
	}
}

func TestServer_RunListenError(t *testing.T) {
	srv := NewServer(
		ServerCfg{Addr: "127.0.0.1:99999"},
		stubProcessor{},
		auth.NewVerifier(testJWTSecret),
		slog.New(slog.DiscardHandler),
	)
	if err := srv.Run(t.Context()); err == nil {
		t.Fatal("Run() = nil, want error for an invalid address")
	}
}
