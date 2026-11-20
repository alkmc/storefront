package http

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeClosesConnectionsAfterShutdownTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	hanging := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	})
	srv := NewInternalServer(hanging, ServerCfg{ShutdownTimeout: 100 * time.Millisecond},
		slog.New(slog.DiscardHandler))

	var lc net.ListenConfig
	lis, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ctx, lis) }()

	if err := get(t.Context(), "http://"+lis.Addr().String()); err == nil {
		t.Fatal("request succeeded, want its connection closed")
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve() = %v, want nil after a shutdown timeout", err)
	}
}

func get(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
