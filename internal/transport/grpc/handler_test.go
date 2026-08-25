package grpc

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"
	"uuid"

	catalogv1 "github.com/alkmc/storefront/api/gen/catalog/v1"
	orderv1 "github.com/alkmc/storefront/api/gen/order/v1"
	"github.com/alkmc/storefront/internal/auth"
	"github.com/alkmc/storefront/internal/auth/authtest"
	"github.com/alkmc/storefront/internal/domain"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

const testJWTSecret = "test-secret"

func TestHandler_CreateProduct(t *testing.T) {
	client := newTestClient(t, stubProcessor{
		CreateFn: func(_ context.Context, p domain.Product) (domain.Product, error) {
			p.ID = domain.ProductID(uuid.NewV7())
			return p, nil
		},
	})

	resp, err := client.CreateProduct(t.Context(), catalogv1.CreateProductRequest_builder{
		Name: "Test", Price: testMoney(1000), Stock: 5,
	}.Build())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetProduct().GetId() == "" {
		t.Error("expected generated id")
	}
	if got := resp.GetProduct().GetName(); got != "Test" {
		t.Errorf("got name %q, want %q", got, "Test")
	}
	if got := resp.GetProduct().GetStock(); got != 5 {
		t.Errorf("got stock %d, want 5", got)
	}
}

func TestHandler_GetProduct(t *testing.T) {
	id := uuid.NewV7()
	client := newTestClient(t, stubProcessor{
		FindByIDFn: func(_ context.Context, id domain.ProductID) (domain.Product, error) {
			return domain.Product{ID: id, Name: "Test"}, nil
		},
	})

	resp, err := client.GetProduct(t.Context(), catalogv1.GetProductRequest_builder{
		Id: id.String(),
	}.Build())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resp.GetProduct().GetId(); got != id.String() {
		t.Errorf("got id %q, want %q", got, id.String())
	}
}

func TestHandler_ListProducts(t *testing.T) {
	p1 := domain.Product{ID: domain.ProductID(uuid.NewV7()), Name: "P1"}
	p2 := domain.Product{ID: domain.ProductID(uuid.NewV7()), Name: "P2"}
	client := newTestClient(t, stubProcessor{
		FindAllFn: func(_ context.Context, _ domain.Cursor, _ int) (domain.ProductPage, error) {
			return domain.ProductPage{Items: []domain.Product{p1, p2}, HasMore: true}, nil
		},
	})

	resp, err := client.ListProducts(t.Context(), catalogv1.ListProductsRequest_builder{
		Limit: 2,
	}.Build())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(resp.GetProducts()); got != 2 {
		t.Errorf("got %d products, want 2", got)
	}
	if got := resp.GetNextCursor(); got != p2.ID.String() {
		t.Errorf("got next cursor %q, want %q", got, p2.ID.String())
	}
}

func TestHandler_UpdateProduct(t *testing.T) {
	id := uuid.NewV7()
	client := newTestClient(t, stubProcessor{
		UpdateFn: func(_ context.Context, p domain.Product) (domain.Product, error) {
			p.Stock = 7 // stock comes from the store, not the request
			return p, nil
		},
	})

	resp, err := client.UpdateProduct(t.Context(), catalogv1.UpdateProductRequest_builder{
		Id: id.String(), Name: "Updated", Price: testMoney(2000),
	}.Build())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resp.GetProduct().GetName(); got != "Updated" {
		t.Errorf("got name %q, want %q", got, "Updated")
	}
	if got := resp.GetProduct().GetStock(); got != 7 {
		t.Errorf("got stock %d, want 7 (from store, not request)", got)
	}
}

