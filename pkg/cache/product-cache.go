package cache

import (
	"context"

	"github.com/alkmc/restClean/pkg/entity"
)

// Cache is responsible for caching mechanism
type Cache interface {
	Set(ctx context.Context, key string, value *entity.Product)
	Get(ctx context.Context, key string) *entity.Product
	Expire(ctx context.Context, key string)
}
