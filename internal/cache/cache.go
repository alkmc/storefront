package cache

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
	"github.com/redis/rueidis"
)

const (
	tagPayload   byte = 'p'
	tagTombstone byte = 't'
)

type (
	cacheEntry struct {
		ID      string     `json:"id"`
		Name    string     `json:"name"`
		Stock   int64      `json:"stock"`
		Version int64      `json:"version"`
		Price   moneyEntry `json:"price"`
	}
	moneyEntry struct {
		MinorAmount int64           `json:"minorAmount"`
		Currency    domain.Currency `json:"currency"`
	}
	Entry struct {
		Product domain.Product
		Hit     bool
		token   []byte
	}
	Redis struct {
		client rueidis.Client
		ttl    time.Duration
	}
)

// New wraps an open Redis client in a cache with the given entry TTL.
func New(client rueidis.Client, ttl time.Duration) *Redis {
	return new(Redis{client: client, ttl: ttl})
}

func (r *Redis) Set(ctx context.Context, key string, p domain.Product, prev Entry) error {
	payload, err := encodeProduct(p)
	if err != nil {
		return fmt.Errorf("marshal cache value for key %q: %w", key, err)
	}
	return r.guardedSet(ctx, key, payload, prev)
}

func (r *Redis) Get(ctx context.Context, key string) (Entry, error) {
	raw, err := r.client.Do(ctx, r.client.B().Get().Key(key).Build()).AsBytes()
	if err != nil {
		if rueidis.IsRedisNil(err) {
			return Entry{}, nil
		}
		return Entry{}, fmt.Errorf("get cache key %q: %w", key, err)
	}
	return classify(raw), nil
}

func (r *Redis) Invalidate(ctx context.Context, key string) error {
	cmd := r.client.B().Set().Key(key).
		Value(rueidis.BinaryString(newTombstone())).
		PxMilliseconds(r.ttl.Milliseconds()).
		Build()
	if err := r.client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("invalidate cache key %q: %w", key, err)
	}
	return nil
}

func (r *Redis) Ping(ctx context.Context) error {
	return r.client.Do(ctx, r.client.B().Ping().Build()).Error()
}

func (r *Redis) guardedSet(ctx context.Context, key string, value []byte, prev Entry) error {
	err := r.client.Do(ctx, r.setCmd(key, value, prev)).Error()
	if err != nil && !rueidis.IsRedisNil(err) {
		return fmt.Errorf("set cache key %q: %w", key, err)
	}
	return nil
}

func (r *Redis) setCmd(key string, value []byte, prev Entry) rueidis.Completed {
	set := r.client.B().Set().Key(key).Value(rueidis.BinaryString(value))
	ttl := r.ttl.Milliseconds()
	if prev.token == nil {
		return set.Nx().PxMilliseconds(ttl).Build()
	}
	return set.Ifeq(rueidis.BinaryString(prev.token)).PxMilliseconds(ttl).Build()
}

func classify(raw []byte) Entry {
	if len(raw) > 0 && raw[0] == tagPayload {
		if p, ok := decodeProduct(raw[1:]); ok {
			return Entry{Product: p, Hit: true}
		}
	}
	return Entry{token: raw}
}

func newTombstone() []byte {
	return append([]byte{tagTombstone}, rand.Text()...)
}

func encodeProduct(p domain.Product) ([]byte, error) {
	raw, err := json.Marshal(cacheEntry{
		ID:      p.ID.String(),
		Name:    p.Name,
		Stock:   p.Stock,
		Version: p.Version,
		Price: moneyEntry{
			MinorAmount: p.Price.MinorAmount,
			Currency:    p.Price.Currency,
		},
	})
	if err != nil {
		return nil, err
	}
	return append([]byte{tagPayload}, raw...), nil
}

func decodeProduct(raw []byte) (domain.Product, bool) {
	var e cacheEntry
	if err := json.Unmarshal(raw, &e); err != nil {
		return domain.Product{}, false
	}
	id, err := uuid.Parse(e.ID)
	if err != nil {
		return domain.Product{}, false
	}
	return domain.Product{
		ID:      id,
		Name:    e.Name,
		Stock:   e.Stock,
		Version: e.Version,
		Price: domain.Money{
			MinorAmount: e.Price.MinorAmount,
			Currency:    e.Price.Currency,
		},
	}, true
}