func TestHandler_DeleteProduct(t *testing.T) {
	id := uuid.NewV7()
	client := newTestClient(t, stubProcessor{
		DeleteFn: func(context.Context, domain.ProductID) error { return nil },
	})

	if _, err := client.DeleteProduct(t.Context(), catalogv1.DeleteProductRequest_builder{
		Id: id.String(),
	}.Build()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandler_CreateOrder(t *testing.T) {
	id := uuid.NewV7()
	sub := uuid.NewV7()
	orderID := uuid.NewV7()
	client := newOrderClient(t, stubProcessor{
		CreateOrderFn: func(
			_ context.Context, userID domain.UserID, pid domain.ProductID, qty int64, _ domain.IdempotencyKey,
		) (domain.Order, bool, error) {
			if userID != domain.UserID(sub) {
				t.Errorf("got user %v, want %v", userID, sub)
			}
			o := domain.Order{ID: domain.OrderID(orderID), UserID: userID, ProductID: pid, Quantity: qty}
			return o, false, nil
		},
	})

	ctx := metadata.AppendToOutgoingContext(
		authCtx(t, sub), metaIdempotencyKey, uuid.NewV7().String(),
	)
	resp, err := client.CreateOrder(
		ctx, orderv1.CreateOrderRequest_builder{
			ProductId: id.String(), Quantity: 2,
		}.Build(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	o := resp.GetOrder()
	if got := o.GetId(); got != orderID.String() {
		t.Errorf("got order id %q, want %q", got, orderID.String())
	}
	if got := o.GetProductId(); got != id.String() {
		t.Errorf("got productId %q, want %q", got, id.String())
	}
	if got := o.GetQuantity(); got != 2 {
		t.Errorf("got quantity %d, want 2", got)
	}
}

func TestHandler_ErrorMapping(t *testing.T) {
	id := uuid.NewV7()

	tests := []struct {
		name string
		proc stubProcessor
		call func(context.Context, catalogv1.ProductServiceClient) error
		want codes.Code
	}{
		{
			name: "invalid uuid",
			proc: stubProcessor{},
			call: func(ctx context.Context, c catalogv1.ProductServiceClient) error {
				_, err := c.GetProduct(ctx, catalogv1.GetProductRequest_builder{Id: "not-a-uuid"}.Build())
				return err
			},
			want: codes.InvalidArgument,
		},
		{
			name: "validation - empty name",
			proc: stubProcessor{},
			call: func(ctx context.Context, c catalogv1.ProductServiceClient) error {
				_, err := c.CreateProduct(ctx, catalogv1.CreateProductRequest_builder{
					Name: "", Price: testMoney(1000),
				}.Build())
				return err
			},
			want: codes.InvalidArgument,
		},
		{
			name: "not found",
			proc: stubProcessor{
				FindByIDFn: func(context.Context, domain.ProductID) (domain.Product, error) {
					return domain.Product{}, domain.ErrNotFound
				},
			},
			call: func(ctx context.Context, c catalogv1.ProductServiceClient) error {
				_, err := c.GetProduct(ctx, catalogv1.GetProductRequest_builder{Id: id.String()}.Build())
				return err
			},
			want: codes.NotFound,
		},
		{
			name: "unavailable",
			proc: stubProcessor{
				CreateFn: func(context.Context, domain.Product) (domain.Product, error) {
					return domain.Product{}, domain.ErrUnavailable
				},
			},
			call: func(ctx context.Context, c catalogv1.ProductServiceClient) error {
				_, err := c.CreateProduct(ctx, catalogv1.CreateProductRequest_builder{
					Name: "Test", Price: testMoney(1000),
				}.Build())
				return err
			},
			want: codes.Unavailable,
		},
		{
			name: "internal",
			proc: stubProcessor{
				CreateFn: func(context.Context, domain.Product) (domain.Product, error) {
					return domain.Product{}, errors.New("boom")
				},
			},
			call: func(ctx context.Context, c catalogv1.ProductServiceClient) error {
				_, err := c.CreateProduct(ctx, catalogv1.CreateProductRequest_builder{
					Name: "Test", Price: testMoney(1000),
				}.Build())
				return err
			},
			want: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, tt.proc)
			// the token is inert on public methods and lets the protected ones reach the handler
			if got := status.Code(tt.call(authCtx(t, uuid.NewV7()), client)); got != tt.want {
				t.Errorf("got code %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandler_CreateOrder_ErrorMapping(t *testing.T) {
	id := uuid.NewV7()

	tests := []struct {
		name string
		proc stubProcessor
		req  *orderv1.CreateOrderRequest
		want codes.Code
	}{
		{
			name: "insufficient stock",
			proc: stubProcessor{
				CreateOrderFn: func(context.Context, domain.UserID, domain.ProductID, int64, domain.IdempotencyKey,
				) (domain.Order, bool, error) {
					return domain.Order{}, false, domain.ErrInsufficientStock
				},
			},
			req:  orderv1.CreateOrderRequest_builder{ProductId: id.String(), Quantity: 2}.Build(),
			want: codes.FailedPrecondition,
		},
		{
			name: "invalid quantity",
			proc: stubProcessor{},
			req:  orderv1.CreateOrderRequest_builder{ProductId: id.String(), Quantity: 0}.Build(),
			want: codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newOrderClient(t, tt.proc)
			ctx := metadata.AppendToOutgoingContext(
				authCtx(t, uuid.NewV7()), metaIdempotencyKey, uuid.NewV7().String(),
			)
			_, err := client.CreateOrder(ctx, tt.req)
			if got := status.Code(err); got != tt.want {
				t.Errorf("got code %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandler_CreateOrder_Idempotency(t *testing.T) {
	id := uuid.NewV7()
	sub := uuid.NewV7()
	orderID := uuid.NewV7()
	idemKey := uuid.NewV7().String()

	t.Run("replay sets the replayed metadata", func(t *testing.T) {
		client := newOrderClient(t, stubProcessor{
			CreateOrderFn: func(
				_ context.Context, _ domain.UserID, pid domain.ProductID, qty int64, idem domain.IdempotencyKey,
			) (domain.Order, bool, error) {
				if idem == "" {
					t.Error("idempotency key not read from metadata")
				}
				o := domain.Order{ID: domain.OrderID(orderID), ProductID: pid, Quantity: qty}
				return o, true, nil
			},
		})
		ctx := metadata.AppendToOutgoingContext(authCtx(t, sub), metaIdempotencyKey, idemKey)
		var header metadata.MD
		if _, err := client.CreateOrder(
			ctx, orderv1.CreateOrderRequest_builder{ProductId: id.String(), Quantity: 2}.Build(),
			grpc.Header(&header),
		); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := header.Get(metaIdempotencyReplayed); len(got) == 0 || got[0] != "true" {
			t.Errorf("got %s %v, want [true]", metaIdempotencyReplayed, got)
		}
	})

	t.Run("mismatch is InvalidArgument", func(t *testing.T) {
		client := newOrderClient(t, stubProcessor{
			CreateOrderFn: func(context.Context, domain.UserID, domain.ProductID, int64, domain.IdempotencyKey,
			) (domain.Order, bool, error) {
				return domain.Order{}, false, domain.ErrIdempotencyMismatch
			},
		})
		ctx := metadata.AppendToOutgoingContext(authCtx(t, sub), metaIdempotencyKey, idemKey)
		_, err := client.CreateOrder(
			ctx, orderv1.CreateOrderRequest_builder{ProductId: id.String(), Quantity: 2}.Build(),
		)
		if got := status.Code(err); got != codes.InvalidArgument {
			t.Errorf("got code %v, want %v", got, codes.InvalidArgument)
		}
	})

	t.Run("missing key is InvalidArgument", func(t *testing.T) {
		called := false
		client := newOrderClient(t, stubProcessor{
			CreateOrderFn: func(context.Context, domain.UserID, domain.ProductID, int64, domain.IdempotencyKey,
			) (domain.Order, bool, error) {
				called = true
				return domain.Order{}, false, nil
			},
		})
		_, err := client.CreateOrder(
			authCtx(t, sub), orderv1.CreateOrderRequest_builder{ProductId: id.String(), Quantity: 2}.Build(),
		)
		if got := status.Code(err); got != codes.InvalidArgument {
			t.Errorf("got code %v, want %v", got, codes.InvalidArgument)
		}
		if called {
			t.Error("processor called despite missing key")
		}
	})

	t.Run("over-long key is InvalidArgument", func(t *testing.T) {
		called := false
		client := newOrderClient(t, stubProcessor{
			CreateOrderFn: func(context.Context, domain.UserID, domain.ProductID, int64, domain.IdempotencyKey,
			) (domain.Order, bool, error) {
				called = true
				return domain.Order{}, false, nil
			},
		})
		key := strings.Repeat("k", domain.MaxIdempotencyKeyLen+1)
		ctx := metadata.AppendToOutgoingContext(authCtx(t, sub), metaIdempotencyKey, key)
		_, err := client.CreateOrder(
			ctx, orderv1.CreateOrderRequest_builder{ProductId: id.String(), Quantity: 2}.Build(),
		)
		if got := status.Code(err); got != codes.InvalidArgument {
			t.Errorf("got code %v, want %v", got, codes.InvalidArgument)
		}
		if called {
			t.Error("processor called despite over-long key")
		}
	})
}

func testMoney(amount int64) *catalogv1.Money {
	return catalogv1.Money_builder{MinorAmount: amount, Currency: string(domain.CurrencyPLN)}.Build()
}

// newTestClient wires the handler behind an in-memory gRPC server over bufconn.
func newTestClient(t *testing.T, p processor) catalogv1.ProductServiceClient {
	t.Helper()
	conn, _ := newTestConn(t, p)
	return catalogv1.NewProductServiceClient(conn)
}

func newOrderClient(t *testing.T, p processor) orderv1.OrderServiceClient {
	t.Helper()
	conn, _ := newTestConn(t, p)
	return orderv1.NewOrderServiceClient(conn)
}

// newTestConn serves the real newServer chain, auth included, over bufconn.
func newTestConn(t *testing.T, p processor) (*grpc.ClientConn, *grpc.Server) {
	t.Helper()
	const maxRequestBytes = 1 << 20 // 1 MiB

	s := NewServer(
		ServerCfg{RequestTimeout: 2 * time.Second, MaxRequestBytes: maxRequestBytes},
		p, auth.NewVerifier(testJWTSecret), slog.New(slog.DiscardHandler),
	)
	lis := bufconn.Listen(maxRequestBytes)
	srv := s.newServer()
	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, srv
}

// TestAuthDeniesByDefault proves every registered unary RPC outside the public list needs a token,
// so a future method cannot ship open by omission.
func TestAuthDeniesByDefault(t *testing.T) {
	conn, srv := newTestConn(t, stubProcessor{})

	var visited int
	for name, info := range srv.GetServiceInfo() {
		if strings.HasPrefix(name, "grpc.") {
			continue
		}
		for _, m := range info.Methods {
			full := "/" + name + "/" + m.Name
			if _, ok := publicMethods[full]; ok {
				continue
			}
			visited++
			err := conn.Invoke(t.Context(), full, &emptypb.Empty{}, &emptypb.Empty{})
			if status.Code(err) != codes.Unauthenticated {
				t.Errorf("%s: got %v, want Unauthenticated without a token", full, status.Code(err))
			}
		}
	}
	if visited == 0 {
		t.Fatal("no protected methods visited, the canary is vacuous")
	}
}

// authCtx carries a valid bearer token in the outgoing metadata.
func authCtx(t *testing.T, sub uuid.UUID) context.Context {
	t.Helper()
	token := authtest.Token(testJWTSecret, sub, time.Now().Add(time.Hour))
	return metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer "+token)
}

func TestHandler_GetOrder(t *testing.T) {
	sub := uuid.NewV7()
	orderID := uuid.NewV7()
	productID := uuid.NewV7()
	created := time.Now().UTC().Truncate(time.Second)

	client := newOrderClient(t, stubProcessor{
		FindOrderFn: func(_ context.Context, userID domain.UserID, id domain.OrderID) (domain.Order, error) {
			if userID != domain.UserID(sub) {
				t.Errorf("got user %v, want %v", userID, sub)
			}
			return domain.Order{
				ID: id, UserID: userID, ProductID: domain.ProductID(productID), Quantity: 2,
				UnitPrice: domain.Money{MinorAmount: 999, Currency: domain.CurrencyPLN},
				CreatedAt: created,
			}, nil
		},
	})

	resp, err := client.GetOrder(authCtx(t, sub), orderv1.GetOrderRequest_builder{
		Id: orderID.String(),
	}.Build())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	o := resp.GetOrder()
	if o.GetId() != orderID.String() || o.GetProductId() != productID.String() || o.GetQuantity() != 2 {
		t.Errorf("got %v, want id %v productId %v quantity 2", o, orderID, productID)
	}
	if got := o.GetUnitPrice().GetMinorAmount(); got != 999 {
		t.Errorf("got unit price %d, want 999", got)
	}
	if got := o.GetCreatedAt().AsTime(); !got.Equal(created) {
		t.Errorf("got createdAt %v, want %v", got, created)
	}
}

func TestHandler_GetOrder_NotFound(t *testing.T) {
	client := newOrderClient(t, stubProcessor{
		FindOrderFn: func(context.Context, domain.UserID, domain.OrderID) (domain.Order, error) {
			return domain.Order{}, domain.ErrNotFound
		},
	})

	_, err := client.GetOrder(authCtx(t, uuid.NewV7()), orderv1.GetOrderRequest_builder{
		Id: uuid.NewV7().String(),
	}.Build())
	if status.Code(err) != codes.NotFound {
		t.Fatalf("got code %v, want NotFound", status.Code(err))
	}
	if got, want := status.Convert(err).Message(), "order not found"; got != want {
		t.Errorf("got msg %q, want %q", got, want)
	}
}

func TestHandler_ListOrders(t *testing.T) {
	sub := uuid.NewV7()
	price := domain.Money{MinorAmount: 500, Currency: domain.CurrencyPLN}
	o1 := domain.Order{
		ID: domain.OrderID(uuid.NewV7()), UserID: domain.UserID(sub), Quantity: 1, UnitPrice: price,
	}
	o2 := domain.Order{
		ID: domain.OrderID(uuid.NewV7()), UserID: domain.UserID(sub), Quantity: 2, UnitPrice: price,
	}

	client := newOrderClient(t, stubProcessor{
		FindOrdersFn: func(_ context.Context, userID domain.UserID, _ domain.Cursor, _ int,
		) (domain.OrderPage, error) {
			if userID != domain.UserID(sub) {
				t.Errorf("got user %v, want %v", userID, sub)
			}
			return domain.OrderPage{Items: []domain.Order{o2, o1}, HasMore: true}, nil
		},
	})

	resp, err := client.ListOrders(authCtx(t, sub), orderv1.ListOrdersRequest_builder{
		Limit: 2,
	}.Build())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(resp.GetOrders()); got != 2 {
		t.Fatalf("got %d orders, want 2", got)
	}
	if got := resp.GetOrders()[0].GetId(); got != o2.ID.String() {
		t.Errorf("got first order %q, want the newest %q", got, o2.ID.String())
	}
	if got := resp.GetNextCursor(); got != o1.ID.String() {
		t.Errorf("got next cursor %q, want %q", got, o1.ID.String())
	}
}

func TestHandler_ProtectedMethodsRequireToken(t *testing.T) {
	conn, _ := newTestConn(t, stubProcessor{
		FindByIDFn: func(_ context.Context, id domain.ProductID) (domain.Product, error) {
			return domain.Product{ID: id, Name: "Public"}, nil
		},
	})
	products := catalogv1.NewProductServiceClient(conn)
	orders := orderv1.NewOrderServiceClient(conn)

	if _, err := orders.CreateOrder(t.Context(), orderv1.CreateOrderRequest_builder{
		ProductId: uuid.NewV7().String(), Quantity: 1,
	}.Build()); status.Code(err) != codes.Unauthenticated {
		t.Errorf("create order: got %v, want Unauthenticated", status.Code(err))
	}
	if _, err := orders.ListOrders(
		t.Context(), orderv1.ListOrdersRequest_builder{}.Build(),
	); status.Code(err) != codes.Unauthenticated {
		t.Errorf("list orders: got %v, want Unauthenticated", status.Code(err))
	}
	if _, err := orders.GetOrder(t.Context(), orderv1.GetOrderRequest_builder{
		Id: uuid.NewV7().String(),
	}.Build()); status.Code(err) != codes.Unauthenticated {
		t.Errorf("get order: got %v, want Unauthenticated", status.Code(err))
	}

	// the catalog stays public, no token needed
	if _, err := products.GetProduct(t.Context(), catalogv1.GetProductRequest_builder{
		Id: uuid.NewV7().String(),
	}.Build()); err != nil {
		t.Errorf("public get product: unexpected error %v", err)
	}
}
