package grpc

import (
	"context"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

type stubProcessor struct {
	CreateFn   func(context.Context, domain.Product) (domain.Product, error)
	FindByIDFn func(context.Context, uuid.UUID) (domain.Product, error)
	FindAllFn  func(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
	UpdateFn   func(context.Context, domain.Product) (domain.Product, error)
	DeleteFn   func(context.Context, uuid.UUID) error
	PurchaseFn func(context.Context, uuid.UUID, int64) (domain.Product, error)
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

func (s stubProcessor) Purchase(ctx context.Context, id uuid.UUID, qty int64) (domain.Product, error) {
	return s.PurchaseFn(ctx, id, qty)
}
