package service

import (
	"context"

	"github.com/alkmc/restClean/internal/entity"
	"github.com/google/uuid"
)

type repository interface {
	Save(context.Context, *entity.Product) (*entity.Product, error)
	FindByID(context.Context, uuid.UUID) (*entity.Product, error)
	FindAll(context.Context) ([]entity.Product, error)
	Update(context.Context, *entity.Product) error
	Delete(context.Context, uuid.UUID) error
}

type service struct {
	repo repository
}

// NewService initializes the business logic layer backed by the provided repository
func NewService(r repository) *service {
	return new(service{repo: r})
}

func (s *service) Create(ctx context.Context, p *entity.Product) (*entity.Product, error) {
	p.ID = uuid.New()
	return s.repo.Save(ctx, p)
}

func (s *service) FindByID(ctx context.Context, id uuid.UUID) (*entity.Product, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *service) FindAll(ctx context.Context) ([]entity.Product, error) {
	return s.repo.FindAll(ctx)
}

func (s *service) Update(ctx context.Context, p *entity.Product) error {
	return s.repo.Update(ctx, p)
}

func (s *service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}
