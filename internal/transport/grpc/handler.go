package grpc

import (
	"context"
	"errors"
	"log/slog"
	"uuid"

	catalogv1 "github.com/alkmc/storefront/api/gen/catalog/v1"
	orderv1 "github.com/alkmc/storefront/api/gen/order/v1"
	"github.com/alkmc/storefront/internal/auth"
	"github.com/alkmc/storefront/internal/domain"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type (
	// processor is the product business logic the gRPC handler depends on.
	processor interface {
		Create(context.Context, domain.Product) (domain.Product, error)
		FindByID(context.Context, domain.ProductID) (domain.Product, error)
		FindAll(context.Context, domain.Cursor, int) (domain.ProductPage, error)
		Update(context.Context, domain.Product) (domain.Product, error)
		Delete(context.Context, domain.ProductID) error
		CreateOrder(
			context.Context, domain.UserID, domain.ProductID, int64, domain.IdempotencyKey,
		) (domain.Order, bool, error)
		FindOrder(context.Context, domain.UserID, domain.OrderID) (domain.Order, error)
		FindOrders(context.Context, domain.UserID, domain.Cursor, int) (domain.OrderPage, error)
	}
	// Handler adapts the product service to the generated ProductServiceServer and OrderServiceServer.
	Handler struct {
		catalogv1.UnimplementedProductServiceServer
		orderv1.UnimplementedOrderServiceServer
		logger    *slog.Logger
		processor processor
	}
)

// NewHandler initializes a gRPC product handler backed by the given processor.
func NewHandler(p processor, l *slog.Logger) *Handler {
	return new(Handler{logger: l, processor: p})
}

func (h *Handler) CreateProduct(
	ctx context.Context, req *catalogv1.CreateProductRequest,
) (*catalogv1.CreateProductResponse, error) {
	p := domain.Product{Name: req.GetName(), Price: toDomainMoney(req.GetPrice()), Stock: req.GetStock()}
	if err := p.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	created, err := h.processor.Create(ctx, p)
	if err != nil {
		return nil, h.toStatus(err, "create product")
	}
	return catalogv1.CreateProductResponse_builder{Product: toProductProto(created)}.Build(), nil
}

func (h *Handler) GetProduct(
	ctx context.Context, req *catalogv1.GetProductRequest,
) (*catalogv1.GetProductResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	p, err := h.processor.FindByID(ctx, domain.ProductID(id))
	if err != nil {
		return nil, h.toStatus(err, "get product")
	}
	return catalogv1.GetProductResponse_builder{Product: toProductProto(p)}.Build(), nil
}

func (h *Handler) ListProducts(
	ctx context.Context, req *catalogv1.ListProductsRequest,
) (*catalogv1.ListProductsResponse, error) {
	cursor, err := domain.ParseCursor(req.GetCursor())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	page, err := h.processor.FindAll(ctx, cursor, domain.NormalizePageSize(int(req.GetLimit())))
	if err != nil {
		return nil, h.toStatus(err, "list products")
	}
	return catalogv1.ListProductsResponse_builder{
		Products:   mapSlice(page.Items, toProductProto),
		NextCursor: page.NextCursor(),
	}.Build(), nil
}

func (h *Handler) UpdateProduct(
	ctx context.Context, req *catalogv1.UpdateProductRequest,
) (*catalogv1.UpdateProductResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	p := domain.Product{ID: domain.ProductID(id), Name: req.GetName(), Price: toDomainMoney(req.GetPrice())}
	if err := p.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	updated, err := h.processor.Update(ctx, p)
	if err != nil {
		return nil, h.toStatus(err, "update product")
	}
	return catalogv1.UpdateProductResponse_builder{Product: toProductProto(updated)}.Build(), nil
}

func (h *Handler) DeleteProduct(
	ctx context.Context, req *catalogv1.DeleteProductRequest,
) (*catalogv1.DeleteProductResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := h.processor.Delete(ctx, domain.ProductID(id)); err != nil {
		return nil, h.toStatus(err, "delete product")
	}
	return catalogv1.DeleteProductResponse_builder{}.Build(), nil
}

const (
	metaIdempotencyKey      = "idempotency-key"
	metaIdempotencyReplayed = "idempotency-replayed"
)

func (h *Handler) CreateOrder(
	ctx context.Context, req *orderv1.CreateOrderRequest,
) (*orderv1.CreateOrderResponse, error) {
	userID, err := h.callerID(ctx)
	if err != nil {
		return nil, err
	}
	productID, err := uuid.Parse(req.GetProductId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	idem, err := idempotencyFromMetadata(ctx)
	if err != nil {
		return nil, err
	}
	qty := req.GetQuantity()
	if !domain.ValidPurchaseQuantity(qty) {
		return nil, status.Error(codes.InvalidArgument, "quantity must be between 1 and 10000")
	}

	order, replayed, err := h.processor.CreateOrder(ctx, userID, domain.ProductID(productID), qty, idem)
	if err != nil {
		return nil, h.toStatus(err, "create order")
	}
	if replayed {
		if err := grpc.SetHeader(ctx, metadata.Pairs(metaIdempotencyReplayed, "true")); err != nil {
			h.logger.Warn("set idempotency-replayed header failed", slog.Any("error", err))
		}
	}
	return orderv1.CreateOrderResponse_builder{
		Order: toOrderProto(order),
	}.Build(), nil
}

// idempotencyFromMetadata reads the required idempotency-key metadata: an opaque string of at most
// domain.MaxIdempotencyKeyLen bytes. A missing, empty, or over-long key is a client error.
func idempotencyFromMetadata(ctx context.Context) (domain.IdempotencyKey, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Errorf(
			codes.InvalidArgument, "%s metadata is required", metaIdempotencyKey,
		)
	}
	vals := md.Get(metaIdempotencyKey)
	if len(vals) == 0 || vals[0] == "" {
		return "", status.Errorf(
			codes.InvalidArgument, "%s metadata is required", metaIdempotencyKey,
		)
	}
	key := vals[0]
	if len(key) > domain.MaxIdempotencyKeyLen {
		return "", status.Errorf(
			codes.InvalidArgument,
			"%s must be at most %d bytes", metaIdempotencyKey, domain.MaxIdempotencyKeyLen,
		)
	}
	return domain.IdempotencyKey(key), nil
}

// toStatus maps domain errors to gRPC codes; unknown errors become Internal.
func (h *Handler) toStatus(err error, op string) error {
	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "deadline exceeded")
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, "product not found")
	case errors.Is(err, domain.ErrInsufficientStock):
		return status.Error(codes.FailedPrecondition, "insufficient stock")
	case errors.Is(err, domain.ErrIdempotencyMismatch):
		return status.Error(codes.InvalidArgument, "idempotency key reused with different payload")
	case errors.Is(err, domain.ErrProductInUse):
		return status.Error(codes.FailedPrecondition, "product has existing orders")
	case errors.Is(err, domain.ErrUnavailable):
		return status.Error(codes.Unavailable, "service temporarily unavailable")
	default:
		h.logger.Error(op+" failed", slog.Any("error", err))
		return status.Error(codes.Internal, "internal error")
	}
}

