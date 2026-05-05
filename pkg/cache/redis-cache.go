package cache

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/alkmc/restClean/pkg/entity"

	"github.com/redis/go-redis/v9"
)

type redisCache struct {
	logger  *slog.Logger
	host    string
	db      int
	expires time.Duration
}

// NewRedis returns new redisCache struct
func NewRedis(l *slog.Logger, host string, db int, exp time.Duration) Cache {
	return &redisCache{
		logger:  l,
		host:    host,
		db:      db,
		expires: exp,
	}
}

func (r *redisCache) getClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     r.host,
		Password: "",
		DB:       r.db,
	})
}

func (r *redisCache) Set(ctx context.Context, key string, prod *entity.Product) {
	client := r.getClient()

	jsonProd, err := json.Marshal(prod)
	if err != nil {
		r.logger.Error("failed to marshal product for cache", slog.Any("error", err))
		return
	}
	client.Set(ctx, key, jsonProd, r.expires*time.Second)
}

func (r *redisCache) Get(ctx context.Context, key string) *entity.Product {
	client := r.getClient()

	val, err := client.Get(ctx, key).Result()
	if err != nil {
		return nil
	}
	p := entity.Product{}
	if err := json.Unmarshal([]byte(val), &p); err != nil {
		r.logger.Error("failed to unmarshal product from cache", slog.Any("error", err))
		return nil
	}
	return &p
}

func (r *redisCache) Expire(ctx context.Context, key string) {
	client := r.getClient()
	client.Del(ctx, key)
}
