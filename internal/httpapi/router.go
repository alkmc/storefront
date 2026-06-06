package httpapi

import (
	"net/http"
	"net/http/pprof"
)

// NewMux initializes new ServeMux and registers routes.
func NewMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /product", h.Add)
	mux.HandleFunc("PUT /product/{id}", h.Update)
	mux.HandleFunc("GET /product", h.Get)
	mux.HandleFunc("GET /product/{id}", h.GetByID)
	mux.HandleFunc("DELETE /product/{id}", h.Delete)

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
