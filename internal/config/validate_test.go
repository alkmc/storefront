package config

import (
	"strings"
	"testing"
	"time"

	"github.com/caarlos0/env/v11"
)

const setting = "SETTING"

func TestPositive(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{
			name:    "negative duration",
			err:     positive(setting, -time.Second),
			wantErr: true,
		},
		{
			name:    "zero duration",
			err:     positive(setting, time.Duration(0)),
			wantErr: true,
		},
		{
			name: "smallest duration",
			err:  positive(setting, time.Nanosecond),
		},
		{
			name:    "negative int32",
			err:     positive(setting, int32(-1)),
			wantErr: true,
		},
		{
			name:    "zero int",
			err:     positive(setting, 0),
			wantErr: true,
		},
		{
			name: "positive int64",
			err:  positive(setting, int64(1)),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertRule(t, tc.err, tc.wantErr, setting)
		})
	}
}

func TestNotNegative(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{
			name:    "negative",
			err:     notNegative(setting, -1),
			wantErr: true,
		},
		{
			name: "zero is allowed",
			err:  notNegative(setting, 0),
		},
		{
			name: "positive",
			err:  notNegative(setting, int32(1)),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertRule(t, tc.err, tc.wantErr, setting)
		})
	}
}

func TestAtLeast(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{
			name:    "below the floor",
			err:     atLeast(setting, 500*time.Microsecond, time.Millisecond),
			wantErr: true,
		},
		{
			name: "on the floor",
			err:  atLeast(setting, time.Millisecond, time.Millisecond),
		},
		{
			name: "above the floor",
			err:  atLeast(setting, time.Second, time.Millisecond),
		},
		{
			name:    "negative below an int floor",
			err:     atLeast(setting, -1, 1),
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertRule(t, tc.err, tc.wantErr, setting)
		})
	}
}

func TestListenPort(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{
			name:    "zero",
			err:     listenPort(setting, 0),
			wantErr: true,
		},
		{
			name:    "negative",
			err:     listenPort(setting, -1),
			wantErr: true,
		},
		{
			name: "lowest",
			err:  listenPort(setting, 1),
		},
		{
			name: "highest",
			err:  listenPort(setting, maxPort),
		},
		{
			name:    "above the range",
			err:     listenPort(setting, maxPort+1),
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertRule(t, tc.err, tc.wantErr, setting)
		})
	}
}

func TestValidateConnLimits(t *testing.T) {
	tests := []struct {
		name       string
		idle, open int32
		wantErr    bool
	}{
		{
			name: "idle below open",
			idle: 5,
			open: 25,
		},
		{
			name: "idle equal to open",
			idle: 25,
			open: 25,
		},
		{
			name:    "idle above open",
			idle:    26,
			open:    25,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pg := Postgres{MaxIdleConns: tc.idle, MaxOpenConns: tc.open}
			assertRule(t, pg.validateConnLimits(), tc.wantErr, "PG_MAX_IDLE_CONNS", "PG_MAX_OPEN_CONNS")
		})
	}
}

func TestValidateListenAddresses(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{name: "distinct ports"},
		{
			name:    "internal port shares the api address",
			mutate:  func(c *Config) { c.HTTP.InternalPort = c.HTTP.Port },
			wantErr: true,
		},
		{
			name:    "grpc port shares the api address",
			mutate:  func(c *Config) { c.GRPC.Port = c.HTTP.Port },
			wantErr: true,
		},
		{
			// the check compares addresses, so one port on two hosts must stay legal
			name: "shared port on distinct hosts",
			mutate: func(c *Config) {
				c.HTTP.Host, c.GRPC.Host = "127.0.0.1", "127.0.0.2"
				c.GRPC.Port = c.HTTP.Port
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig(t)
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}
			assertRule(t, cfg.validateListenAddresses(), tc.wantErr, "HTTP_INTERNAL_PORT")
		})
	}
}

