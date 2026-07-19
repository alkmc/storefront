// Package auth verifies bearer tokens and carries the caller identity in context.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ErrInvalidToken covers every verification failure, transports reply 401 without detail.
var ErrInvalidToken = errors.New("invalid token")

// Verifier checks HS256 JWTs signed with a shared secret.
type Verifier struct {
	secret []byte
	parser *jwt.Parser
}

// NewVerifier builds a Verifier around the shared secret.
func NewVerifier(secret string) *Verifier {
	return new(Verifier{
		secret: []byte(secret),
		// the allowlist keeps alg none and RS256 key confusion away from the signature check
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithExpirationRequired(),
		),
	})
}

// Verify validates a compact HS256 JWT and returns its subject.
func (v *Verifier) Verify(token string) (uuid.UUID, error) {
	var c jwt.RegisteredClaims
	if _, err := v.parser.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) {
		return v.secret, nil
	}); err != nil {
		return uuid.Nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	sub, err := uuid.Parse(c.Subject)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: sub is not a uuid: %w", ErrInvalidToken, err)
	}
	return sub, nil
}

// BearerToken extracts the token from an RFC 6750 Authorization value, the scheme match ignores case.
func BearerToken(header string) (string, bool) {
	const scheme = "Bearer "
	if len(header) < len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return "", false
	}
	return header[len(scheme):], true
}

// ctxKey is unexported so only this package can set the identity.
type ctxKey struct{}

// WithUserID returns a context carrying the authenticated caller id.
func WithUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// UserID returns the authenticated caller id, ok is false when the context has none.
func UserID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ctxKey{}).(uuid.UUID)
	return id, ok
}
