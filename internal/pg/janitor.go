package pg

import (
	"context"
	"log/slog"
	"time"
)

// purger deletes rows past their retention window, reporting how many went.
type purger interface {
	PurgeIdempotencyKeys(context.Context) (int64, error)
}

// Janitor periodically purges expired idempotency keys until its context is cancelled.
type Janitor struct {
	purger   purger
	interval time.Duration
	logger   *slog.Logger
}

// NewJanitor returns a Janitor that purges once per interval.
func NewJanitor(p purger, interval time.Duration, logger *slog.Logger) *Janitor {
	return new(Janitor{purger: p, interval: interval, logger: logger})
}

// Run purges after each interval until ctx is cancelled, logging a failure without stopping.
func (j *Janitor) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(j.interval):
			n, err := j.purger.PurgeIdempotencyKeys(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				j.logger.Warn("idempotency purge failed", slog.Any("error", err))
				continue
			}
			if n > 0 {
				j.logger.Info("purged expired idempotency keys", slog.Int64("count", n))
			}
		}
	}
}
