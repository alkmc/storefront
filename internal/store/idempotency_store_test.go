//go:build integration

package store

import (
	"errors"
	"sync"
	"testing"
	"uuid"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/go-cmp/cmp"
)

func TestPostgres_CreateOrder_IdempotentReplay(t *testing.T) {
	repo := setupTestContainerDB(t)
	ctx := t.Context()

	productID := seedProduct(t, repo, 5)
	userID := uuid.NewV7()
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
	if diff := cmp.Diff(first, replay); diff != "" {
		t.Errorf("replay diverged (-first +replay):\n%s", diff)
	}

	// stock decremented exactly once and only one order exists
	if s := productStock(t, repo, productID); s != 3 {
		t.Errorf("stock %d, want 3 (single decrement)", s)
	}
	page, err := repo.FindOrders(ctx, domain.UserID(userID), domain.Cursor{}, 10)
	if err != nil {
		t.Fatalf("list orders: %v", err)
	}
	if len(page.Items) != 1 {
		t.Errorf("got %d orders, want 1", len(page.Items))
	}
}

func TestPostgres_CreateOrder_IdempotencyMismatch(t *testing.T) {
	repo := setupTestContainerDB(t)
	ctx := t.Context()

	productID := seedProduct(t, repo, 5)
	userID := uuid.NewV7()
	key := domain.IdempotencyKey("key-x")

	if _, _, err := repo.CreateOrder(ctx, testOrder(userID, productID, 2), key); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// same key, different payload
	if _, _, err := repo.CreateOrder(ctx, testOrder(userID, productID, 3), key); !errors.Is(
		err, domain.ErrIdempotencyMismatch,
	) {
		t.Fatalf("got %v, want domain.ErrIdempotencyMismatch", err)
	}

	// the rejected mismatch changed nothing
	if s := productStock(t, repo, productID); s != 3 {
		t.Errorf("stock %d, want 3 (mismatch must not decrement)", s)
	}
}

func TestPostgres_CreateOrder_KeyReusableAfterFailure(t *testing.T) {
	repo := setupTestContainerDB(t)
	ctx := t.Context()

	productID := seedProduct(t, repo, 1)
	userID := uuid.NewV7()
	key := domain.IdempotencyKey("retry-key")

	// a failed purchase must not persist the key
	if _, _, err := repo.CreateOrder(ctx, testOrder(userID, productID, 2), key); !errors.Is(
		err, domain.ErrInsufficientStock,
	) {
		t.Fatalf("got %v, want domain.ErrInsufficientStock", err)
	}

	// same key, satisfiable payload: a leftover row would have made this a mismatch, not a success
	_, replayed, err := repo.CreateOrder(ctx, testOrder(userID, productID, 1), key)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if replayed {
		t.Error("retry after a failed purchase must not replay")
	}
	if s := productStock(t, repo, productID); s != 0 {
		t.Errorf("stock %d, want 0", s)
	}
}

func TestPostgres_CreateOrder_ExpiredKeyReexecutes(t *testing.T) {
	repo := setupTestContainerDB(t)
	ctx := t.Context()

	productID := seedProduct(t, repo, 5)
	userID := uuid.NewV7()
	idem := domain.IdempotencyKey("expiring-key")

	first, _, err := repo.CreateOrder(ctx, testOrder(userID, productID, 2), idem)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	expireKey(t, repo, userID, idem)

	// same key and payload, but a reclaimed key buys anew
	second, replayed, err := repo.CreateOrder(ctx, testOrder(userID, productID, 2), idem)
	if err != nil {
		t.Fatalf("reclaim call: %v", err)
	}
	if replayed {
		t.Error("expired key must reexecute, not replay")
	}
	if second.ID == first.ID {
		t.Error("reclaim must create a new order")
	}
	if s := productStock(t, repo, productID); s != 1 {
		t.Errorf("stock %d, want 1 (two decrements)", s)
	}

	// the reclaimed key now replays the second order, not the first
	replay, replayed, err := repo.CreateOrder(ctx, testOrder(userID, productID, 2), idem)
	if err != nil {
		t.Fatalf("replay call: %v", err)
	}
	if !replayed {
		t.Error("third call must replay the reclaimed order")
	}
	if replay.ID != second.ID {
		t.Errorf("replay id %v, want second order %v", replay.ID, second.ID)
	}
}

