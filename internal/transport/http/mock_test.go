package http

import (
	"context"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

type mockProcessor struct {
	create   func(context.Context, domain.Product) (domain.Product, error)
	findByID func(context.Context, uuid.UUID) (domain.Product, error)
	findAll  func(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
	update   func(context.Context, domain.Product) error
	delete   func(context.Context, uuid.UUID) error
}

func (m *mockProcessor) Create(ctx context.Context, p domain.Product) (domain.Product, error) {
	return m.create(ctx, p)
}

func (m *mockProcessor) FindByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	if m.findByID == nil {
		return domain.Product{}, domain.ErrNotFound
	}
	return m.findByID(ctx, id)
}

func (m *mockProcessor) FindAll(ctx context.Context, cursor uuid.NullUUID, limit int,
) (domain.ProductPage, error) {
	return m.findAll(ctx, cursor, limit)
}

func (m *mockProcessor) Update(ctx context.Context, p domain.Product) error {
	return m.update(ctx, p)
}

func (m *mockProcessor) Delete(ctx context.Context, id uuid.UUID) error {
	return m.delete(ctx, id)
}
