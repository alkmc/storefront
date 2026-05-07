package config

import (
	"maps"
	"strings"
	"testing"
	"time"
)

func TestLoadUsesDefaultsAndRequiredPostgresEnv(t *testing.T) {
	cfg, err := load(lookupFrom(map[string]string{
		"PG_HOST":     "db",
		"PG_PORT":     "5432",
		"PG_USER":     "app",
		"PG_PASSWORD": "secret",
		"PG_DB":       "products",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := cfg.HTTP.Address(), ":7000"; got != want {
		t.Fatalf("got HTTP address %q, want %q", got, want)
	}
	if got, want := cfg.HTTP.ReadTimeout, 5*time.Second; got != want {
		t.Fatalf("got read timeout %s, want %s", got, want)
	}
	if got, want := cfg.HTTP.RequestTimeout, 2*time.Second; got != want {
		t.Fatalf("got request timeout %s, want %s", got, want)
	}
	if got, want := cfg.Redis.Address(), "localhost:6379"; got != want {
		t.Fatalf("got Redis address %q, want %q", got, want)
	}
	if got, want := cfg.Redis.DB, 0; got != want {
		t.Fatalf("got Redis DB %d, want %d", got, want)
	}
	if got, want := cfg.Redis.TTL, 10*time.Second; got != want {
		t.Fatalf("got Redis TTL %s, want %s", got, want)
	}
	if !strings.Contains(cfg.Postgres.DSN(), "sslmode=disable") {
		t.Fatalf("DSN does not contain default sslmode: %q", cfg.Postgres.DSN())
	}
}

func TestLoadAllowsEnvOverrides(t *testing.T) {
	cfg, err := load(lookupFrom(map[string]string{
		"HTTP_HOST":             "127.0.0.1",
		"HTTP_PORT":             "8080",
		"HTTP_READ_TIMEOUT":     "1s",
		"HTTP_WRITE_TIMEOUT":    "2s",
		"HTTP_IDLE_TIMEOUT":     "3s",
		"HTTP_SHUTDOWN_TIMEOUT": "4s",
		"HTTP_REQUEST_TIMEOUT":  "500ms",
		"PG_HOST":               "db.internal",
		"PG_PORT":               "15432",
		"PG_USER":               "app",
		"PG_PASSWORD":           "p@ss word",
		"PG_DB":                 "products",
		"PG_SSLMODE":            "require",
		"REDIS_HOST":            "redis.internal",
		"REDIS_PORT":            "16379",
		"REDIS_DB":              "2",
		"REDIS_CACHE_TTL":       "30s",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := cfg.HTTP.Address(), "127.0.0.1:8080"; got != want {
		t.Fatalf("got HTTP address %q, want %q", got, want)
	}
	if got, want := cfg.HTTP.RequestTimeout, 500*time.Millisecond; got != want {
		t.Fatalf("got request timeout %s, want %s", got, want)
	}
	if got, want := cfg.Redis.Address(), "redis.internal:16379"; got != want {
		t.Fatalf("got Redis address %q, want %q", got, want)
	}
	if got, want := cfg.Redis.DB, 2; got != want {
		t.Fatalf("got Redis DB %d, want %d", got, want)
	}
	if got, want := cfg.Redis.TTL, 30*time.Second; got != want {
		t.Fatalf("got Redis TTL %s, want %s", got, want)
	}

	dsn := cfg.Postgres.DSN()
	for _, want := range []string{
		"postgres://app:p%40ss%20word@db.internal:15432/products",
		"sslmode=require",
	} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("DSN %q does not contain %q", dsn, want)
		}
	}
}

func TestLoadRequiresPostgresEnv(t *testing.T) {
	_, err := load(lookupFrom(map[string]string{}))
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), `environment variable "PG_HOST" is required`; got != want {
		t.Fatalf("got error %q, want %q", got, want)
	}
}

func TestLoadReturnsParseErrors(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "invalid duration",
			env: map[string]string{
				"HTTP_READ_TIMEOUT": "soon",
			},
			want: "parse HTTP_READ_TIMEOUT",
		},
		{
			name: "invalid int",
			env: map[string]string{
				"REDIS_DB": "one",
			},
			want: "parse REDIS_DB",
		},
		{
			name: "negative int",
			env: map[string]string{
				"REDIS_DB": "-1",
			},
			want: "REDIS_DB must be non-negative",
		},
		{
			name: "non-positive duration",
			env: map[string]string{
				"REDIS_CACHE_TTL": "0s",
			},
			want: "REDIS_CACHE_TTL must be positive",
		},
		{
			name: "invalid port",
			env: map[string]string{
				"HTTP_PORT": "http",
			},
			want: "parse HTTP_PORT",
		},
		{
			name: "zero port",
			env: map[string]string{
				"REDIS_PORT": "0",
			},
			want: "parse REDIS_PORT",
		},
		{
			name: "port out of range",
			env: map[string]string{
				"PG_PORT": "65536",
			},
			want: "parse PG_PORT",
		},
	}

	base := map[string]string{
		"PG_HOST":     "db",
		"PG_PORT":     "5432",
		"PG_USER":     "app",
		"PG_PASSWORD": "secret",
		"PG_DB":       "products",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{}
			maps.Copy(env, base)
			maps.Copy(env, tt.env)

			_, err := load(lookupFrom(env))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got error %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

func lookupFrom(env map[string]string) lookupFunc {
	return func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}
}
