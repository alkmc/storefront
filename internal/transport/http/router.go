package http

import (
	"net/http"
	"net/http/pprof"
)

// NewMux registers routes, private ones wrapped in requireAuth.
func NewMux(h *Handler, requireAuth func(http.HandlerFunc) http.HandlerFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/product", h.Add)
	mux.HandleFunc("PUT /v1/product/{id}", h.Update)
	mux.HandleFunc("GET /v1/product", h.Get)
	mux.HandleFunc("GET /v1/product/{id}", h.GetByID)
	mux.HandleFunc("DELETE /v1/product/{id}", h.Delete)
	mux.HandleFunc("POST /v1/product/{id}/purchase", requireAuth(h.Purchase))

	return mux
}

// NewInternalMux returns a mux for the internal-only port.
func NewInternalMux(hh *InternalHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", hh.Healthz)
	mux.HandleFunc("GET /readyz", hh.Readyz)
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return mux
}
