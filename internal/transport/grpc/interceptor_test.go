package grpc

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestLoggingRecordsMethodAndCode(t *testing.T) {
	const method = "/catalog.v1.ProductService/GetProduct"

	var buf bytes.Buffer
	interceptor := logging(slog.New(slog.NewJSONHandler(&buf, nil)))
	_, err := interceptor(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: method},
		func(context.Context, any) (any, error) {
			return nil, status.Error(codes.NotFound, "nope")
		})

	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
	for _, want := range []string{method, "NotFound"} {
		if got := buf.String(); !strings.Contains(got, want) {
			t.Errorf("log missing %q: %s", want, got)
		}
	}
}

func TestRecoveryTurnsPanicIntoInternal(t *testing.T) {
	interceptor := recovery(slog.New(slog.DiscardHandler))
	_, err := interceptor(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: "/test/Panic"},
		func(context.Context, any) (any, error) { panic("boom") })

	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %v, want Internal", status.Code(err))
	}
}

func TestTimeoutCancelsSlowHandler(t *testing.T) {
	interceptor := timeout(time.Millisecond)
	_, err := interceptor(t.Context(), nil, &grpc.UnaryServerInfo{},
		func(ctx context.Context, _ any) (any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
}
