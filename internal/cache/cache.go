package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/alkmc/restClean/internal/entity"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ErrCacheMiss is returned by Get when the key is not present in the cache.
var ErrCacheMiss = errors.New("cache: key not found")

// cacheEntry is the JSON shape stored in Redis, decoupled from entity.Product.
type cacheEntry struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedis returns a Redis-backed cache with the given TTL.
func NewRedis(ctx context.Context, host string, db int, ttl time.Duration) (*RedisCache, error) {
	client := redis.NewClient(new(redis.Options{
		Addr: host,
		DB:   db,
	}))

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return new(RedisCache{client: client, ttl: ttl}), nil
}

func (r *RedisCache) Set(ctx context.Context, key string, value entity.Product) error {
	data, err := json.Marshal(cacheEntry{
		ID:    value.ID.String(),
		Name:  value.Name,
		Price: value.Price,
	})
	if err != nil {
		return fmt.Errorf("marshal cache value for key %q: %w", key, err)
	}
	if err := r.client.Set(ctx, key, data, r.ttl).Err(); err != nil {
		return fmt.Errorf("set cache key %q: %w", key, err)
	}
	return nil
}

func (r *RedisCache) Get(ctx context.Context, key string) (entity.Product, error) {
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
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
	return entity.Product{ID: id, Name: e.Name, Price: e.Price}, nil
}

func (r *RedisCache) Invalidate(ctx context.Context, key string) error {
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("invalidate cache key %q: %w", key, err)
	}
	return nil
}

func (r *RedisCache) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *RedisCache) Close() error {
	if err := r.client.Close(); err != nil {
		return fmt.Errorf("close redis client: %w", err)
	}
	return nil
}
