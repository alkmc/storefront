//go:build integration

package store

import (
	"testing"
	"time"
)

func TestListener_BuffersNotifyBetweenWaits(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	l := NewListener(repo.pool, OutboxChannel)
	defer l.Close()

	// The first wait times out and leaves the LISTEN connection subscribed.
	if err := l.Await(ctx, 50*time.Millisecond); err != nil {
		t.Fatalf("await before notify: %v", err)
	}

	if _, err := repo.pool.Exec(ctx, queryNotifyOutbox); err != nil {
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
