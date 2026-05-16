package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
}

func TestSecureHeaders(t *testing.T) {
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
			mw := secureHeaders(tt.hstsEnabled, tt.hstsMaxAge)
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
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/product", nil)
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

func TestCSRFInvalidTrustedOrigin(t *testing.T) {
	if _, err := csrf([]string{"not a url"}); err == nil {
		t.Errorf("expected error for invalid trusted origin")
	}
}
