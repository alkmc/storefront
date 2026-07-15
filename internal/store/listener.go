package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// closeTimeout bounds the TERMINATE handshake of the hijacked connection.
const closeTimeout = time.Second

// Listener owns a dedicated LISTEN connection, single goroutine use only.
type Listener struct {
	pool    *pgxpool.Pool
	channel string
	conn    *pgx.Conn
}

// NewListener returns a listener for channel, the connection is acquired lazily on first Await.
func NewListener(pool *pgxpool.Pool, channel string) *Listener {
	return new(Listener{pool: pool, channel: channel})
}

// Await waits for a NOTIFY or timeout (a poll tick), both return nil.
func (l *Listener) Await(ctx context.Context, timeout time.Duration) error {
	conn, err := l.acquire(ctx)
	if err != nil {
		return err
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err := conn.WaitForNotification(waitCtx); err != nil {
		if ctx.Err() != nil {
			l.release(ctx)
			return ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		l.release(ctx)
		return fmt.Errorf("listen wait: %w", err)
	}
	return nil
}

// Close ends the LISTEN connection so the next wait acquires a fresh one.
func (l *Listener) Close() {
	l.release(context.Background())
}

// release closes the connection detached from ctx cancellation so TERMINATE still goes out.
func (l *Listener) release(ctx context.Context) {
	if l.conn == nil {
		return
	}
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), closeTimeout)
	defer cancel()
	_ = l.conn.Close(closeCtx)
	l.conn = nil
}

// acquire returns the LISTEN connection, held between waits to buffer NOTIFYs sent meanwhile.
func (l *Listener) acquire(ctx context.Context) (*pgx.Conn, error) {
	if l.conn != nil {
		return l.conn, nil
	}
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("listen acquire: %w", err)
	}
	// LISTEN takes no bind parameters, so the channel is inlined as a sanitized identifier.
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{l.channel}.Sanitize()); err != nil {
		conn.Release()
		return nil, fmt.Errorf("listen %s: %w", l.channel, err)
	}
	// Hijacked out of the pool, a long held pool conn would block pool.Close forever.
	l.conn = conn.Hijack()
	return l.conn, nil
}
