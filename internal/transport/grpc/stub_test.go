package grpc

import (
	"context"

	"github.com/alkmc/storefront/internal/domain"
)

type stubProcessor struct {
	CreateFn      func(context.Context, domain.Product) (domain.Product, error)
	FindByIDFn    func(context.Context, domain.ProductID) (domain.Product, error)
	FindAllFn     func(context.Context, domain.Cursor, int) (domain.ProductPage, error)
	UpdateFn      func(context.Context, domain.Product) (domain.Product, error)
	DeleteFn      func(context.Context, domain.ProductID) error
	CreateOrderFn func(
		context.Context, domain.UserID, domain.ProductID, int64, domain.IdempotencyKey,
	) (domain.Order, bool, error)

	FindOrderFn  func(context.Context, domain.UserID, domain.OrderID) (domain.Order, error)
	FindOrdersFn func(context.Context, domain.UserID, domain.Cursor, int) (domain.OrderPage, error)
}

func (s stubProcessor) Create(ctx context.Context, p domain.Product) (domain.Product, error) {
	return s.CreateFn(ctx, p)
}

func (s stubProcessor) FindByID(ctx context.Context, id domain.ProductID) (domain.Product, error) {
	return s.FindByIDFn(ctx, id)
}

func (s stubProcessor) FindAll(
	ctx context.Context, cursor domain.Cursor, limit int,
) (domain.ProductPage, error) {
	return s.FindAllFn(ctx, cursor, limit)
}

func (s stubProcessor) Update(ctx context.Context, p domain.Product) (domain.Product, error) {
	return s.UpdateFn(ctx, p)
}

func (s stubProcessor) Delete(ctx context.Context, id domain.ProductID) error {
	return s.DeleteFn(ctx, id)
}

func (s stubProcessor) CreateOrder(
	ctx context.Context, userID domain.UserID, id domain.ProductID, qty int64, idem domain.IdempotencyKey,
) (domain.Order, bool, error) {
	return s.CreateOrderFn(ctx, userID, id, qty, idem)
}

func (s stubProcessor) FindOrder(ctx context.Context, userID domain.UserID, orderID domain.OrderID,
) (domain.Order, error) {
	return s.FindOrderFn(ctx, userID, orderID)
}

func (s stubProcessor) FindOrders(ctx context.Context, userID domain.UserID, cursor domain.Cursor, limit int,
) (domain.OrderPage, error) {
	return s.FindOrdersFn(ctx, userID, cursor, limit)
}
