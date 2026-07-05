package http

import (
	"net/http"
	"time"
)

// ServerCfg carries the settings the HTTP servers need.
type ServerCfg struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// NewAPIServer builds the public HTTP server with timeouts from cfg.
func NewAPIServer(h http.Handler, cfg ServerCfg) *http.Server {
	return &http.Server{
		Addr:         cfg.Addr,
		Handler:      h,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}
}

// NewInternalServer builds the internal probes server.
func NewInternalServer(h http.Handler, cfg ServerCfg) *http.Server {
	return &http.Server{
		Addr:        cfg.Addr,
		Handler:     h,
		ReadTimeout: cfg.ReadTimeout,
	}
}
