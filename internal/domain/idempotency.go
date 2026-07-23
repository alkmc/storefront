package domain

import "errors"

// ErrIdempotencyMismatch signals a reused idempotency key paired with a different request payload.
var ErrIdempotencyMismatch = errors.New("domain: idempotency key reused with different payload")

// MaxIdempotencyKeyLen bounds the opaque key both transports accept.
const MaxIdempotencyKeyLen = 255

// IdempotencyKey is the caller-supplied opaque key that makes order creation replay-safe.
type IdempotencyKey string
