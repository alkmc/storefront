package grpc

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	catalogv1 "github.com/alkmc/storefront/api/gen/catalog/v1"
	orderv1 "github.com/alkmc/storefront/api/gen/order/v1"
	"github.com/alkmc/storefront/internal/auth"
	"github.com/alkmc/storefront/internal/auth/authtest"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

func TestRequireAuth(t *testing.T) {
	sub := uuid.Must(uuid.NewV7())
	valid := "Bearer " + authtest.Token(testJWTSecret, sub, time.Now().Add(time.Hour))

	tests := []struct {
		name     string
		method   string
		md       metadata.MD
		wantCode codes.Code
		wantUser bool
	}{
		{
			name:     "protected without metadata",
			method:   catalogv1.ProductService_PurchaseProduct_FullMethodName,
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "protected with garbage token",
			method:   orderv1.OrderService_GetOrder_FullMethodName,
			md:       metadata.Pairs("authorization", "Bearer garbage"),
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "protected with valid token",
			method:   orderv1.OrderService_ListOrders_FullMethodName,
			md:       metadata.Pairs("authorization", valid),
			wantCode: codes.OK,
			wantUser: true,
		},
		{
			name:     "public without token",
			method:   catalogv1.ProductService_GetProduct_FullMethodName,
			wantCode: codes.OK,
		},
		{
			name:     "unlisted method denied by default",
			method:   "/future.v1.FutureService/New",
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "infrastructure without token",
			method:   "/grpc.health.v1.Health/Check",
			wantCode: codes.OK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := requireAuth(auth.NewVerifier(testJWTSecret), slog.New(slog.DiscardHandler))
			ctx := t.Context()
			if tt.md != nil {
				ctx = metadata.NewIncomingContext(ctx, tt.md)
			}
			var (
				gotUser uuid.UUID
				gotOK   bool
			)
			_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: tt.method},
				func(ctx context.Context, _ any) (any, error) {
					gotUser, gotOK = auth.UserID(ctx)
					return "ok", nil
				})

			if status.Code(err) != tt.wantCode {
				t.Fatalf("got code %v, want %v", status.Code(err), tt.wantCode)
			}
			if tt.wantUser && (!gotOK || gotUser != sub) {
				t.Errorf("got user %v ok %v, want %v", gotUser, gotOK, sub)
			}
		})
	}
}
