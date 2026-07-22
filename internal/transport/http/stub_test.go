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
	findByID    func(context.Context, uuid.UUID) (domain.Product, error)
	findAll     func(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
	update      func(context.Context, domain.Product) (domain.Product, error)
	delete      func(context.Context, uuid.UUID) error
	createOrder func(context.Context, domain.UserID, uuid.UUID, int64) (domain.Order, error)

	findOrder  func(context.Context, domain.UserID, domain.OrderID) (domain.Order, error)
	findOrders func(context.Context, domain.UserID, uuid.NullUUID, int) (domain.OrderPage, error)
}

func (s *stubProcessor) Create(ctx context.Context, p domain.Product) (domain.Product, error) {
	return s.create(ctx, p)
}

func (s *stubProcessor) FindByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
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

func (s *stubProcessor) Delete(ctx context.Context, id uuid.UUID) error {
	return s.delete(ctx, id)
}

func (s *stubProcessor) CreateOrder(ctx context.Context, userID domain.UserID, id uuid.UUID, qty int64,
) (domain.Order, error) {
	return s.createOrder(ctx, userID, id, qty)
}

func (s *stubProcessor) FindOrder(ctx context.Context, userID domain.UserID, orderID domain.OrderID,
) (domain.Order, error) {
	return s.findOrder(ctx, userID, orderID)
}

func (s *stubProcessor) FindOrders(ctx context.Context, userID domain.UserID, cursor uuid.NullUUID, limit int,
) (domain.OrderPage, error) {
	return s.findOrders(ctx, userID, cursor, limit)
}
