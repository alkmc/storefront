package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"time"

	"github.com/klauspost/compress/gzhttp"
)

type Middleware = func(http.Handler) http.Handler

// NewMiddleware builds the standard middleware chain
func NewMiddleware(l *slog.Logger, compressMinBytes int, maxBodyBytes int64) (Middleware, error) {
	compression, err := compress(compressMinBytes)
	if err != nil {
		return nil, err
	}
	return func(next http.Handler) http.Handler {
		return chain(next, recoverer(l), logging(l), bodyLimit(maxBodyBytes), compression)
	}, nil
}

// chain composes middlewares in top-down order
func chain(h http.Handler, mws ...Middleware) http.Handler {
	for _, mw := range slices.Backward(mws) {
		h = mw(h)
	}
	return h
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

func compress(minBytes int) (Middleware, error) {
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

// logging logs method, path, status, and request duration.
func logging(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := new(statusRecorder{ResponseWriter: w, status: http.StatusOK})
			defer func() {
				logger.Info("http request",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", rec.status),
					slog.Duration("duration", time.Since(start)),
				)
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// recoverer catches panics and prevents the server from crashing
func recoverer(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("panic recovered",
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
