package http

import (
	"net/http"
	"net/http/pprof"
)

// NewMux registers routes, private ones wrapped in requireAuth.
func NewMux(h *Handler, requireAuth func(http.HandlerFunc) http.HandlerFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/products", h.Add)
	mux.HandleFunc("PUT /v1/products/{id}", h.Update)
	mux.HandleFunc("GET /v1/products", h.Get)
	mux.HandleFunc("GET /v1/products/{id}", h.GetByID)
	mux.HandleFunc("DELETE /v1/products/{id}", h.Delete)
	mux.HandleFunc("POST /v1/products/{id}/purchase", requireAuth(h.Purchase))
	mux.HandleFunc("GET /v1/orders", requireAuth(h.ListOrders))
	mux.HandleFunc("GET /v1/orders/{id}", requireAuth(h.GetOrder))

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
