// Package authtest signs bearer tokens for tests and dev tooling.
package authtest

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Token returns a compact HS256 JWT for sub expiring at exp.
func Token(secret string, sub uuid.UUID, exp time.Time) string {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   sub.String(),
		ExpiresAt: jwt.NewNumericDate(exp),
	})
	signed, err := t.SignedString([]byte(secret))
	if err != nil {
		// hmac signing of registered claims has no failure path
		panic(err)
	}
	return signed
}
