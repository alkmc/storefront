// Package outbox relays pending outbox rows to the message broker.
package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/alkmc/storefront/internal/event"
)

// maxBackoff caps the relay's wait while draining keeps failing.
const maxBackoff = 30 * time.Second

type (
	// drainer claims, publishes, and settles a batch of pending outbox rows,
	// the bool reports a full claim, signalling more rows likely wait.
	drainer interface {
		DrainBatch(ctx context.Context, batchSize, maxAttempts int,
			publish func(context.Context, event.Record) error,
		) (int, bool, error)
	}
	// waiter blocks until an outbox wakeup or the poll timeout, Run owns its lifecycle.
	waiter interface {
		Await(ctx context.Context, timeout time.Duration) error
		Close()
	}
	// publisher emits a single outbox record to the broker.
	publisher interface {
		Publish(context.Context, event.Record) error
	}
	// Config tunes the relay's batching, waiting, and publish behaviour.
	Config struct {
		BatchSize      int
		PollInterval   time.Duration
		PublishTimeout time.Duration
		MaxAttempts    int
	}
	// Relay drains the outbox to the publisher, woken by NOTIFY with a poll fallback.
	Relay struct {
		store    drainer
		listener waiter
		pub      publisher
		cfg      Config
		log      *slog.Logger
	}
)

// New returns a Relay that drains the outbox per the given config.
func New(
	store drainer, listener waiter, pub publisher, cfg Config, log *slog.Logger,
) *Relay {
	return &Relay{
		store: store, listener: listener, pub: pub, cfg: cfg, log: log,
	}
}

// Run drains the outbox until ctx is cancelled, backing off when draining fails.
// It owns the listener and closes it on return.
func (r *Relay) Run(ctx context.Context) error {
	defer r.listener.Close()

	backoff := r.cfg.PollInterval
	for {
		published, full, err := r.store.DrainBatch(
			ctx, r.cfg.BatchSize, r.cfg.MaxAttempts, r.publishWithTimeout,
		)
		if ctx.Err() != nil {
			return nil
		}

		switch {
		case err != nil:
			r.log.Error("outbox drain failed",
				slog.Int("published", published), slog.Any("error", err))
			backoff = min(backoff*2, maxBackoff)
			// Backing off deaf to NOTIFY, or steady writes would retry at their own rate.
			if err := pause(ctx, backoff); err != nil {
				return nil
			}
		case full:
			// A full claim usually means more rows wait: drain again immediately.
			backoff = r.cfg.PollInterval
		default:
			backoff = r.cfg.PollInterval
			if err := r.wait(ctx); err != nil {
				return nil
			}
		}
	}
}

// wait blocks on a NOTIFY wakeup, falling back to a plain sleep when waiting fails.
func (r *Relay) wait(ctx context.Context) error {
	err := r.listener.Await(ctx, r.cfg.PollInterval)
	if err == nil || ctx.Err() != nil {
		return ctx.Err()
	}
	r.log.Error("outbox wakeup failed", slog.Any("error", err))
	return pause(ctx, r.cfg.PollInterval)
}

// publishWithTimeout keeps a hung broker from pinning the relay's claim tx.
func (r *Relay) publishWithTimeout(ctx context.Context, rec event.Record) error {
	ctx, cancel := context.WithTimeout(ctx, r.cfg.PublishTimeout)
	defer cancel()
	return r.pub.Publish(ctx, rec)
}

// pause sleeps d out in full, only ctx cancellation cuts it short.
func pause(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
