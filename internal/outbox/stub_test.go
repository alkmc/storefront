package outbox

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/alkmc/storefront/internal/event"
)

type (
	counter struct {
		n atomic.Int64
	}
	fakeDrainer struct {
		counter
		batchSizes []int
		errs       []error
	}
	fakeListener struct {
		counter
		wake   chan struct{}
		errs   []error
		closes counter
	}
	fakePublisher struct {
		counter
		failAfter  int64
		poisonCall int64
		shouldHang bool
	}
)

func (c *counter) inc() int64 {
	return c.n.Add(1)
}

func (c *counter) callCount() int64 {
	return c.n.Load()
}

func (f *fakeDrainer) DrainBatch(
	ctx context.Context, batchSize, _ int, publish func(context.Context, event.Record) error,
) (int, bool, error) {
	count, err := f.result(int(f.inc()))
	if err != nil {
		return 0, false, err
	}
	full := count == batchSize
	published := 0
	for range count {
		err := publish(ctx, event.Record{})
		if err == nil {
			published++
			continue
		}
		if !errors.Is(err, event.ErrUndeliverable) {
			return published, full, err
		}
	}
	return published, full, nil
}

// result returns the outcome scripted for the n-th call, an empty poll once the script runs out.
func (f *fakeDrainer) result(n int) (count int, err error) {
	if n <= len(f.batchSizes) {
		count = f.batchSizes[n-1]
	}
	if n <= len(f.errs) {
		err = f.errs[n-1]
	}
	return count, err
}

// Await mimics the store listener: a wake signal or the timeout ends the wait.
func (f *fakeListener) Await(ctx context.Context, timeout time.Duration) error {
	i := int(f.inc()) - 1
	if i < len(f.errs) && f.errs[i] != nil {
		return f.errs[i]
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.wake:
		return nil
	case <-time.After(timeout):
		return nil
	}
}

func (p *fakePublisher) Publish(ctx context.Context, _ event.Record) error {
	calls := p.inc()
	if p.shouldHang {
		<-ctx.Done()
		return ctx.Err()
	}
	if calls == p.poisonCall {
		return event.ErrUndeliverable
	}
	if p.failAfter > 0 && calls > p.failAfter {
		return errors.New("publish error")
	}
	return nil
}

func (f *fakeListener) Close() {
	f.closes.inc()
}
