package config

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresDSN(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Postgres
		wantTLS bool
	}{
		{
			name: "plain values",
			cfg: Postgres{
				Host:     "localhost",
				Port:     5432,
				User:     "storefront",
				Password: "secret",
				Database: "storefront",
				SSLMode:  "disable",
			},
		},
		{
			name: "reserved characters",
			cfg: Postgres{
				Host:     "db.internal",
				Port:     6543,
				User:     "app:user@x",
				Password: "@:/?#%",
				Database: "store/front",
				SSLMode:  "disable",
			},
		},
		{
			name: "ipv6 host",
			cfg: Postgres{
				Host:     "::1",
				Port:     5432,
				User:     "storefront",
				Password: "secret",
				Database: "storefront",
				SSLMode:  "disable",
			},
		},
		{
			name: "tls required",
			cfg: Postgres{
				Host:     "localhost",
				Port:     5432,
				User:     "storefront",
				Password: "secret",
				Database: "storefront",
				SSLMode:  "require",
			},
			wantTLS: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pgconn.ParseConfig(tc.cfg.DSN())
			if err != nil {
				t.Fatalf("parse DSN: %v", err)
			}
			if got.Host != tc.cfg.Host {
				t.Errorf("host = %q, want %q", got.Host, tc.cfg.Host)
			}
			if int(got.Port) != tc.cfg.Port {
				t.Errorf("port = %d, want %d", got.Port, tc.cfg.Port)
			}
			if got.User != tc.cfg.User {
				t.Errorf("user = %q, want %q", got.User, tc.cfg.User)
			}
			if got.Password != tc.cfg.Password.Reveal() {
				t.Errorf("password does not round-trip")
			}
			if got.Database != tc.cfg.Database {
				t.Errorf("database = %q, want %q", got.Database, tc.cfg.Database)
			}
			if hasTLS := got.TLSConfig != nil; hasTLS != tc.wantTLS {
				t.Errorf("TLS enabled = %v, want %v", hasTLS, tc.wantTLS)
			}
		})
	}
}
