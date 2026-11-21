package httpapi

import (
	"net/http"
	"net/http/pprof"
	"strings"
)

// NewMux registers routes, private ones wrapped in requireAuth.
func NewMux(h *Handler, requireAuth func(http.HandlerFunc) http.HandlerFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/products", h.Add)
	mux.HandleFunc("PUT /v1/products/{id}", h.Update)
	mux.HandleFunc("GET /v1/products", h.Get)
	mux.HandleFunc("GET /v1/products/{id}", h.GetByID)
	mux.HandleFunc("DELETE /v1/products/{id}", h.Delete)
	mux.HandleFunc("POST /v1/orders", requireAuth(h.CreateOrder))
	mux.HandleFunc("GET /v1/orders", requireAuth(h.ListOrders))
	mux.HandleFunc("GET /v1/orders/{id}", requireAuth(h.GetOrder))

	mux.HandleFunc("/v1/products", methodNotAllowed(http.MethodGet, http.MethodHead, http.MethodPost))
	mux.HandleFunc("/v1/products/{id}",
		methodNotAllowed(http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete))
	mux.HandleFunc("/v1/orders", methodNotAllowed(http.MethodGet, http.MethodHead, http.MethodPost))
	mux.HandleFunc("/v1/orders/{id}", methodNotAllowed(http.MethodGet, http.MethodHead))
	mux.HandleFunc("/", notFound)

	return mux
}

// notFound replaces ServeMux's plain-text 404.
func notFound(w http.ResponseWriter, _ *http.Request) {
	respondError(w, http.StatusNotFound, msgUnknownRoute)
}

// methodNotAllowed replaces ServeMux's plain-text 405.
func methodNotAllowed(allowed ...string) http.HandlerFunc {
	allow := strings.Join(allowed, ", ")
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", allow)
		respondError(w, http.StatusMethodNotAllowed, msgMethodNotAllowed)
	}
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
