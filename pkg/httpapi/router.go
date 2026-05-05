package httpapi

import (
	"net/http"
)

// NewMux initializes and returns a new standard library based ServeMux wrapped in middlewares
func NewMux(h *Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /product", h.Add)
	mux.HandleFunc("GET /product", h.Get)
	mux.HandleFunc("GET /product/{id}", h.GetByID)
	mux.HandleFunc("PUT /product/{id}", h.Update)
	mux.HandleFunc("DELETE /product/{id}", h.Delete)

	return recoverPanic()(logging()(mux))
}