func TestPostgres_CreateOrder_IdempotentUnderConcurrency(t *testing.T) {
	repo := setupTestContainerDB(t)

	const buyers = 10
	productID := seedProduct(t, repo, 5)
	userID := uuid.NewV7()
	idem := domain.IdempotencyKey("same-key")

	ids := concurrentCreateOrders(t, repo, buyers, userID, productID, 1, idem)
	if len(ids) != 1 {
		t.Errorf("got %d distinct order ids, want 1 (all replay one order)", len(ids))
	}
	if s := productStock(t, repo, productID); s != 4 {
		t.Errorf("stock %d, want 4 (exactly one decrement)", s)
	}
}

func TestPostgres_CreateOrder_ConcurrentReclaim(t *testing.T) {
	repo := setupTestContainerDB(t)
	ctx := t.Context()

	const buyers = 10
	productID := seedProduct(t, repo, 5)
	userID := uuid.NewV7()
	idem := domain.IdempotencyKey("reclaim-key")

	first, _, err := repo.CreateOrder(ctx, testOrder(userID, productID, 1), idem)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	expireKey(t, repo, userID, idem)

	ids := concurrentCreateOrders(t, repo, buyers, userID, productID, 1, idem)
	if len(ids) != 1 {
		t.Fatalf("got %d distinct order ids, want 1 (one reclaims, the rest replay it)", len(ids))
	}
	if _, replayedOld := ids[uuid.UUID(first.ID)]; replayedOld {
		t.Error("burst replayed the expired order instead of reclaiming")
	}
	if s := productStock(t, repo, productID); s != 3 {
		t.Errorf("stock %d, want 3 (initial plus exactly one reclaim decrement)", s)
	}
}

func TestPostgres_PurgeIdempotencyKeys(t *testing.T) {
	repo := setupTestContainerDB(t)
	ctx := t.Context()

	seed := func(key, expiresIn string) {
		t.Helper()
		if _, err := repo.pool.Exec(
			ctx,
			`INSERT INTO idempotency_keys (user_id, key, request_hash, expires_at)
			 VALUES ($1, $2, '\x00', now() + `+expiresIn+`)`,
			uuid.NewV7(), key,
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

// expireKey backdates the key TTL so the next CreateOrder reclaims it instead of replaying.
func expireKey(t *testing.T, repo *Postgres, userID uuid.UUID, idem domain.IdempotencyKey) {
	t.Helper()
	if _, err := repo.pool.Exec(
		t.Context(),
		`UPDATE idempotency_keys SET expires_at = now() - interval '1 hour' WHERE user_id = $1 AND key = $2`,
		userID, string(idem),
	); err != nil {
		t.Fatalf("expire key: %v", err)
	}
}

// concurrentCreateOrders fires n CreateOrder calls with the same key at once and returns the distinct
// order ids they produce, so a test can assert they all resolve to a single order.
func concurrentCreateOrders(
	t *testing.T, repo *Postgres, n int, userID, productID uuid.UUID, qty int64, idem domain.IdempotencyKey,
) map[uuid.UUID]struct{} {
	t.Helper()
	ctx := t.Context()
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		ids = make(map[uuid.UUID]struct{})
	)
	start := make(chan struct{})
	for range n {
		wg.Go(func() {
			<-start
			order, _, err := repo.CreateOrder(ctx, testOrder(userID, productID, qty), idem)
			if err != nil {
				t.Errorf("concurrent CreateOrder: %v", err)
				return
			}
			mu.Lock()
			ids[uuid.UUID(order.ID)] = struct{}{}
			mu.Unlock()
		})
	}
	close(start)
	wg.Wait()
	return ids
}
