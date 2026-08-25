package outbox

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

// epsilon is the timing slack used to pin exact backoff deadlines.
const epsilon = 100 * time.Millisecond

func TestRelay(t *testing.T) {
	t.Run("StandardPolling", func(t *testing.T) {
		fd := &fakeDrainer{batchSizes: []int{0, 0}}
		cfg := testConfig()
		runRelay(t, fd, &fakeListener{}, &fakePublisher{}, cfg, func(t *testing.T) {
			synctest.Wait()
			assertDrainCalls(t, fd, 1)

			synctest.Sleep(cfg.PollInterval)
			assertDrainCalls(t, fd, 2)
		})
	})

	t.Run("FullBatch", func(t *testing.T) {
		fd := &fakeDrainer{batchSizes: []int{5, 5, 0}}
		runRelay(t, fd, &fakeListener{}, &fakePublisher{}, testConfig(), func(t *testing.T) {
			// Full batches re-drain immediately: all 3 land before settle, no sleep.
			synctest.Wait()
			assertDrainCalls(t, fd, 3)
		})
	})

	t.Run("FullClaimPartialPublish", func(t *testing.T) {
		fd := &fakeDrainer{batchSizes: []int{5, 5, 0}}
		pub := &fakePublisher{poisonCall: 2}
		runRelay(t, fd, &fakeListener{}, pub, testConfig(), func(t *testing.T) {
			// Full claims re-drain immediately even when a poison row goes unpublished.
			synctest.Wait()
			assertDrainCalls(t, fd, 3)
			assertPublishCalls(t, pub, 10)
		})
	})

	t.Run("Wakeup", func(t *testing.T) {
		fd := &fakeDrainer{batchSizes: []int{0, 0}}
		fl := &fakeListener{}
		cfg := testConfig()
		runRelay(t, fd, fl, &fakePublisher{}, cfg, func(t *testing.T) {
			synctest.Wait()
			assertDrainCalls(t, fd, 1)

			// A NOTIFY mid-wait re-drains well before the poll tick.
			synctest.Sleep(cfg.PollInterval / 4)
			fl.wake <- struct{}{}
			synctest.Wait()
			assertDrainCalls(t, fd, 2)
		})
	})

	t.Run("WakeupErrorFallsBackToSleep", func(t *testing.T) {
		fd := &fakeDrainer{}
		fl := &fakeListener{errs: []error{errors.New("listen failed")}}
		cfg := testConfig()
		runRelay(t, fd, fl, &fakePublisher{}, cfg, func(t *testing.T) {
			synctest.Wait()
			assertDrainCalls(t, fd, 1)

			// The broken wait does not hot-loop: the next drain still lands one tick later.
			synctest.Sleep(cfg.PollInterval - epsilon)
			assertDrainCalls(t, fd, 1)
			synctest.Sleep(epsilon)
			assertDrainCalls(t, fd, 2)
		})
	})

	t.Run("ExponentialBackoff", func(t *testing.T) {
		fd := &fakeDrainer{
			errs: []error{errors.New("drain error"), errors.New("drain error"), nil},
		}
		cfg := testConfig()
		runRelay(t, fd, &fakeListener{}, &fakePublisher{}, cfg, func(t *testing.T) {
			synctest.Wait()
			assertDrainCalls(t, fd, 1)

			// 1st backoff = 2 * PollInterval = 2s
			backoff1 := cfg.PollInterval * 2
			synctest.Sleep(backoff1 - epsilon)
			assertDrainCalls(t, fd, 1)
			synctest.Sleep(epsilon)
			assertDrainCalls(t, fd, 2)

			// 2nd backoff = 4 * PollInterval = 4s
			backoff2 := cfg.PollInterval * 4
			synctest.Sleep(backoff2 - epsilon)
			assertDrainCalls(t, fd, 2)
			synctest.Sleep(epsilon)
			assertDrainCalls(t, fd, 3)

			// Success resets backoff back to PollInterval = 1s
			synctest.Sleep(cfg.PollInterval)
			assertDrainCalls(t, fd, 4)
		})
	})

	t.Run("BackoffCap", func(t *testing.T) {
		fd := &fakeDrainer{errs: slices.Repeat([]error{errors.New("drain failed")}, 6)}
		cfg := testConfig()
		runRelay(t, fd, &fakeListener{}, &fakePublisher{}, cfg, func(t *testing.T) {
			// 1st poll (fail) -> wait 2s
			synctest.Wait()
			// 2nd poll (fail) -> wait 4s
			synctest.Sleep(cfg.PollInterval * 2)
			// 3rd poll (fail) -> wait 8s
			synctest.Sleep(cfg.PollInterval * 4)
			// 4th poll (fail) -> wait 16s
			synctest.Sleep(cfg.PollInterval * 8)
			// 5th poll (fail) -> wait is capped at maxBackoff
			synctest.Sleep(cfg.PollInterval * 16)
			// 6th poll (fail) -> wait is capped at maxBackoff
			synctest.Sleep(maxBackoff)

			assertDrainCalls(t, fd, 6)

			// Wait after the 6th poll should be capped at maxBackoff
			synctest.Sleep(maxBackoff - epsilon)
			assertDrainCalls(t, fd, 6)

			synctest.Sleep(epsilon)
			assertDrainCalls(t, fd, 7)
		})
	})

	t.Run("BackoffIgnoresWakeup", func(t *testing.T) {
		fd := &fakeDrainer{errs: []error{errors.New("drain failed")}}
		fl := &fakeListener{}
		cfg := testConfig()
		runRelay(t, fd, fl, &fakePublisher{}, cfg, func(t *testing.T) {
			synctest.Wait()
			assertDrainCalls(t, fd, 1)

			// The failed drain backs off on a plain timer and never touches the NOTIFY listener.
			synctest.Sleep(cfg.PollInterval*2 - epsilon)
			if got := fl.callCount(); got != 0 {
				t.Errorf("Await calls during backoff = %d, want 0", got)
			}
			assertDrainCalls(t, fd, 1)
			synctest.Sleep(epsilon)
			assertDrainCalls(t, fd, 2)
		})
	})

	t.Run("PublishTimeout", func(t *testing.T) {
		fd := &fakeDrainer{batchSizes: []int{1, 0}}
		pub := &fakePublisher{shouldHang: true}
		cfg := testConfig()
		cfg.PublishTimeout = 3 * time.Second
		runRelay(t, fd, &fakeListener{}, pub, cfg, func(t *testing.T) {
			start := time.Now()
			synctest.Wait()

			synctest.Sleep(cfg.PublishTimeout)

			if got := time.Since(start); got != cfg.PublishTimeout {
				t.Errorf("expected %v to timeout, got %v", cfg.PublishTimeout, got)
			}
			assertPublishCalls(t, pub, 1)
		})
	})

	t.Run("PartialPublishError", func(t *testing.T) {
		fd := &fakeDrainer{batchSizes: []int{5}}
		pub := &fakePublisher{failAfter: 2} // first 2 succeed, 3rd fails
		runRelay(t, fd, &fakeListener{}, pub, testConfig(), func(t *testing.T) {
			synctest.Wait()
			assertDrainCalls(t, fd, 1)
			assertPublishCalls(t, pub, 3)
		})
	})

	t.Run("GracefulShutdown", func(t *testing.T) {
		fd := &fakeDrainer{batchSizes: []int{0}}
		cfg := testConfig()
		cfg.PollInterval = 10 * time.Second

		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			done := make(chan error, 1)

			fl := &fakeListener{wake: make(chan struct{})}
			r := New(fd, fl, &fakePublisher{}, cfg, slog.New(slog.DiscardHandler))
			go func() {
				done <- r.Run(ctx)
			}()

			synctest.Wait()
			assertDrainCalls(t, fd, 1)

			cancel()

			select {
			case err := <-done:
				if err != nil {
					t.Errorf("expected nil error on shutdown, got %v", err)
				}
			case <-time.After(1 * time.Second):
				t.Fatal("relay did not shut down within 1s")
			}

			if got := fl.closes.callCount(); got != 1 {
				t.Errorf("listener Close calls on shutdown = %d, want 1", got)
			}
		})
	})
}

func testConfig() Config {
	return Config{BatchSize: 5, PollInterval: time.Second, PublishTimeout: 5 * time.Second}
}

// runRelay runs body in a synctest bubble with a started relay and a cancellable ctx.
func runRelay(
	t *testing.T, fd *fakeDrainer, fl *fakeListener, pub *fakePublisher, cfg Config, body func(*testing.T),
) {
	t.Helper()
	synctest.Test(t, func(t *testing.T) {
		// A select on a channel made outside the bubble is not durably blocking and hangs synctest.Wait.
		fl.wake = make(chan struct{})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		r := New(fd, fl, pub, cfg, slog.New(slog.DiscardHandler))
		go func() { _ = r.Run(ctx) }()
		body(t)
	})
}

func assertDrainCalls(t *testing.T, fd *fakeDrainer, want int64) {
	t.Helper()
	if got := fd.callCount(); got != want {
		t.Errorf("DrainBatch calls = %d, want %d", got, want)
	}
}

func assertPublishCalls(t *testing.T, pub *fakePublisher, want int64) {
	t.Helper()
	if got := pub.callCount(); got != want {
		t.Errorf("publish calls = %d, want %d", got, want)
	}
}
