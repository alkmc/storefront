package service

import (
	"context"

	"github.com/alkmc/storefront/internal/cache"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

type stubCache struct{}

func (stubCache) Set(_ context.Context, _ string, _ domain.Product) error {
	return nil
}

func (stubCache) Get(_ context.Context, _ string) (domain.Product, error) {
	return domain.Product{}, cache.ErrCacheMiss
}

func (stubCache) Invalidate(_ context.Context, _ string) error {
	return nil
}

type SpyStore struct {
	SaveFn     func(context.Context, domain.Product) (domain.Product, error)
	FindByIDFn func(context.Context, uuid.UUID) (domain.Product, error)
	FindAllFn  func(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
	UpdateFn   func(context.Context, domain.Product) error
	DeleteFn   func(context.Context, uuid.UUID) error
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

func (s *SpyStore) Update(ctx context.Context, p domain.Product) error {
	if s.UpdateFn != nil {
		return s.UpdateFn(ctx, p)
	}
	return nil
}

func (s *SpyStore) Delete(ctx context.Context, id uuid.UUID) error {
	if s.DeleteFn != nil {
		return s.DeleteFn(ctx, id)
	}
	return nil
}