func TestValidateResponseDeadline(t *testing.T) {
	tests := []struct {
		name           string
		write, request time.Duration
		wantErr        bool
	}{
		{
			name:    "write beyond the budget",
			write:   10 * time.Second,
			request: 2 * time.Second,
		},
		{
			name:    "write equal to the budget",
			write:   2 * time.Second,
			request: 2 * time.Second,
			wantErr: true,
		},
		{
			name:    "write below the budget",
			write:   time.Second,
			request: 2 * time.Second,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := HTTP{WriteTimeout: tc.write, RequestTimeout: tc.request}
			assertRule(t, h.validateResponseDeadline(), tc.wantErr, "HTTP_WRITE_TIMEOUT", "HTTP_REQUEST_TIMEOUT")
		})
	}
}

func TestAuthValidate(t *testing.T) {
	tests := []struct {
		name    string
		secret  Secret
		wantErr bool
	}{
		{
			name:    "empty",
			wantErr: true,
		},
		{
			name:    "one byte short",
			secret:  Secret(strings.Repeat("k", minJWTSecretBytes-1)),
			wantErr: true,
		},
		{
			name:   "exactly the hash output size",
			secret: Secret(strings.Repeat("k", minJWTSecretBytes)),
		},
		{
			name:   "longer than required",
			secret: Secret(strings.Repeat("k", minJWTSecretBytes*2)),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertRule(t, Auth{JWTSecret: tc.secret}.validate(), tc.wantErr, "AUTH_JWT_SECRET")
		})
	}
}

// TestConfigValidateReachesEverySection proves every section hangs off Config.validate.
func TestConfigValidateReachesEverySection(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		setting string
	}{
		{
			name:    "HTTP",
			mutate:  func(c *Config) { c.HTTP.RequestTimeout = 0 },
			setting: "HTTP_REQUEST_TIMEOUT",
		},
		{
			name:    "GRPC",
			mutate:  func(c *Config) { c.GRPC.RequestTimeout = 0 },
			setting: "GRPC_REQUEST_TIMEOUT",
		},
		{
			name:    "Postgres",
			mutate:  func(c *Config) { c.Postgres.ConnMaxLifetime = 0 },
			setting: "PG_CONN_MAX_LIFETIME",
		},
		{
			name:    "Redis",
			mutate:  func(c *Config) { c.Redis.TTL = 0 },
			setting: "REDIS_CACHE_TTL",
		},
		{
			name:    "Outbox",
			mutate:  func(c *Config) { c.Outbox.BatchSize = 0 },
			setting: "OUTBOX_BATCH_SIZE",
		},
		{
			name:    "Service",
			mutate:  func(c *Config) { c.Service.LoadTimeout = 0 },
			setting: "SERVICE_LOAD_TIMEOUT",
		},
		{
			name:    "Idempotency",
			mutate:  func(c *Config) { c.Idempotency.TTL = 0 },
			setting: "IDEMPOTENCY_TTL",
		},
		{
			name:    "Auth",
			mutate:  func(c *Config) { c.Auth.JWTSecret = "" },
			setting: "AUTH_JWT_SECRET",
		},
		{
			name:    "shutdown timeout",
			mutate:  func(c *Config) { c.ShutdownTimeout = 0 },
			setting: "SHUTDOWN_TIMEOUT",
		},
		{
			name:    "response deadline",
			mutate:  func(c *Config) { c.HTTP.WriteTimeout = c.HTTP.RequestTimeout },
			setting: "HTTP_WRITE_TIMEOUT",
		},
		{
			name:    "connection limits",
			mutate:  func(c *Config) { c.Postgres.MaxIdleConns = c.Postgres.MaxOpenConns + 1 },
			setting: "PG_MAX_IDLE_CONNS",
		},
		{
			name:    "listen addresses",
			mutate:  func(c *Config) { c.GRPC.Port = c.HTTP.Port },
			setting: "GRPC_PORT",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig(t)
			tc.mutate(&cfg)
			assertRule(t, cfg.validate(), true, tc.setting)
		})
	}
}

// TestDefaultsAreValid proves every envDefault satisfies every rule, so a bare .env starts.
func TestDefaultsAreValid(t *testing.T) {
	if err := defaultConfig(t).validate(); err != nil {
		t.Fatalf("defaults rejected: %v", err)
	}
}

