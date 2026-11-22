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
		db pinger
	}
)

func NewInternalHandler(db pinger) *InternalHandler {
	return new(InternalHandler{db: db})
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
	w.WriteHeader(http.StatusNoContent)
}
