package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maxPort = 65535
	// minJWTSecretBytes follows RFC 7518 section 3.2, an HMAC key must be at least the hash output size.
	minJWTSecretBytes = 32
)

// validate rejects settings that fail silently, leaving loud failures to their consumers.
func (c Config) validate() error {
	return errors.Join(
		c.HTTP.validate(),
		c.GRPC.validate(),
		c.Postgres.validate(),
		c.Valkey.validate(),
		c.Outbox.validate(),
		c.Service.validate(),
		c.validateCacheTimeout(),
		c.Idempotency.validate(),
		c.Auth.validate(),
		// zero hands the handler a context that has already expired
		positive("REQUEST_TIMEOUT", c.RequestTimeout),
		c.validateShutdownTimeout(),
		c.validateListenAddresses(),
	)
}

// validateShutdownTimeout lets in-flight requests finish before shutdown cuts their connections.
func (c Config) validateShutdownTimeout() error {
	if minimum := c.RequestTimeout + ResponseMargin; c.ShutdownTimeout < minimum {
		return fmt.Errorf(
			"SHUTDOWN_TIMEOUT (%v) must be at least REQUEST_TIMEOUT + %v (%v)",
			c.ShutdownTimeout, ResponseMargin, minimum,
		)
	}
	return nil
}

// validateCacheTimeout leaves room for the database read within the load budget.
func (c Config) validateCacheTimeout() error {
	if c.Valkey.Timeout >= c.Service.LoadTimeout {
		return fmt.Errorf(
			"VALKEY_TIMEOUT (%v) must be less than SERVICE_LOAD_TIMEOUT (%v)",
			c.Valkey.Timeout, c.Service.LoadTimeout,
		)
	}
	return nil
}

// validateListenAddresses rejects duplicates, whose bind error would blame another process.
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
		// zero rejects every request that carries a body
		positive("HTTP_MAX_BODY_BYTES", h.MaxBodyBytes),
		// zero compresses everything, which is fine, but a negative floor is not
		notNegative("HTTP_COMPRESS_MIN_BYTES", h.CompressMinBytes),
		// zero is the canonical way to clear HSTS, a negative age builds an invalid header
		notNegative("HSTS_MAX_AGE", h.HSTSMaxAge),
		exactOrigins("CORS_ALLOWED_ORIGINS", h.CORSAllowedOrigins),
	)
}

func (g GRPC) validate() error {
	return errors.Join(
		listenPort("GRPC_PORT", g.Port),
		// zero makes the server reject every message that carries a payload
		positive("GRPC_MAX_REQUEST_BYTES", g.MaxRequestBytes),
	)
}

func (p Postgres) validate() error {
	return errors.Join(
		positive("PG_MAX_OPEN_CONNS", p.MaxOpenConns),
		notNegative("PG_MAX_IDLE_CONNS", p.MaxIdleConns),
		// zero expires every connection on creation, and the pool then blames its own hooks
		positive("PG_CONN_MAX_LIFETIME", p.ConnMaxLifetime),
		p.validateConnLimits(),
	)
}

// validateConnLimits rejects more idle than open conns, which pgxpool doesn't check.
func (p Postgres) validateConnLimits() error {
	if p.MaxIdleConns > p.MaxOpenConns {
		return fmt.Errorf(
			"PG_MAX_IDLE_CONNS (%d) must not exceed PG_MAX_OPEN_CONNS (%d)",
			p.MaxIdleConns, p.MaxOpenConns,
		)
	}
	return nil
}

func (v Valkey) validate() error {
	return errors.Join(
		// the cache truncates a TTL to whole milliseconds and Valkey rejects a PX of zero
		atLeast("VALKEY_CACHE_TTL", v.TTL, time.Millisecond),
		atLeast("VALKEY_CACHE_NEG_TTL", v.NegTTL, time.Millisecond),
		positive("VALKEY_TIMEOUT", v.Timeout),
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
	// zero cancels every detached fill, so the cache never gets populated
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

// bounded lists the setting types the checks below accept, with ~int64 covering time.Duration.
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

// exactOrigins rejects wildcards, CSRF matches trusted origins literally so a pattern blocks every write.
func exactOrigins(name string, origins []string) error {
	for _, o := range origins {
		if strings.Contains(o, "*") {
			return fmt.Errorf("%s must list exact origins, got %q", name, o)
		}
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
