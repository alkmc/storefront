package cache

import (
	"encoding/json"
	"log"
	"time"

	"github.com/alkmc/restClean/pkg/entity"

	"github.com/go-redis/redis/v7"
)

type redisCache struct {
	host    string
	db      int
	expires time.Duration
}

// NewRedis returns new redisCache struct
func NewRedis(host string, db int, exp time.Duration) Cache {
	return &redisCache{
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

func (r *redisCache) Set(key string, prod *entity.Product) {
	client := r.getClient()

	jsonProd, err := json.Marshal(prod)
	if err != nil {
		log.Println(err)
		return
	}
	client.Set(key, jsonProd, r.expires*time.Second)
}

func (r *redisCache) Get(key string) *entity.Product {
	client := r.getClient()

	val, err := client.Get(key).Result()
	if err != nil {
		return nil
	}
	p := entity.Product{}
	if err := json.Unmarshal([]byte(val), &p); err != nil {
		log.Println(err)
		return nil
	}
	return &p
}

func (r *redisCache) Expire(key string) {
	client := r.getClient()
	client.Del(key)
}
