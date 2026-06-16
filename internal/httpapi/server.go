package httpapi

import (
	"net/http"

	"github.com/alkmc/storefront/internal/config"
)

// NewAPIServer builds the public HTTP server with timeouts from cfg.
func NewAPIServer(cfg config.HTTP, h http.Handler) *http.Server {
	return &http.Server{
		Addr:         cfg.Address(),
		Handler:      h,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}
}

// NewInternalServer builds the internal probes server.
func NewInternalServer(cfg config.HTTP, h http.Handler) *http.Server {
	return &http.Server{
		Addr:        cfg.InternalAddress(),
		Handler:     h,
		ReadTimeout: cfg.ReadTimeout,
	}
}
