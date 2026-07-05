package grpc

import (
	"context"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

type mockProcessor struct {
	CreateFn   func(context.Context, domain.Product) (domain.Product, error)
	FindByIDFn func(context.Context, uuid.UUID) (domain.Product, error)
	FindAllFn  func(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
	UpdateFn   func(context.Context, domain.Product) error
	DeleteFn   func(context.Context, uuid.UUID) error
}

func (m mockProcessor) Create(ctx context.Context, p domain.Product) (domain.Product, error) {
	return m.CreateFn(ctx, p)
}

func (m mockProcessor) FindByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	return m.FindByIDFn(ctx, id)
}

func (m mockProcessor) FindAll(
	ctx context.Context, cursor uuid.NullUUID, limit int,
) (domain.ProductPage, error) {
	return m.FindAllFn(ctx, cursor, limit)
}

func (m mockProcessor) Update(ctx context.Context, p domain.Product) error {
	return m.UpdateFn(ctx, p)
}

func (m mockProcessor) Delete(ctx context.Context, id uuid.UUID) error {
	return m.DeleteFn(ctx, id)
}
