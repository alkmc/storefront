package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alkmc/storefront/internal/auth"
	"github.com/alkmc/storefront/internal/auth/authtest"
	"github.com/google/uuid"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
}

func TestSecurityHeaders(t *testing.T) {
	static := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "frame-ancestors 'none'",
	}

	tests := []struct {
		name        string
		hstsEnabled bool
		hstsMaxAge  int
		wantHSTS    string
	}{
		{
			name:        "hsts disabled",
			hstsEnabled: false,
			wantHSTS:    "",
		},
		{
			name:        "hsts enabled",
			hstsEnabled: true,
			hstsMaxAge:  31536000,
			wantHSTS:    "max-age=31536000; includeSubDomains",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := securityHeaders(tt.hstsEnabled, tt.hstsMaxAge)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			mw(okHandler()).ServeHTTP(rec, req)

			for k, v := range static {
				if got := rec.Header().Get(k); got != v {
					t.Errorf("%s: got %q, want %q", k, got, v)
				}
			}
			if got := rec.Header().Get("Strict-Transport-Security"); got != tt.wantHSTS {
				t.Errorf("HSTS: got %q, want %q", got, tt.wantHSTS)
			}
		})
	}
}

func TestCSRF(t *testing.T) {
	trusted := "https://app.example.com"
	untrusted := "https://evil.example.com"

	mw, err := csrf([]string{trusted})
	if err != nil {
		t.Fatalf("csrf init: %v", err)
	}

	tests := []struct {
		name        string
		origin      string
		wantStatus  int
		wantCalled  bool
		wantBodyHas string
	}{
		{
			name:       "trusted origin registered",
			origin:     trusted,
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:        "untrusted blocked by custom JSON deny handler",
			origin:      untrusted,
			wantStatus:  http.StatusForbidden,
			wantBodyHas: `"message":"cross-origin request rejected"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				called = true
			})
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/products", nil)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			req.Header.Set("Origin", tt.origin)
			rec := httptest.NewRecorder()
			mw(next).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d", rec.Code, tt.wantStatus)
			}
			if called != tt.wantCalled {
				t.Errorf("handler called: got %v, want %v", called, tt.wantCalled)
			}
			if tt.wantBodyHas != "" {
				if got := rec.Header().Get("Content-Type"); got != MediaTypeJSON {
					t.Errorf("content-type: got %q, want %q", got, MediaTypeJSON)
				}
				if body := rec.Body.String(); !strings.Contains(body, tt.wantBodyHas) {
					t.Errorf("body missing %q: %q", tt.wantBodyHas, body)
				}
			}
		})
	}
}

func TestCSRFRejectsInvalidTrustedOrigin(t *testing.T) {
	if _, err := csrf([]string{"not a url"}); err == nil {
		t.Errorf("expected error for invalid trusted origin")
	}
}

func TestAuth(t *testing.T) {
	sub := uuid.Must(uuid.NewV7())
	valid := authtest.Token(testJWTSecret, sub, time.Now().Add(time.Hour))

	tests := []struct {
		name       string
		header     string
		wantStatus int
	}{
		{
			name:       "missing header",
			header:     "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "basic scheme",
			header:     "Basic abc",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "bearer without token",
			header:     "Bearer ",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "expired token",
			header:     "Bearer " + authtest.Token(testJWTSecret, sub, time.Now().Add(-time.Minute)),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "valid token",
			header:     "Bearer " + valid,
			wantStatus: http.StatusOK,
		},
		{
			name:       "lowercase scheme accepted",
			header:     "bearer " + valid,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				gotUser uuid.UUID
				called  bool
			)
			next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				called = true
				gotUser, _ = auth.UserID(r.Context())
			})
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/orders", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			Auth(auth.NewVerifier(testJWTSecret))(next).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK {
				if !called {
					t.Fatal("handler not called for a valid token")
				}
				if gotUser != sub {
					t.Errorf("got user %v in context, want %v", gotUser, sub)
				}
				return
			}
			if called {
				t.Error("handler called despite rejected token")
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
				t.Errorf("got WWW-Authenticate %q, want %q", got, "Bearer")
			}
		})
	}
}

func TestCORS(t *testing.T) {
	mw, err := corsPolicy([]string{"https://app.example.com"}, 600)
	if err != nil {
		t.Fatalf("cors init: %v", err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/v1/orders", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", "authorization")
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, req)

	allowed := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
	if !strings.Contains(allowed, "authorization") {
		t.Errorf("preflight does not allow Authorization: %q", allowed)
	}
}
