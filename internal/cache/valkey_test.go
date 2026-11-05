//go:build integration

package cache

import (
	"testing"
	"time"
	"uuid"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/testcontainers/testcontainers-go"
	tcvalkey "github.com/testcontainers/testcontainers-go/modules/valkey"
	"github.com/valkey-io/valkey-go"
)

const (
	// keep in sync with compose.yaml; IFEQ needs Valkey 8.1 or later
	valkeyImage = "valkey/valkey:9.1.3"

	testTTL    = time.Minute
	testNegTTL = 10 * time.Second
)

func setupTestContainerValkey(t *testing.T) *Valkey {
	t.Helper()
	ctx := t.Context()

	container, err := tcvalkey.Run(ctx, valkeyImage)
	if err != nil {
		t.Fatalf("run valkey container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("terminate valkey container: %v", err)
		}
	})

	endpoint, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("valkey endpoint: %v", err)
	}
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{endpoint},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey client: %v", err)
	}
	t.Cleanup(client.Close)

	return New(client, testTTL, testNegTTL)
}

func testProduct(name string) domain.Product {
	return domain.Product{
		ID:    domain.ProductID(uuid.NewV7()),
		Name:  name,
		Stock: 5,
		Price: domain.Money{MinorAmount: 100, Currency: domain.CurrencyPLN},
	}
}

func TestValkey_GuardRejectsStaleSet(t *testing.T) {
	t.Parallel()
	v := setupTestContainerValkey(t)
	ctx := t.Context()

	fresh := testProduct("fresh")
	stale := testProduct("stale")
	key := fresh.ID.String()

	cold, err := v.Get(ctx, key)
	if err != nil || cold.Hit {
		t.Fatalf("cold Get: hit=%v err=%v", cold.Hit, err)
	}
	if err := v.Set(ctx, key, fresh, cold); err != nil {
		t.Fatalf("cold Set: %v", err)
	}

	if err := v.Invalidate(ctx, key); err != nil {
		t.Fatalf("invalidate: %v", err)
	}

	if err := v.Set(ctx, key, stale, cold); err != nil {
		t.Fatalf("unaware Set: %v", err)
	}
	if got, _ := v.Get(ctx, key); got.Hit {
		t.Fatalf("a reader holding no token resurrected the product: %+v", got.Product)
	}

	stalePrev, err := v.Get(ctx, key)
	if err != nil || stalePrev.Hit {
		t.Fatalf("post-invalidate Get: hit=%v err=%v", stalePrev.Hit, err)
	}

	if err := v.Invalidate(ctx, key); err != nil {
		t.Fatalf("second invalidate: %v", err)
	}

	if err := v.Set(ctx, key, stale, stalePrev); err != nil {
		t.Fatalf("stale Set: %v", err)
	}
	if got, _ := v.Get(ctx, key); got.Hit {
		t.Fatalf("stale Set populated the cache with %+v", got.Product)
	}

	entry, err := v.Get(ctx, key)
	if err != nil {
		t.Fatalf("fresh Get: %v", err)
	}
	if err := v.Set(ctx, key, fresh, entry); err != nil {
		t.Fatalf("fresh Set: %v", err)
	}
	got, err := v.Get(ctx, key)
	if err != nil {
		t.Fatalf("final Get: %v", err)
	}
	if !got.Hit || got.Product != fresh {
		t.Errorf("final Get = %+v (hit=%v), want %+v", got.Product, got.Hit, fresh)
	}
}

func TestValkey_SetReplacesEmptyValue(t *testing.T) {
	t.Parallel()
	v := setupTestContainerValkey(t)

	tests := []struct {
		name string
		prev func(t *testing.T, key string) Entry
	}{
		{
			name: "token read back",
			prev: func(t *testing.T, key string) Entry {
				t.Helper()
				entry, err := v.Get(t.Context(), key)
				if err != nil || entry.Hit {
					t.Fatalf("Get over empty value: hit=%v err=%v", entry.Hit, err)
				}
				return entry
			},
		},
		{
			// the client may hand an empty value back as a nil slice
			name: "token read back as nil",
			prev: func(*testing.T, string) Entry { return Entry{exists: true} },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			p := testProduct(tc.name)
			key := p.ID.String()

			// no TTL, so a misread empty value would block the fill for good
			if err := v.client.Do(ctx, v.client.B().Set().Key(key).Value("").Build()).Error(); err != nil {
				t.Fatalf("seed empty value: %v", err)
			}
			if err := v.Set(ctx, key, p, tc.prev(t, key)); err != nil {
				t.Fatalf("Set: %v", err)
			}
			got, err := v.Get(ctx, key)
			if err != nil {
				t.Fatalf("final Get: %v", err)
			}
			if !got.Hit || got.Product != p {
				t.Errorf("final Get = %+v (hit=%v), want %+v", got.Product, got.Hit, p)
			}
		})
	}
}