// callerID reads the authenticated user, its absence on a protected method is an interceptor wiring bug.
func (h *Handler) callerID(ctx context.Context) (domain.UserID, error) {
	userID, ok := auth.UserID(ctx)
	if !ok {
		h.logger.Error("user id missing on protected method")
		return domain.UserID{}, status.Error(codes.Internal, "internal error")
	}
	return domain.UserID(userID), nil
}

func (h *Handler) GetOrder(
	ctx context.Context, req *orderv1.GetOrderRequest,
) (*orderv1.GetOrderResponse, error) {
	userID, err := h.callerID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	o, err := h.processor.FindOrder(ctx, userID, domain.OrderID(id))
	if err != nil {
		// the shared toStatus says "product not found", an order needs its own message
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, h.toStatus(err, "get order")
	}
	return orderv1.GetOrderResponse_builder{Order: toOrderProto(o)}.Build(), nil
}

func (h *Handler) ListOrders(
	ctx context.Context, req *orderv1.ListOrdersRequest,
) (*orderv1.ListOrdersResponse, error) {
	userID, err := h.callerID(ctx)
	if err != nil {
		return nil, err
	}
	cursor, err := domain.ParseCursor(req.GetCursor())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	page, err := h.processor.FindOrders(ctx, userID, cursor, domain.NormalizePageSize(int(req.GetLimit())))
	if err != nil {
		return nil, h.toStatus(err, "list orders")
	}
	return orderv1.ListOrdersResponse_builder{
		Orders:     mapSlice(page.Items, toOrderProto),
		NextCursor: page.NextCursor(),
	}.Build(), nil
}
