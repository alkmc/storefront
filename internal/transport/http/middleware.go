package http

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strconv"
	"time"
	"uuid"

	"github.com/alkmc/storefront/internal/auth"
	"github.com/jub0bs/cors"
	"github.com/klauspost/compress/gzhttp"
)

type (
	// MiddlewareCfg carries the transport-level knobs the middleware chain needs.
	MiddlewareCfg struct {
		MaxBodyBytes       int64
		CompressMinBytes   int
		CORSAllowedOrigins []string
		CORSMaxAge         int
		HSTSEnabled        bool
		HSTSMaxAge         int
	}
	Middleware = func(http.Handler) http.Handler
	// verifier checks a bearer token and returns the caller id.
	verifier interface {
		Verify(string) (uuid.UUID, error)
	}
)

// NewMiddleware builds the standard middleware chain.
func NewMiddleware(cfg MiddlewareCfg) (Middleware, error) {
	compressMW, err := compression(cfg.CompressMinBytes)
	if err != nil {
		return nil, err
	}
	csrfMW, err := csrf(cfg.CORSAllowedOrigins)
	if err != nil {
		return nil, err
	}
	corsMW, err := corsPolicy(cfg.CORSAllowedOrigins, cfg.CORSMaxAge)
	if err != nil {
		return nil, err
	}

	return func(next http.Handler) http.Handler {
		return chain(
			next,
			logging,
			recovery,
			securityHeaders(cfg.HSTSEnabled, cfg.HSTSMaxAge),
			corsMW,
			csrfMW,
			bodyLimit(cfg.MaxBodyBytes),
			compressMW,
		)
	}, nil
}

// chain composes middlewares in top-down order
func chain(h http.Handler, mws ...Middleware) http.Handler {
	for _, mw := range slices.Backward(mws) {
		h = mw(h)
	}
	return h
}

// logging logs method, path, status, and request duration.
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := new(statusRecorder{ResponseWriter: w, status: http.StatusOK})
		defer func() {
			slog.Default().Info(
				"http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", time.Since(start)),
			)
		}()
		next.ServeHTTP(rec, r)
	})
}

// recovery catches panics and prevents the server from crashing
func recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Default().Error(
					"panic recovered",
					slog.Any("error", err),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("stack", string(debug.Stack())),
				)
				respondError(w, http.StatusInternalServerError, msgInternalError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// securityHeaders sets baseline OWASP response headers on every response.
func securityHeaders(hstsEnabled bool, hstsMaxAge int) Middleware {
	var hsts string
	if hstsEnabled {
		hsts = "max-age=" + strconv.Itoa(hstsMaxAge) + "; includeSubDomains"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Content-Security-Policy", "frame-ancestors 'none'")
			if hsts != "" {
				h.Set("Strict-Transport-Security", hsts)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// corsPolicy enforces an origin allowlist. Empty list disables CORS entirely.
func corsPolicy(origins []string, maxAge int) (Middleware, error) {
	if len(origins) == 0 {
		return func(next http.Handler) http.Handler {
			return next
		}, nil
	}
	m, err := cors.NewMiddleware(cors.Config{
		Origins:         origins,
		Methods:         []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete},
		RequestHeaders:  []string{"Content-Type", "Authorization"},
		MaxAgeInSeconds: maxAge,
	})
	if err != nil {
		return nil, fmt.Errorf("cors: %w", err)
	}
	return m.Wrap, nil
}

// csrf rejects cross-origin state-changing requests.
func csrf(trustedOrigins []string) (Middleware, error) {
	cop := http.NewCrossOriginProtection()
	for _, o := range trustedOrigins {
		if err := cop.AddTrustedOrigin(o); err != nil {
			return nil, fmt.Errorf("csrf trusted origin %q: %w", o, err)
		}
	}
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		respondError(w, http.StatusForbidden, "cross-origin request rejected")
	}))
	return cop.Handler, nil
}

// bodyLimit caps the request body size for methods that carry one.
func bodyLimit(maxBytes int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch:
				http.MaxBytesHandler(next, maxBytes).ServeHTTP(w, r)
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

// compression applies zstd/gzip to JSON responses larger than minBytes
func compression(minBytes int) (Middleware, error) {
	wrap, err := gzhttp.NewWrapper(
		gzhttp.MinSize(minBytes),
		gzhttp.ContentTypes([]string{MediaTypeJSON}),
	)
	if err != nil {
		return nil, fmt.Errorf("compress: %w", err)
	}

	return func(next http.Handler) http.Handler {
		return wrap(next)
	}, nil
}

// Auth rejects requests without a valid bearer token and puts the caller id in the context.
func Auth(v verifier) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			token, ok := auth.BearerToken(r.Header.Get("Authorization"))
			if !ok {
				unauthorized(w)
				return
			}
			sub, err := v.Verify(token)
			if err != nil {
				slog.Default().Debug("token rejected", slog.Any("error", err))
				unauthorized(w)
				return
			}
			next(w, r.WithContext(auth.WithUserID(r.Context(), sub)))
		}
	}
}

// unauthorized replies with a bare 401 and the RFC 6750 challenge, the reason stays in the log.
func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	respondError(w, http.StatusUnauthorized, "invalid or missing token")
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the underlying writer's capabilities.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
