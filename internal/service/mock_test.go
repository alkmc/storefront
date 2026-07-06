package service

import (
	"context"

	"github.com/alkmc/storefront/internal/cache"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

type mockCache struct{}

func (mockCache) Set(_ context.Context, _ string, _ domain.Product) error {
	return nil
}

func (mockCache) Get(_ context.Context, _ string) (domain.Product, error) {
	return domain.Product{}, cache.ErrCacheMiss
}

func (mockCache) Invalidate(_ context.Context, _ string) error {
	return nil
}

type MockStore struct {
	SaveFn     func(context.Context, domain.Product) (domain.Product, error)
	FindByIDFn func(context.Context, uuid.UUID) (domain.Product, error)
	FindAllFn  func(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
	UpdateFn   func(context.Context, domain.Product) error
	DeleteFn   func(context.Context, uuid.UUID) error
}

func (m *MockStore) Save(ctx context.Context, p domain.Product) (domain.Product, error) {
	return m.SaveFn(ctx, p)
}

func (m *MockStore) FindByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	return m.FindByIDFn(ctx, id)
}

func (m *MockStore) FindAll(ctx context.Context, cursor uuid.NullUUID, limit int,
) (domain.ProductPage, error) {
	return m.FindAllFn(ctx, cursor, limit)
}

func (m *MockStore) Update(ctx context.Context, p domain.Product) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, p)
	}
	return nil
}

func (m *MockStore) Delete(ctx context.Context, id uuid.UUID) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return nil
}
