package service

import (
	"context"

	"github.com/alkmc/storefront/internal/cache"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

type stubCache struct{}

func (stubCache) Set(_ context.Context, _ string, _ domain.Product, _ cache.Entry) error {
	return nil
}

func (stubCache) SetMissing(_ context.Context, _ string, _ cache.Entry) error {
	return nil
}

func (stubCache) Get(_ context.Context, _ string) (cache.Entry, error) {
	return cache.Entry{}, nil
}

func (stubCache) Invalidate(_ context.Context, _ string) error {
	return nil
}

type SpyCache struct {
	GetFn            func(context.Context, string) (cache.Entry, error)
	Sets             int
	SetMissings      int
	InvalidateCtxErr error
	Invalidates      int
	InvalidatedKeys  []string
}

func (c *SpyCache) Set(_ context.Context, _ string, _ domain.Product, _ cache.Entry) error {
	c.Sets++
	return nil
}

func (c *SpyCache) SetMissing(_ context.Context, _ string, _ cache.Entry) error {
	c.SetMissings++
	return nil
}

func (c *SpyCache) Get(ctx context.Context, key string) (cache.Entry, error) {
	if c.GetFn != nil {
		return c.GetFn(ctx, key)
	}
	return cache.Entry{}, nil
}

func (c *SpyCache) Invalidate(ctx context.Context, key string) error {
	c.Invalidates++
	c.InvalidateCtxErr = ctx.Err()
	c.InvalidatedKeys = append(c.InvalidatedKeys, key)
	return nil
}

type SpyStore struct {
	SaveFn     func(context.Context, domain.Product) (domain.Product, error)
	FindByIDFn func(context.Context, uuid.UUID) (domain.Product, error)
	FindAllFn  func(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
	UpdateFn   func(context.Context, domain.Product) (domain.Product, error)
	DeleteFn   func(context.Context, uuid.UUID) error
	PurchaseFn func(context.Context, domain.Order) (domain.Product, domain.Order, error)

	FindOrderFn  func(context.Context, domain.UserID, domain.OrderID) (domain.Order, error)
	FindOrdersFn func(context.Context, domain.UserID, uuid.NullUUID, int) (domain.OrderPage, error)
}

func (s *SpyStore) Save(ctx context.Context, p domain.Product) (domain.Product, error) {
	return s.SaveFn(ctx, p)
}

func (s *SpyStore) FindByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	return s.FindByIDFn(ctx, id)
}

func (s *SpyStore) FindAll(ctx context.Context, cursor uuid.NullUUID, limit int,
) (domain.ProductPage, error) {
	return s.FindAllFn(ctx, cursor, limit)
}

func (s *SpyStore) Update(ctx context.Context, p domain.Product) (domain.Product, error) {
	if s.UpdateFn != nil {
		return s.UpdateFn(ctx, p)
	}
	return domain.Product{}, nil
}

func (s *SpyStore) Delete(ctx context.Context, id uuid.UUID) error {
	if s.DeleteFn != nil {
		return s.DeleteFn(ctx, id)
	}
	return nil
}

func (s *SpyStore) Purchase(ctx context.Context, o domain.Order) (domain.Product, domain.Order, error) {
	if s.PurchaseFn != nil {
		return s.PurchaseFn(ctx, o)
	}
	return domain.Product{}, domain.Order{}, nil
}

func (s *SpyStore) FindOrder(
	ctx context.Context, userID domain.UserID, orderID domain.OrderID,
) (domain.Order, error) {
	if s.FindOrderFn != nil {
		return s.FindOrderFn(ctx, userID, orderID)
	}
	return domain.Order{}, nil
}

func (s *SpyStore) FindOrders(ctx context.Context, userID domain.UserID, cursor uuid.NullUUID, limit int,
) (domain.OrderPage, error) {
	if s.FindOrdersFn != nil {
		return s.FindOrdersFn(ctx, userID, cursor, limit)
	}
	return domain.OrderPage{}, nil
}
