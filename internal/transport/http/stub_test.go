package http

import (
	"context"

	"github.com/alkmc/storefront/internal/cache"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

type nopCache struct{}

func (nopCache) Set(context.Context, string, domain.Product, cache.Entry) error { return nil }

func (nopCache) SetMissing(context.Context, string, cache.Entry) error { return nil }

func (nopCache) Get(context.Context, string) (cache.Entry, error) { return cache.Entry{}, nil }

func (nopCache) Invalidate(context.Context, string) error { return nil }

type stubProcessor struct {
	create      func(context.Context, domain.Product) (domain.Product, error)
	findByID    func(context.Context, domain.ProductID) (domain.Product, error)
	findAll     func(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
	update      func(context.Context, domain.Product) (domain.Product, error)
	delete      func(context.Context, domain.ProductID) error
	createOrder func(context.Context, domain.UserID, domain.ProductID, int64, domain.IdempotencyKey,
	) (domain.Order, bool, error)

	findOrder  func(context.Context, domain.UserID, domain.OrderID) (domain.Order, error)
	findOrders func(context.Context, domain.UserID, uuid.NullUUID, int) (domain.OrderPage, error)
}

func (s *stubProcessor) Create(ctx context.Context, p domain.Product) (domain.Product, error) {
	return s.create(ctx, p)
}

func (s *stubProcessor) FindByID(ctx context.Context, id domain.ProductID) (domain.Product, error) {
	if s.findByID == nil {
		return domain.Product{}, domain.ErrNotFound
	}
	return s.findByID(ctx, id)
}

func (s *stubProcessor) FindAll(ctx context.Context, cursor uuid.NullUUID, limit int,
) (domain.ProductPage, error) {
	return s.findAll(ctx, cursor, limit)
}

func (s *stubProcessor) Update(ctx context.Context, p domain.Product) (domain.Product, error) {
	return s.update(ctx, p)
}

func (s *stubProcessor) Delete(ctx context.Context, id domain.ProductID) error {
	return s.delete(ctx, id)
}

func (s *stubProcessor) CreateOrder(
	ctx context.Context, userID domain.UserID, id domain.ProductID, qty int64, idem domain.IdempotencyKey,
) (domain.Order, bool, error) {
	return s.createOrder(ctx, userID, id, qty, idem)
}

func (s *stubProcessor) FindOrder(ctx context.Context, userID domain.UserID, orderID domain.OrderID,
) (domain.Order, error) {
	return s.findOrder(ctx, userID, orderID)
}

func (s *stubProcessor) FindOrders(ctx context.Context, userID domain.UserID, cursor uuid.NullUUID, limit int,
) (domain.OrderPage, error) {
	return s.findOrders(ctx, userID, cursor, limit)
}
