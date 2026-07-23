//go:build integration

package store

import (
	"errors"
	"sync"
	"testing"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

func TestPostgres_CreateOrder_IdempotentReplay(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	productID := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, domain.Product{ID: productID, Name: "Widget", Price: testMoney(1000), Stock: 5},
	); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	userID := uuid.Must(uuid.NewV7())
	idem := domain.IdempotencyKey("key-1")

	first, firstReplayed, err := repo.CreateOrder(ctx, testOrder(userID, productID, 2), idem)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if firstReplayed {
		t.Error("first call must not be a replay")
	}

	replay, replayReplayed, err := repo.CreateOrder(ctx, testOrder(userID, productID, 2), idem)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !replayReplayed {
		t.Error("second call with the same key must replay")
	}

	// the replay reproduces the original order exactly (fetched via JOIN)
	if replay.ID != first.ID || replay.ProductID != first.ProductID ||
		replay.Quantity != first.Quantity || replay.UnitPrice != first.UnitPrice {
		t.Errorf("replay diverged: first %+v, replay %+v", first, replay)
	}
	if !replay.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("replay created_at: got %v, want %v", replay.CreatedAt, first.CreatedAt)
	}

	// stock decremented exactly once and only one order exists
	got, err := repo.FindByID(ctx, productID)
	if err != nil {
		t.Fatalf("reload product: %v", err)
	}
	if got.Stock != 3 {
		t.Errorf("stock %d, want 3 (single decrement)", got.Stock)
	}
	page, err := repo.FindOrders(ctx, domain.UserID(userID), uuid.NullUUID{}, 10)
	if err != nil {
		t.Fatalf("list orders: %v", err)
	}
	if len(page.Items) != 1 {
		t.Errorf("got %d orders, want 1", len(page.Items))
	}
}

func TestPostgres_CreateOrder_IdempotencyMismatch(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	productID := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, domain.Product{ID: productID, Name: "Widget", Price: testMoney(1000), Stock: 5},
	); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	userID := uuid.Must(uuid.NewV7())

	first := domain.IdempotencyKey("key-x")
	if _, _, err := repo.CreateOrder(ctx, testOrder(userID, productID, 2), first); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// same key, different payload
	mismatch := domain.IdempotencyKey("key-x")
	if _, _, err := repo.CreateOrder(ctx, testOrder(userID, productID, 3), mismatch); !errors.Is(
		err, domain.ErrIdempotencyMismatch,
	) {
		t.Fatalf("got %v, want domain.ErrIdempotencyMismatch", err)
	}

	// the rejected mismatch changed nothing
	got, err := repo.FindByID(ctx, productID)
	if err != nil {
		t.Fatalf("reload product: %v", err)
	}
	if got.Stock != 3 {
		t.Errorf("stock %d, want 3 (mismatch must not decrement)", got.Stock)
	}
}

func TestPostgres_CreateOrder_KeyReusableAfterFailure(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	productID := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, domain.Product{ID: productID, Name: "Widget", Price: testMoney(1000), Stock: 1},
	); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	userID := uuid.Must(uuid.NewV7())

	// a failed purchase must not persist the key
	failed := domain.IdempotencyKey("retry-key")
	if _, _, err := repo.CreateOrder(ctx, testOrder(userID, productID, 2), failed); !errors.Is(
		err, domain.ErrInsufficientStock,
	) {
		t.Fatalf("got %v, want domain.ErrInsufficientStock", err)
	}

	// same key, satisfiable payload: a leftover row would have made this a mismatch, not a success
	retry := domain.IdempotencyKey("retry-key")
	_, replayed, err := repo.CreateOrder(ctx, testOrder(userID, productID, 1), retry)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if replayed {
		t.Error("retry after a failed purchase must not replay")
	}
	got, err := repo.FindByID(ctx, productID)
	if err != nil {
		t.Fatalf("reload product: %v", err)
	}
	if got.Stock != 0 {
		t.Errorf("stock %d, want 0", got.Stock)
	}
}

func TestPostgres_CreateOrder_IdempotentUnderConcurrency(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	const buyers = 10
	productID := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, domain.Product{ID: productID, Name: "Widget", Price: testMoney(1000), Stock: 5},
	); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	userID := uuid.Must(uuid.NewV7())
	idem := domain.IdempotencyKey("same-key")

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		ids   = make(map[uuid.UUID]struct{})
		fails int
	)
	start := make(chan struct{})
	for range buyers {
		wg.Go(func() {
			<-start
			order, _, err := repo.CreateOrder(ctx, testOrder(userID, productID, 1), idem)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fails++
				return
			}
			ids[uuid.UUID(order.ID)] = struct{}{}
		})
	}
	close(start)
	wg.Wait()

	if fails != 0 {
		t.Fatalf("got %d failures, want 0", fails)
	}
	if len(ids) != 1 {
		t.Errorf("got %d distinct order ids, want 1 (all replay one order)", len(ids))
	}
	got, err := repo.FindByID(ctx, productID)
	if err != nil {
		t.Fatalf("reload product: %v", err)
	}
	if got.Stock != 4 {
		t.Errorf("stock %d, want 4 (exactly one decrement)", got.Stock)
	}
}

func TestPostgres_PurgeIdempotencyKeys(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	seed := func(key, expiresIn string) {
		t.Helper()
		if _, err := repo.pool.Exec(
			ctx,
			`INSERT INTO idempotency_keys (user_id, key, request_hash, expires_at)
			 VALUES ($1, $2, '\x00', now() + `+expiresIn+`)`,
			uuid.Must(uuid.NewV7()), key,
		); err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
	}
	seed("expired", "interval '-1 hour'")
	seed("live", "interval '1 hour'")

	n, err := repo.PurgeIdempotencyKeys(ctx)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 1 {
		t.Errorf("purged %d rows, want 1", n)
	}
}
