package pg

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

const testInterval = time.Hour

type fakePurger struct {
	calls   atomic.Int64
	failing bool
}

func (f *fakePurger) PurgeIdempotencyKeys(context.Context) (int64, error) {
	f.calls.Add(1)
	if f.failing {
		return 0, errors.New("purge boom")
	}
	return 0, nil
}

func TestJanitor_PurgesEachInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fp := &fakePurger{}
		j := NewJanitor(fp, testInterval, slog.New(slog.DiscardHandler))
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() { _ = j.Run(ctx) }()

		synctest.Wait()
		if got := fp.calls.Load(); got != 0 {
			t.Fatalf("purge before first interval = %d, want 0", got)
		}

		time.Sleep(testInterval)
		synctest.Wait()
		if got := fp.calls.Load(); got != 1 {
			t.Errorf("purge after 1 interval = %d, want 1", got)
		}

		time.Sleep(testInterval)
		synctest.Wait()
		if got := fp.calls.Load(); got != 2 {
			t.Errorf("purge after 2 intervals = %d, want 2", got)
		}
	})
}

func TestJanitor_StopsOnContextCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		j := NewJanitor(&fakePurger{}, testInterval, slog.New(slog.DiscardHandler))
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- j.Run(ctx) }()

		synctest.Wait()
		cancel()
		synctest.Wait()

		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run returned %v, want nil", err)
			}
		default:
			t.Fatal("Run did not return after cancel")
		}
	})
}

func TestJanitor_ContinuesAfterError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fp := &fakePurger{failing: true} // purge fails, the loop must keep going
		j := NewJanitor(fp, testInterval, slog.New(slog.DiscardHandler))
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() { _ = j.Run(ctx) }()

		time.Sleep(testInterval)
		synctest.Wait()
		if got := fp.calls.Load(); got != 1 {
			t.Fatalf("after 1 interval = %d, want 1 (errored)", got)
		}
		time.Sleep(testInterval)
		synctest.Wait()
		if got := fp.calls.Load(); got != 2 {
			t.Errorf("after an error the loop stopped: calls = %d, want 2", got)
		}
	})
}
