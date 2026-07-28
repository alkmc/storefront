//go:build integration

package pg

import (
	"testing"
	"time"

	"github.com/alkmc/storefront/internal/pg/pgtest"
)

// testChannel is a self-contained NOTIFY channel, the listener needs no schema.
const testChannel = "pg_test_channel"

func TestListener_BuffersNotifyBetweenWaits(t *testing.T) {
	// LISTEN/NOTIFY needs no schema, so a bare database is enough
	pool := pgtest.Pool(t)
	ctx := t.Context()

	l := NewListener(pool, testChannel)
	defer l.Close()

	// The first wait times out and leaves the LISTEN connection subscribed.
	if err := l.Await(ctx, 50*time.Millisecond); err != nil {
		t.Fatalf("await before notify: %v", err)
	}

	if _, err := pool.Exec(ctx, "SELECT pg_notify($1, '')", testChannel); err != nil {
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
