package grpc

import (
	"context"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

type stubProcessor struct {
	CreateFn      func(context.Context, domain.Product) (domain.Product, error)
	FindByIDFn    func(context.Context, uuid.UUID) (domain.Product, error)
	FindAllFn     func(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
	UpdateFn      func(context.Context, domain.Product) (domain.Product, error)
	DeleteFn      func(context.Context, uuid.UUID) error
	CreateOrderFn func(context.Context, domain.UserID, uuid.UUID, int64) (domain.Order, error)

	FindOrderFn  func(context.Context, domain.UserID, domain.OrderID) (domain.Order, error)
	FindOrdersFn func(context.Context, domain.UserID, uuid.NullUUID, int) (domain.OrderPage, error)
}

func (s stubProcessor) Create(ctx context.Context, p domain.Product) (domain.Product, error) {
	return s.CreateFn(ctx, p)
}

func (s stubProcessor) FindByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	return s.FindByIDFn(ctx, id)
}

func (s stubProcessor) FindAll(
	ctx context.Context, cursor uuid.NullUUID, limit int,
) (domain.ProductPage, error) {
	return s.FindAllFn(ctx, cursor, limit)
}

func (s stubProcessor) Update(ctx context.Context, p domain.Product) (domain.Product, error) {
	return s.UpdateFn(ctx, p)
}

func (s stubProcessor) Delete(ctx context.Context, id uuid.UUID) error {
	return s.DeleteFn(ctx, id)
}

func (s stubProcessor) CreateOrder(ctx context.Context, userID domain.UserID, id uuid.UUID, qty int64,
) (domain.Order, error) {
	return s.CreateOrderFn(ctx, userID, id, qty)
}

func (s stubProcessor) FindOrder(ctx context.Context, userID domain.UserID, orderID domain.OrderID,
) (domain.Order, error) {
	return s.FindOrderFn(ctx, userID, orderID)
}

func (s stubProcessor) FindOrders(ctx context.Context, userID domain.UserID, cursor uuid.NullUUID, limit int,
) (domain.OrderPage, error) {
	return s.FindOrdersFn(ctx, userID, cursor, limit)
}
