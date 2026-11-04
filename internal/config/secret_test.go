package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// TestSecretRedaction guards the value receivers on Secret, pointer ones let fmt and slog print it raw.
func TestSecretRedaction(t *testing.T) {
	const (
		pgPassword       = "pg-password"
		valkeyPassword   = "valkey-password"
		rabbitMQPassword = "rabbitmq-password"
		jwtSecret        = "jwt-secret"
	)
	plain := []string{pgPassword, valkeyPassword, rabbitMQPassword, jwtSecret}
	cfg := Config{
		Postgres: Postgres{Password: pgPassword},
		Valkey:   Valkey{Password: valkeyPassword},
		RabbitMQ: RabbitMQ{Password: rabbitMQPassword},
		Auth:     Auth{JWTSecret: jwtSecret},
	}

	tests := []struct {
		name   string
		render func() string
	}{
		{
			name:   "fmt config",
			render: func() string { return fmt.Sprintf("%+v", cfg) },
		},
		{
			name:   "fmt go syntax config",
			render: func() string { return fmt.Sprintf("%#v", cfg) },
		},
		{
			name:   "fmt mistyped verb secret",
			render: func() string { return fmt.Sprintf("%d", cfg.Postgres.Password) },
		},
		{
			name: "json logger config",
			render: func() string {
				var buf bytes.Buffer
				Log{}.NewLogger(&buf).Info("config", slog.Any("config", cfg))
				return buf.String()
			},
		},
		{
			name: "json logger secret",
			render: func() string {
				var buf bytes.Buffer
				Log{}.NewLogger(&buf).Info("secret", slog.Any("password", cfg.Postgres.Password))
				return buf.String()
			},
		},
		{
			name: "text logger secret",
			render: func() string {
				var buf bytes.Buffer
				slog.New(slog.NewTextHandler(&buf, nil)).Info("secret", slog.Any("password", cfg.Postgres.Password))
				return buf.String()
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := tc.render()
			for _, p := range plain {
				if strings.Contains(out, p) {
					t.Errorf("output leaks %q: %s", p, out)
				}
			}
			if !strings.Contains(out, redacted) {
				t.Errorf("output lacks %q: %s", redacted, out)
			}
		})
	}
}
