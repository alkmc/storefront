package httpapi

import (
	"log/slog"
	"net/http"
)

// NewMux initializes and returns a new ServeMux wrapped in middlewares
func NewMux(l *slog.Logger, h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /product", h.Add)
	mux.HandleFunc("GET /product", h.Get)
	mux.HandleFunc("GET /product/{id}", h.GetByID)
	mux.HandleFunc("PUT /product/{id}", h.Update)
	mux.HandleFunc("DELETE /product/{id}", h.Delete)
	return recoverPanic(l)(logging(l)(mux))
}

// NewInternalMux returns a minimal mux for liveness and readiness probes.
func NewInternalMux(hh *InternalHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", hh.Healthz)
	mux.HandleFunc("GET /readyz", hh.Readyz)
	return mux
}
