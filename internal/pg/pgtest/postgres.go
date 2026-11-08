//go:build integration

// Package pgtest starts throwaway Postgres containers for integration tests.
package pgtest

import (
	"testing"

	"github.com/alkmc/storefront/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	// keep in sync with compose.yaml
	postgresImage = "postgres:18"

	dbName     = "testdb"
	dbUser     = "testuser"
	dbPassword = "testpassword"
)

// Pool starts a Postgres container and returns a ready pool on an empty database.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return open(t, run(t))
}

// MigratedPool starts a Postgres container with the migrations applied.
func MigratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := run(t)
	migrate(t, dsn)
	return open(t, dsn)
}

// run starts the container, terminates it on cleanup and returns its DSN.
func run(t *testing.T) string {
	t.Helper()
	ctx := t.Context()

	container, err := postgres.Run(
		ctx,
		postgresImage,
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("run postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("terminate pg container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("container connection string: %v", err)
	}

	return dsn
}

// migrate applies the full schema over a short lived database/sql handle, goose needs one.
func migrate(t *testing.T, dsn string) {
	t.Helper()

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse pg config: %v", err)
	}
	db := stdlib.OpenDB(*cfg)
	if _, err := migrations.Up(t.Context(), db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close migration db: %v", err)
	}
}

// open returns a pool that is closed on cleanup, before the container goes down.
func open(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("create pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(t.Context()); err != nil {
		t.Fatalf("ping db: %v", err)
	}

	return pool
}
