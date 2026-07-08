package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
	"github.com/redis/rueidis"
)

// ErrCacheMiss is returned by Get when the key is not present in the cache.
var ErrCacheMiss = errors.New("cache: key not found")

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
	Redis struct {
		client rueidis.Client
		ttl    time.Duration
	}
)

// New wraps an open Redis client in a cache with the given entry TTL.
func New(client rueidis.Client, ttl time.Duration) *Redis {
	return new(Redis{client: client, ttl: ttl})
}

func (r *Redis) Set(ctx context.Context, key string, value domain.Product) error {
	data, err := json.Marshal(cacheEntry{
		ID:      value.ID.String(),
		Name:    value.Name,
		Stock:   value.Stock,
		Version: value.Version,
		Price: moneyEntry{
			MinorAmount: value.Price.MinorAmount,
			Currency:    value.Price.Currency,
		},
	})
	if err != nil {
		return fmt.Errorf("marshal cache value for key %q: %w", key, err)
	}

	cmd := r.client.B().Set().Key(key).
		Value(rueidis.BinaryString(data)).
		PxMilliseconds(r.ttl.Milliseconds()).
		Build()
	if err := r.client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("set cache key %q: %w", key, err)
	}

	return nil
}

func (r *Redis) Get(ctx context.Context, key string) (domain.Product, error) {
	data, err := r.client.Do(ctx, r.client.B().Get().Key(key).Build()).AsBytes()
	if err != nil {
		if rueidis.IsRedisNil(err) {
			return domain.Product{}, ErrCacheMiss
		}
		return domain.Product{}, fmt.Errorf("get cache key %q: %w", key, err)
	}

	var e cacheEntry
	if err := json.Unmarshal(data, &e); err != nil {
		return domain.Product{}, fmt.Errorf("unmarshal cache value for key %q: %w", key, err)
	}
	id, err := uuid.Parse(e.ID)
	if err != nil {
		return domain.Product{}, fmt.Errorf("parse cached id for key %q: %w", key, err)
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
	}, nil
}

func (r *Redis) Invalidate(ctx context.Context, key string) error {
	if err := r.client.Do(ctx, r.client.B().Del().Key(key).Build()).Error(); err != nil {
		return fmt.Errorf("invalidate cache key %q: %w", key, err)
	}
	return nil
}

func (r *Redis) Ping(ctx context.Context) error {
	return r.client.Do(ctx, r.client.B().Ping().Build()).Error()
}
