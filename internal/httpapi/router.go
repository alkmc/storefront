package httpapi

import (
	"net/http"
)

// NewMux initializes new ServeMux and registers routes.
func NewMux(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /product", h.Add)
	mux.HandleFunc("PUT /product/{id}", h.Update)
	mux.HandleFunc("GET /product", h.Get)
	mux.HandleFunc("GET /product/{id}", h.GetByID)
	mux.HandleFunc("DELETE /product/{id}", h.Delete)

	return mux
}

// NewInternalMux returns a minimal mux for liveness and readiness probes.
func NewInternalMux(hh *InternalHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", hh.Healthz)
	mux.HandleFunc("GET /readyz", hh.Readyz)
	return mux
}
