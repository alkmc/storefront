//go:build integration

package cache

import (
	"testing"
	"time"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
	"github.com/redis/rueidis"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

const (
	testTTL    = time.Minute
	testNegTTL = 10 * time.Second
)

// setupTestContainerRedis needs an image with IFEQ, which landed in Redis 8.4.
func setupTestContainerRedis(t *testing.T) *Redis {
	t.Helper()
	ctx := t.Context()

	container, err := tcredis.Run(ctx, "redis:8.8")
	if err != nil {
		t.Fatalf("run redis container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("terminate redis container: %v", err)
		}
	})

	endpoint, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("redis endpoint: %v", err)
	}
	client, err := rueidis.NewClient(rueidis.ClientOption{
		InitAddress:  []string{endpoint},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("redis client: %v", err)
	}
	t.Cleanup(client.Close)

	return New(client, testTTL, testNegTTL)
}

func testProduct(name string) domain.Product {
	return domain.Product{
		ID:    uuid.Must(uuid.NewV7()),
		Name:  name,
		Stock: 5,
		Price: domain.Money{MinorAmount: 100, Currency: domain.CurrencyPLN},
	}
}

func TestRedis_GuardRejectsStaleSet(t *testing.T) {
	t.Parallel()
	r := setupTestContainerRedis(t)
	ctx := t.Context()

	fresh := testProduct("fresh")
	stale := testProduct("stale")
	key := fresh.ID.String()

	cold, err := r.Get(ctx, key)
	if err != nil || cold.Hit {
		t.Fatalf("cold Get: hit=%v err=%v", cold.Hit, err)
	}
	if err := r.Set(ctx, key, fresh, cold); err != nil {
		t.Fatalf("cold Set: %v", err)
	}

	if err := r.Invalidate(ctx, key); err != nil {
		t.Fatalf("invalidate: %v", err)
	}

	if err := r.Set(ctx, key, stale, cold); err != nil {
		t.Fatalf("unaware Set: %v", err)
	}
	if got, _ := r.Get(ctx, key); got.Hit {
		t.Fatalf("a reader holding no token resurrected the product: %+v", got.Product)
	}

	stalePrev, err := r.Get(ctx, key)
	if err != nil || stalePrev.Hit {
		t.Fatalf("post-invalidate Get: hit=%v err=%v", stalePrev.Hit, err)
	}

	if err := r.Invalidate(ctx, key); err != nil {
		t.Fatalf("second invalidate: %v", err)
	}

	if err := r.Set(ctx, key, stale, stalePrev); err != nil {
		t.Fatalf("stale Set: %v", err)
	}
	if got, _ := r.Get(ctx, key); got.Hit {
		t.Fatalf("stale Set populated the cache with %+v", got.Product)
	}

	entry, err := r.Get(ctx, key)
	if err != nil {
		t.Fatalf("fresh Get: %v", err)
	}
	if err := r.Set(ctx, key, fresh, entry); err != nil {
		t.Fatalf("fresh Set: %v", err)
	}
	got, err := r.Get(ctx, key)
	if err != nil {
		t.Fatalf("final Get: %v", err)
	}
	if !got.Hit || got.Product != fresh {
		t.Errorf("final Get = %+v (hit=%v), want %+v", got.Product, got.Hit, fresh)
	}
}
