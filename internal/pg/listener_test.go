//go:build integration

package pg

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testChannel is a self-contained NOTIFY channel, the listener needs no schema.
const testChannel = "pg_test_channel"

func TestListener_BuffersNotifyBetweenWaits(t *testing.T) {
	pool := setupTestPool(t)
	ctx := t.Context()

	l := NewListener(pool, testChannel)
	defer l.Close()

	// The first wait times out and leaves the LISTEN connection subscribed.
	if err := l.Await(ctx, 50*time.Millisecond); err != nil {
		t.Fatalf("await before notify: %v", err)
	}

	if _, err := pool.Exec(ctx, "SELECT pg_notify('"+testChannel+"', '')"); err != nil {
		t.Fatalf("notify: %v", err)
	}

	// A NOTIFY sent between waits is buffered on the held connection, so a long wait returns at once.
	start := time.Now()
	if err := l.Await(ctx, 30*time.Second); err != nil {
		t.Fatalf("await after notify: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("await took %v, want a buffered NOTIFY to end it quickly", elapsed)
	}

	// Close drops the connection and the next wait re-acquires a fresh one.
	l.Close()
	if err := l.Await(ctx, 50*time.Millisecond); err != nil {
		t.Fatalf("await after close: %v", err)
	}
}

// setupTestPool starts a bare Postgres container, no migrations, LISTEN/NOTIFY needs no schema.
func setupTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := t.Context()

	// the postgres image logs the ready line twice: first the init bootstrap, then the real start
	const dbReadyLogOccurrences = 2

	pgContainer, err := postgres.Run(
		ctx,
		"postgres:18",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpassword"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(dbReadyLogOccurrences).
				WithStartupTimeout(10*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := pgContainer.Terminate(context.Background()); err != nil {
			t.Fatalf("failed to terminate pg container: %v", err)
		}
	})

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to create pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to ping db: %v", err)
	}

	return pool
}
