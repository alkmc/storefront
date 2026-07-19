package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/alkmc/storefront/internal/auth/authtest"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testSecret = "test-secret"

// sign builds an adversarial token with the library instead of the production signer.
func sign(t *testing.T, method jwt.SigningMethod, key any, claims jwt.MapClaims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func TestVerifier_Verify(t *testing.T) {
	sub := uuid.New()
	future := time.Now().Add(time.Hour).Unix()
	hmacKey := []byte(testSecret)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:  "valid",
			token: authtest.Token(testSecret, sub, time.Now().Add(time.Hour)),
		},
		{
			name: "unknown claims ignored",
			token: sign(t, jwt.SigningMethodHS256, hmacKey,
				jwt.MapClaims{"sub": sub.String(), "exp": future, "iss": "other"}),
		},
		{
			name:    "expired",
			token:   authtest.Token(testSecret, sub, time.Now().Add(-time.Minute)),
			wantErr: true,
		},
		{
			name:    "wrong secret",
			token:   authtest.Token("other-secret", sub, time.Now().Add(time.Hour)),
			wantErr: true,
		},
		{
			name: "alg none rejected",
			token: sign(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType,
				jwt.MapClaims{"sub": sub.String(), "exp": future}),
			wantErr: true,
		},
		{
			name: "alg RS256 rejected despite a valid signature",
			token: sign(t, jwt.SigningMethodRS256, rsaKey,
				jwt.MapClaims{"sub": sub.String(), "exp": future}),
			wantErr: true,
		},
		{
			name:    "garbage base64",
			token:   "!!!.!!!.!!!",
			wantErr: true,
		},
		{
			name:    "wrong segment count",
			token:   "onlyone.two",
			wantErr: true,
		},
		{
			name:    "missing sub",
			token:   sign(t, jwt.SigningMethodHS256, hmacKey, jwt.MapClaims{"exp": future}),
			wantErr: true,
		},
		{
			name: "sub not a uuid",
			token: sign(t, jwt.SigningMethodHS256, hmacKey,
				jwt.MapClaims{"sub": "admin", "exp": future}),
			wantErr: true,
		},
		{
			name:    "missing exp",
			token:   sign(t, jwt.SigningMethodHS256, hmacKey, jwt.MapClaims{"sub": sub.String()}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewVerifier(testSecret).Verify(tt.token)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidToken) {
					t.Fatalf("got %v, want ErrInvalidToken", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != sub {
				t.Errorf("sub: got %v, want %v", got, sub)
			}
		})
	}
}

func TestUserID(t *testing.T) {
	id := uuid.New()
	if got, ok := UserID(WithUserID(t.Context(), id)); !ok || got != id {
		t.Errorf("got %v ok %v, want %v ok true", got, ok, id)
	}
	if _, ok := UserID(t.Context()); ok {
		t.Error("expected no user id in a fresh context")
	}
}
