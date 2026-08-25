package grpc

import (
	"context"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"
	"uuid"

	catalogv1 "github.com/alkmc/storefront/api/gen/catalog/v1"
	"github.com/alkmc/storefront/internal/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// publicMethods lists the RPCs deliberately served without a caller identity.
var publicMethods = map[string]struct{}{
	catalogv1.ProductService_GetProduct_FullMethodName:    {},
	catalogv1.ProductService_ListProducts_FullMethodName:  {},
	catalogv1.ProductService_CreateProduct_FullMethodName: {},
	catalogv1.ProductService_UpdateProduct_FullMethodName: {},
	catalogv1.ProductService_DeleteProduct_FullMethodName: {},
}

// verifier checks a bearer token and returns the caller id.
type verifier interface {
	Verify(string) (uuid.UUID, error)
}

// requireAuth denies by default, only listed methods and grpc.* infrastructure skip the token check.
func requireAuth(v verifier, log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
	) (any, error) {
		// the grpc. prefix covers health and reflection, infrastructure stays token free
		if _, ok := publicMethods[info.FullMethod]; ok || strings.HasPrefix(info.FullMethod, "/grpc.") {
			return handler(ctx, req)
		}
		token, ok := bearerFromContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "invalid or missing token")
		}
		sub, err := v.Verify(token)
		if err != nil {
			log.Debug("token rejected", slog.Any("error", err))
			return nil, status.Error(codes.Unauthenticated, "invalid or missing token")
		}
		return handler(auth.WithUserID(ctx, sub), req)
	}
}

// logging logs each unary RPC with its method, resulting code, and duration.
func logging(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		log.Info(
			"grpc request",
			slog.String("method", info.FullMethod),
			slog.String("code", status.Code(err).String()),
			slog.Duration("duration", time.Since(start)),
		)
		return resp, err
	}
}

// timeout bounds each unary RPC to d so a missing client deadline can't pin a handler.
func timeout(d time.Duration) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
	) (any, error) {
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		return handler(ctx, req)
	}
}

// recovery turns a panic in a unary handler into a codes.Internal error.
func recovery(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
	) (resp any, err error) {
		defer func() {
			if p := recover(); p != nil {
				log.Error(
					"grpc panic recovered",
					slog.Any("panic", p),
					slog.String("method", info.FullMethod),
					slog.String("stack", string(debug.Stack())),
				)
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

// bearerFromContext reads the token from the authorization metadata.
func bearerFromContext(ctx context.Context) (string, bool) {
	vals := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(vals) == 0 {
		return "", false
	}
	return auth.BearerToken(vals[0])
}
