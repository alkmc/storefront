package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/alkmc/storefront/internal/config"
	"github.com/alkmc/storefront/internal/entity"
	"github.com/google/uuid"
	"github.com/redis/rueidis"
)

// ErrCacheMiss is returned by Get when the key is not present in the cache.
var ErrCacheMiss = errors.New("cache: key not found")

type (
	cacheEntry struct {
		ID    string     `json:"id"`
		Name  string     `json:"name"`
		Price moneyEntry `json:"price"`
	}
	moneyEntry struct {
		MinorAmount int64           `json:"minorAmount"`
		Currency    entity.Currency `json:"currency"`
	}
	RedisCache struct {
		client rueidis.Client
		ttl    time.Duration
	}
)

// NewRedis returns a Redis-backed cache configured from cfg.
func NewRedis(ctx context.Context, cfg config.Redis) (*RedisCache, error) {
	client, err := rueidis.NewClient(rueidis.ClientOption{
		InitAddress: []string{cfg.Address()},
		Password:    cfg.Password.Reveal(),
		SelectDB:    cfg.DB,
	})
	if err != nil {
		return nil, fmt.Errorf("create redis client: %w", err)
	}

	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return new(RedisCache{client: client, ttl: cfg.TTL}), nil
}

func (r *RedisCache) Set(ctx context.Context, key string, value entity.Product) error {
	data, err := json.Marshal(cacheEntry{
		ID:   value.ID.String(),
		Name: value.Name,
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

func (r *RedisCache) Get(ctx context.Context, key string) (entity.Product, error) {
	data, err := r.client.Do(ctx, r.client.B().Get().Key(key).Build()).AsBytes()
	if err != nil {
		if rueidis.IsRedisNil(err) {
			return entity.Product{}, ErrCacheMiss
		}
		return entity.Product{}, fmt.Errorf("get cache key %q: %w", key, err)
	}
	var e cacheEntry
	if err := json.Unmarshal(data, &e); err != nil {
		return entity.Product{}, fmt.Errorf("unmarshal cache value for key %q: %w", key, err)
	}
	id, err := uuid.Parse(e.ID)
	if err != nil {
		return entity.Product{}, fmt.Errorf("parse cached id for key %q: %w", key, err)
	}
	return entity.Product{
		ID:   id,
		Name: e.Name,
		Price: entity.Money{
			MinorAmount: e.Price.MinorAmount,
			Currency:    e.Price.Currency,
		},
	}, nil
}

func (r *RedisCache) Invalidate(ctx context.Context, key string) error {
	if err := r.client.Do(ctx, r.client.B().Del().Key(key).Build()).Error(); err != nil {
		return fmt.Errorf("invalidate cache key %q: %w", key, err)
	}
	return nil
}

func (r *RedisCache) Ping(ctx context.Context) error {
	return r.client.Do(ctx, r.client.B().Ping().Build()).Error()
}

func (r *RedisCache) Close() {
	r.client.Close()
}
