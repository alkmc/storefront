package grpc

import (
	"context"
	"errors"
	"log/slog"

	catalogv1 "github.com/alkmc/storefront/api/gen/catalog/v1"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type (
	// processor is the product business logic the gRPC handler depends on.
	processor interface {
		Create(context.Context, domain.Product) (domain.Product, error)
		FindByID(context.Context, uuid.UUID) (domain.Product, error)
		FindAll(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
		Update(context.Context, domain.Product) (domain.Product, error)
		Delete(context.Context, uuid.UUID) error
		Purchase(context.Context, uuid.UUID, int64) (domain.Product, error)
	}
	// Handler adapts the product service to the generated ProductServiceServer.
	Handler struct {
		catalogv1.UnimplementedProductServiceServer
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
	return catalogv1.CreateProductResponse_builder{Product: toProto(created)}.Build(), nil
}

func (h *Handler) GetProduct(
	ctx context.Context, req *catalogv1.GetProductRequest,
) (*catalogv1.GetProductResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	p, err := h.processor.FindByID(ctx, id)
	if err != nil {
		return nil, h.toStatus(err, "get product")
	}
	return catalogv1.GetProductResponse_builder{Product: toProto(p)}.Build(), nil
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
		Products:   toProtos(page.Items),
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
	p := domain.Product{ID: id, Name: req.GetName(), Price: toDomainMoney(req.GetPrice())}
	if err := p.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	updated, err := h.processor.Update(ctx, p)
	if err != nil {
		return nil, h.toStatus(err, "update product")
	}
	return catalogv1.UpdateProductResponse_builder{Product: toProto(updated)}.Build(), nil
}

func (h *Handler) DeleteProduct(
	ctx context.Context, req *catalogv1.DeleteProductRequest,
) (*catalogv1.DeleteProductResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := h.processor.Delete(ctx, id); err != nil {
		return nil, h.toStatus(err, "delete product")
	}
	return catalogv1.DeleteProductResponse_builder{}.Build(), nil
}

func (h *Handler) PurchaseProduct(
	ctx context.Context, req *catalogv1.PurchaseProductRequest,
) (*catalogv1.PurchaseProductResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	qty := req.GetQuantity()
	if !domain.ValidPurchaseQuantity(qty) {
		return nil, status.Error(codes.InvalidArgument, "quantity must be between 1 and 10000")
	}
	p, err := h.processor.Purchase(ctx, id, qty)
	if err != nil {
		return nil, h.toStatus(err, "purchase product")
	}
	return catalogv1.PurchaseProductResponse_builder{
		ProductId:      p.ID.String(),
		Quantity:       qty,
		RemainingStock: p.Stock,
	}.Build(), nil
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
	case errors.Is(err, domain.ErrUnavailable):
		return status.Error(codes.Unavailable, "service temporarily unavailable")
	default:
		h.logger.Error(op+" failed", slog.Any("error", err))
		return status.Error(codes.Internal, "internal error")
	}
}
