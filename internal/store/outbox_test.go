//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/alkmc/storefront/internal/event"
	"github.com/google/uuid"
)

func TestOutbox_WritesEmitEventsInTx(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	id := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, domain.Product{ID: id, Name: "Car", Price: testMoney(1000), Stock: 5},
	); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := repo.Update(
		ctx, domain.Product{ID: id, Name: "Sedan", Price: testMoney(1200)},
	); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := repo.Purchase(ctx, id, 2); err != nil {
		t.Fatalf("purchase: %v", err)
	}
	if err := repo.Delete(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	rows := outboxState(t, repo)
	if len(rows) != 4 {
		t.Fatalf("got %d outbox rows, want 4", len(rows))
	}

	wants := []struct {
		eventType string
		version   int64
		quantity  int64
		stock     int64
	}{
		{eventType: event.TypeCreated, version: 1, stock: 5},
		{eventType: event.TypeUpdated, version: 2, stock: 5},
		{eventType: event.TypePurchased, version: 3, quantity: 2, stock: 3},
		{eventType: event.TypeDeleted, version: 4, stock: 0},
	}
	for i, want := range wants {
		row := rows[i]
		if row.eventType != want.eventType {
			t.Errorf("row %d: got type %q, want %q", i, row.eventType, want.eventType)
		}

		var e event.Event
		if err := json.Unmarshal(row.payload, &e); err != nil {
			t.Fatalf("row %d: unmarshal payload: %v", i, err)
		}
		if e.EventID != row.messageID {
			t.Errorf("row %d: payload eventId %s != message_id %s", i, e.EventID, row.messageID)
		}
		if e.ProductID != id {
			t.Errorf("row %d: got productId %s, want %s", i, e.ProductID, id)
		}
		if e.Version != want.version {
			t.Errorf("row %d: got version %d, want %d", i, e.Version, want.version)
		}
		if e.Quantity != want.quantity || e.Stock != want.stock {
			t.Errorf("row %d: got quantity/stock %d/%d, want %d/%d",
				i, e.Quantity, e.Stock, want.quantity, want.stock)
		}
		if e.OccurredAt.IsZero() {
			t.Errorf("row %d: occurredAt is zero", i)
		}
	}

	// A rolled-back write leaves no event behind.
	if _, err := repo.Purchase(ctx, id, 1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("purchase after delete: got %v, want ErrNotFound", err)
	}
	if _, err := repo.Save(
		ctx, domain.Product{ID: uuid.Must(uuid.NewV7()), Name: "Bike", Price: testMoney(-1)},
	); err == nil {
		t.Fatal("save with negative price: expected error")
	}
	if got := outboxState(t, repo); len(got) != 4 {
		t.Errorf("failed writes added outbox rows: got %d, want 4", len(got))
	}
}

func TestOutbox_DrainBatchPublishesAndDeletes(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	seedProducts(t, repo, 3)

	var published []event.Record
	n, _, err := repo.DrainBatch(ctx, 10, 3, func(_ context.Context, r event.Record) error {
		published = append(published, r)
		return nil
	})
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if n != 3 || len(published) != 3 {
		t.Fatalf("got n=%d published=%d, want 3", n, len(published))
	}
	for i, r := range published {
		if r.MessageID == uuid.Nil {
			t.Errorf("record %d: zero message id", i)
		}
		if r.Type != event.TypeCreated {
			t.Errorf("record %d: got type %q, want %q", i, r.Type, event.TypeCreated)
		}
		if !json.Valid(r.Payload) {
			t.Errorf("record %d: invalid payload %q", i, r.Payload)
		}
	}
	if got := outboxState(t, repo); len(got) != 0 {
		t.Errorf("outbox not drained: %d rows left", len(got))
	}
}

