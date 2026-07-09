package http

import (
	"context"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

type stubProcessor struct {
	create   func(context.Context, domain.Product) (domain.Product, error)
	findByID func(context.Context, uuid.UUID) (domain.Product, error)
	findAll  func(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
	update   func(context.Context, domain.Product) (domain.Product, error)
	delete   func(context.Context, uuid.UUID) error
	purchase func(context.Context, uuid.UUID, int64) (domain.Product, error)
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

func (s *stubProcessor) Purchase(ctx context.Context, id uuid.UUID, qty int64) (domain.Product, error) {
	return s.purchase(ctx, id, qty)
}
