package httpapi

import (
	"context"
	"net/http"
	"time"
)

type (
	pinger interface {
		Ping(context.Context) error
	}
	InternalHandler struct {
		db    pinger
		cache pinger
	}
)

func NewInternalHandler(db pinger, cache pinger) *InternalHandler {
	return &InternalHandler{db: db, cache: cache}
}

func (h *InternalHandler) Healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (h *InternalHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		http.Error(w, "db unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := h.cache.Ping(ctx); err != nil {
		http.Error(w, "cache unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