func TestOutbox_TransientFailureKeepsRowDue(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	seedProducts(t, repo, 2)

	// First row publishes, the second hits a "broker down" error: fail-fast.
	var calls int
	n, _, err := repo.DrainBatch(ctx, 10, 3, func(context.Context, event.Record) error {
		calls++
		if calls == 2 {
			return errors.New("broker down")
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected joined transient error")
	}
	if n != 1 {
		t.Fatalf("got n=%d, want 1", n)
	}

	// The published row is committed, the failed one stays untouched and still due.
	rows := outboxState(t, repo)
	if len(rows) != 1 {
		t.Fatalf("got %d outbox rows, want 1", len(rows))
	}
	if rows[0].attempts != 0 {
		t.Errorf("transient failure bumped attempts: got %d, want 0", rows[0].attempts)
	}
	if !rows[0].due {
		t.Error("transient failure delayed next_attempt_at")
	}

	n, _, err = repo.DrainBatch(ctx, 10, 3, func(context.Context, event.Record) error { return nil })
	if err != nil || n != 1 {
		t.Fatalf("redrain: got n=%d err=%v, want 1 <nil>", n, err)
	}
}

func TestOutbox_PoisonDeadLettersAfterMaxAttempts(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()
	const maxAttempts = 2

	seedOutboxRows(t, repo, event.TypeCreated, event.TypeUpdated)
	poisonCreated := func(_ context.Context, r event.Record) error {
		if r.Type == event.TypeCreated {
			return fmt.Errorf("%w: unroutable", event.ErrUndeliverable)
		}
		return nil
	}

	// Poison does not stop the batch and costs an attempt with backoff.
	n, _, err := repo.DrainBatch(ctx, 10, maxAttempts, poisonCreated)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if n != 1 {
		t.Fatalf("got n=%d, want 1", n)
	}
	rows := outboxState(t, repo)
	if len(rows) != 1 || rows[0].eventType != event.TypeCreated {
		t.Fatalf("unexpected outbox state: %+v", rows)
	}
	if rows[0].attempts != 1 {
		t.Errorf("got attempts %d, want 1", rows[0].attempts)
	}
	if rows[0].due {
		t.Error("poison row not backed off")
	}

	// Not due yet: nothing to claim.
	if n, _, err := repo.DrainBatch(ctx, 10, maxAttempts, poisonCreated); n != 0 || err != nil {
		t.Fatalf("backed-off drain: got n=%d err=%v, want 0 <nil>", n, err)
	}

	// At maxAttempts the row moves to outbox_dead with its last error.
	makeOutboxDue(t, repo)
	if _, _, err := repo.DrainBatch(ctx, 10, maxAttempts, poisonCreated); err != nil {
		t.Fatalf("final drain: %v", err)
	}
	if rows := outboxState(t, repo); len(rows) != 0 {
		t.Errorf("outbox not empty: %+v", rows)
	}

	var (
		eventType string
		attempts  int32
		lastError string
	)
	row := repo.pool.QueryRow(ctx, `SELECT event_type, attempts, last_error FROM outbox_dead`)
	if err := row.Scan(&eventType, &attempts, &lastError); err != nil {
		t.Fatalf("scan outbox_dead: %v", err)
	}
	if eventType != event.TypeCreated || attempts != maxAttempts {
		t.Errorf("got dead row %s/%d, want %s/%d", eventType, attempts, event.TypeCreated, maxAttempts)
	}
	if lastError == "" {
		t.Error("last_error is empty")
	}
}

func TestOutbox_ConcurrentRelaysPublishExactlyOnce(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	const (
		total     = 60
		batchSize = 25
	)
	types := make([]string, total)
	for i := range types {
		types[i] = event.TypeCreated
	}
	seedOutboxRows(t, repo, types...)

	var (
		wg    sync.WaitGroup
		calls atomic.Int64
	)
	for range 2 {
		wg.Go(func() {
			for {
				n, _, err := repo.DrainBatch(ctx, batchSize, 3, func(context.Context, event.Record) error {
					calls.Add(1)
					return nil
				})
				if err != nil {
					t.Errorf("drain: %v", err)
					return
				}
				if n == 0 {
					return
				}
			}
		})
	}
	wg.Wait()

	if got := calls.Load(); got != total {
		t.Errorf("got %d publishes, want exactly %d", got, total)
	}
	if rows := outboxState(t, repo); len(rows) != 0 {
		t.Errorf("outbox not empty: %d rows left", len(rows))
	}
}

type outboxRowState struct {
	messageID uuid.UUID
	eventType string
	attempts  int32
	due       bool
	payload   []byte
}

// outboxState returns all outbox rows in id order.
func outboxState(t *testing.T, repo *Postgres) []outboxRowState {
	t.Helper()
	rows, err := repo.pool.Query(t.Context(),
		`SELECT message_id, event_type, attempts, next_attempt_at <= now(), payload
		 FROM outbox ORDER BY id`)
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	defer rows.Close()

	var out []outboxRowState
	for rows.Next() {
		var r outboxRowState
		if err := rows.Scan(&r.messageID, &r.eventType, &r.attempts, &r.due, &r.payload); err != nil {
			t.Fatalf("scan outbox: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("outbox rows: %v", err)
	}
	return out
}

// seedProducts saves n products, leaving one created event per product in the outbox.
func seedProducts(t *testing.T, repo *Postgres, n int) {
	t.Helper()
	for i := range n {
		if _, err := repo.Save(t.Context(), domain.Product{
			ID: uuid.Must(uuid.NewV7()), Name: fmt.Sprintf("P%d", i), Price: testMoney(100),
		}); err != nil {
			t.Fatalf("seed product %d: %v", i, err)
		}
	}
}

// seedOutboxRows inserts bare outbox rows directly, one per event type.
func seedOutboxRows(t *testing.T, repo *Postgres, eventTypes ...string) {
	t.Helper()
	for i, typ := range eventTypes {
		if _, err := repo.pool.Exec(
			t.Context(),
			queryInsertOutbox, uuid.Must(uuid.NewV7()), typ, []byte(`{}`),
		); err != nil {
			t.Fatalf("seed outbox row %d: %v", i, err)
		}
	}
}

// makeOutboxDue clears every row's retry backoff.
func makeOutboxDue(t *testing.T, repo *Postgres) {
	t.Helper()
	if _, err := repo.pool.Exec(
		t.Context(),
		`UPDATE outbox SET next_attempt_at = now()`,
	); err != nil {
		t.Fatalf("reset next_attempt_at: %v", err)
	}
}
