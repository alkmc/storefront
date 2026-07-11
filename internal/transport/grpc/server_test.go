package grpc

import (
	"log/slog"
	"testing"
)

const reflectionService = "grpc.reflection.v1.ServerReflection"

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
			s := NewServer(ServerCfg{Reflection: tc.enabled}, stubProcessor{}, slog.New(slog.DiscardHandler))
			_, got := s.newServer().GetServiceInfo()[reflectionService]
			if got != tc.enabled {
				t.Errorf("reflection service registered = %v, want %v", got, tc.enabled)
			}
		})
	}
}
