package service

import (
	"context"
	"time"

	"golang.org/x/sync/singleflight"
)

// callGroup deduplicates concurrent calls for the same key into one typed call.
type callGroup[T any] struct {
	group   singleflight.Group
	timeout time.Duration
}

// Do runs fn once per in-flight key and shares the result with every waiter.
// The call is detached from its caller, a client disconnect must not fail the waiters.
func (c *callGroup[T]) Do(
	ctx context.Context, key string, fn func(context.Context) (T, error),
) (T, error) {
	v, err, _ := c.group.Do(key, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.timeout)
		defer cancel()
		return fn(ctx)
	})
	result, _ := v.(T)
	return result, err
}