// TestConfigValidateAggregates proves errors.Join reports every violation in one run.
func TestConfigValidateAggregates(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.HTTP.RequestTimeout = 0
	cfg.Outbox.BatchSize = 0
	cfg.Idempotency.TTL = 0

	assertRule(
		t, cfg.validate(), true,
		"HTTP_REQUEST_TIMEOUT", "OUTBOX_BATCH_SIZE", "IDEMPOTENCY_TTL",
	)
}

// TestLoadRejectsEmptyRequired covers the notEmpty tags, which fail before validate runs.
func TestLoadRejectsEmptyRequired(t *testing.T) {
	for _, name := range []string{
		"PG_HOST", "PG_USER", "PG_DB", "REDIS_HOST", "RABBITMQ_HOST", "RABBITMQ_USER",
	} {
		t.Run(name, func(t *testing.T) {
			setRequired(t)
			t.Setenv(name, "")

			_, err := Load()
			if err == nil {
				t.Fatalf("expected an error naming %s, got nil", name)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error %q does not name %s", err, name)
			}
		})
	}
}

// TestLoadValidates covers the server binary, a parsable value can only fail in validate.
func TestLoadValidates(t *testing.T) {
	setRequired(t)
	t.Setenv("OUTBOX_BATCH_SIZE", "0")

	_, err := Load()
	assertRule(t, err, true, "OUTBOX_BATCH_SIZE")
}

// TestLoadPostgresValidates covers the migration binary, which loads the database settings alone.
func TestLoadPostgresValidates(t *testing.T) {
	setRequired(t)
	t.Setenv("PG_MAX_IDLE_CONNS", "50")
	t.Setenv("PG_MAX_OPEN_CONNS", "10")

	_, err := LoadPostgres()
	assertRule(t, err, true, "PG_MAX_IDLE_CONNS")
}

// TestLoadAuth covers the dev token binary, which holds the signing key to the same bar as the server.
func TestLoadAuth(t *testing.T) {
	t.Run("rejects a weak key", func(t *testing.T) {
		t.Setenv("AUTH_JWT_SECRET", "changeit")

		_, err := LoadAuth()
		assertRule(t, err, true, "AUTH_JWT_SECRET")
	})

	t.Run("returns a strong key", func(t *testing.T) {
		want := strings.Repeat("k", minJWTSecretBytes)
		t.Setenv("AUTH_JWT_SECRET", want)

		cfg, err := LoadAuth()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := cfg.JWTSecret.Reveal(); got != want {
			t.Errorf("secret %q, want %q", got, want)
		}
	})
}

// assertRule checks a rule outcome, and that a rejection names its setting and reports its value.
func assertRule(t *testing.T, err error, wantErr bool, settings ...string) {
	t.Helper()
	if !wantErr {
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected an error naming %v, got nil", settings)
	}
	for _, s := range settings {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("error %q does not name %s", err, s)
		}
	}
	if !strings.ContainsAny(err.Error(), "0123456789") {
		t.Errorf("error %q does not report the offending value", err)
	}
}

// defaultConfig parses a config from the required variables alone, so every other value is its default.
func defaultConfig(t *testing.T) Config {
	t.Helper()
	setRequired(t)

	cfg, err := env.ParseAs[Config]()
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return cfg
}

func setRequired(t *testing.T) {
	t.Helper()
	required := map[string]string{
		"PG_HOST":           "postgres",
		"PG_PORT":           "5432",
		"PG_USER":           "pguser",
		"PG_PASSWORD":       "secret",
		"PG_DB":             "storefront",
		"REDIS_HOST":        "redis",
		"REDIS_PORT":        "6379",
		"REDIS_PASSWORD":    "secret",
		"RABBITMQ_HOST":     "rabbitmq",
		"RABBITMQ_PORT":     "5672",
		"RABBITMQ_USER":     "guest",
		"RABBITMQ_PASSWORD": "secret",
		"AUTH_JWT_SECRET":   strings.Repeat("k", minJWTSecretBytes),
	}
	for k, v := range required {
		t.Setenv(k, v)
	}
}
