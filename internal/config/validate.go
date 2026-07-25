package config

import (
	"errors"
	"fmt"
	"time"
)

const (
	maxPort = 65535
	// minJWTSecretBytes follows RFC 7518 section 3.2, an HMAC key must match the hash output size.
	minJWTSecretBytes = 32
)

// validate rejects settings that fail silently, those that fail loudly stay with their consumers.
func (c Config) validate() error {
	return errors.Join(
		c.HTTP.validate(),
		c.GRPC.validate(),
		c.Postgres.validate(),
		c.Redis.validate(),
		c.Outbox.validate(),
		c.Service.validate(),
		c.Idempotency.validate(),
		c.Auth.validate(),
		// an expired context leaves in flight connections undrained on the way out
		positive("SHUTDOWN_TIMEOUT", c.ShutdownTimeout),
		c.validateListenAddresses(),
	)
}

// validateListenAddresses keeps the listeners apart, a bind collision blames a foreign process.
func (c Config) validateListenAddresses() error {
	seen := make(map[string]struct{}, 3)
	for _, addr := range []string{c.HTTP.Address(), c.HTTP.InternalAddress(), c.GRPC.Address()} {
		if _, dup := seen[addr]; dup {
			return fmt.Errorf(
				"HTTP_PORT, HTTP_INTERNAL_PORT and GRPC_PORT must resolve to distinct listen addresses, %q repeats",
				addr,
			)
		}
		seen[addr] = struct{}{}
	}
	return nil
}

func (h HTTP) validate() error {
	return errors.Join(
		listenPort("HTTP_PORT", h.Port),
		listenPort("HTTP_INTERNAL_PORT", h.InternalPort),
		// net/http reads zero as no timeout, so the limit would vanish without a word
		positive("HTTP_READ_TIMEOUT", h.ReadTimeout),
		positive("HTTP_WRITE_TIMEOUT", h.WriteTimeout),
		positive("HTTP_IDLE_TIMEOUT", h.IdleTimeout),
		// zero hands the handler a context that already expired
		positive("HTTP_REQUEST_TIMEOUT", h.RequestTimeout),
		// zero rejects every request that carries a body
		positive("HTTP_MAX_BODY_BYTES", h.MaxBodyBytes),
		// zero compresses everything, which is a choice, a negative floor is not
		notNegative("HTTP_COMPRESS_MIN_BYTES", h.CompressMinBytes),
		// zero is the canonical way to clear HSTS, a negative age builds an invalid header
		notNegative("HSTS_MAX_AGE", h.HSTSMaxAge),
		h.validateResponseDeadline(),
	)
}

// validateResponseDeadline keeps the write deadline past the handler budget, or a response gets cut.
func (h HTTP) validateResponseDeadline() error {
	if h.WriteTimeout <= h.RequestTimeout {
		return fmt.Errorf(
			"HTTP_WRITE_TIMEOUT (%v) must exceed HTTP_REQUEST_TIMEOUT (%v)",
			h.WriteTimeout, h.RequestTimeout,
		)
	}
	return nil
}

func (g GRPC) validate() error {
	return errors.Join(
		listenPort("GRPC_PORT", g.Port),
		positive("GRPC_REQUEST_TIMEOUT", g.RequestTimeout),
		// zero makes the server reject every message that carries a payload
		positive("GRPC_MAX_REQUEST_BYTES", g.MaxRequestBytes),
	)
}

func (p Postgres) validate() error {
	return errors.Join(
		positive("PG_MAX_OPEN_CONNS", p.MaxOpenConns),
		notNegative("PG_MAX_IDLE_CONNS", p.MaxIdleConns),
		// zero expires every connection on creation, then the pool fails blaming its own hooks
		positive("PG_CONN_MAX_LIFETIME", p.ConnMaxLifetime),
		p.validateConnLimits(),
	)
}

// validateConnLimits catches an idle floor above the open cap, a relation pgxpool never checks.
func (p Postgres) validateConnLimits() error {
	if p.MaxIdleConns > p.MaxOpenConns {
		return fmt.Errorf(
			"PG_MAX_IDLE_CONNS (%d) must not exceed PG_MAX_OPEN_CONNS (%d)",
			p.MaxIdleConns, p.MaxOpenConns,
		)
	}
	return nil
}

func (r Redis) validate() error {
	return errors.Join(
		// the cache truncates a TTL to whole milliseconds and Redis rejects a PX of zero
		atLeast("REDIS_CACHE_TTL", r.TTL, time.Millisecond),
		atLeast("REDIS_CACHE_NEG_TTL", r.NegTTL, time.Millisecond),
	)
}

func (o Outbox) validate() error {
	return errors.Join(
		// zero claims no rows, so the relay idles forever without draining or logging
		positive("OUTBOX_BATCH_SIZE", o.BatchSize),
		// the interval seeds the retry backoff, zero turns a failing drain into a busy loop
		positive("OUTBOX_POLL_INTERVAL", o.PollInterval),
		positive("OUTBOX_PUBLISH_TIMEOUT", o.PublishTimeout),
		// zero sends the first permanent failure straight to the dead-letter table
		positive("OUTBOX_MAX_ATTEMPTS", o.MaxAttempts),
	)
}

func (s Service) validate() error {
	// zero expires every detached read fill, so the cache never fills
	return positive("SERVICE_LOAD_TIMEOUT", s.LoadTimeout)
}

func (i Idempotency) validate() error {
	return errors.Join(
		// zero stores a key that is already expired, so a retry buys again instead of replaying
		positive("IDEMPOTENCY_TTL", i.TTL),
		// zero makes the janitor purge in a tight loop against the database
		positive("IDEMPOTENCY_PURGE_INTERVAL", i.PurgeInterval),
	)
}

// validate rejects a signing key too weak for HS256, an empty one would let anyone forge a token.
func (a Auth) validate() error {
	if n := len(a.JWTSecret.Reveal()); n < minJWTSecretBytes {
		return fmt.Errorf("AUTH_JWT_SECRET must be at least %d bytes, got %d", minJWTSecretBytes, n)
	}
	return nil
}

// bounded is every setting type the checks below accept, time.Duration included through ~int64.
type bounded interface {
	~int | ~int32 | ~int64
}

func positive[T bounded](name string, v T) error {
	if v <= 0 {
		return fmt.Errorf("%s must be greater than zero, got %v", name, v)
	}
	return nil
}

func notNegative[T bounded](name string, v T) error {
	if v < 0 {
		return fmt.Errorf("%s must not be negative, got %v", name, v)
	}
	return nil
}

func atLeast[T bounded](name string, v, floor T) error {
	if v < floor {
		return fmt.Errorf("%s must be at least %v, got %v", name, floor, v)
	}
	return nil
}

// listenPort rejects zero, which binds a random port the rest of the system cannot reach.
func listenPort(name string, v int) error {
	if v < 1 || v > maxPort {
		return fmt.Errorf("%s must be between 1 and %d, got %d", name, maxPort, v)
	}
	return nil
}
