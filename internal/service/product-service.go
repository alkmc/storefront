package service

import (
	"context"

	"github.com/alkmc/restClean/internal/entity"
	"github.com/alkmc/restClean/internal/repository"
	"github.com/google/uuid"
)

type productService struct {
	repo repository.Repository
}

// NewService returns new Product Service
func NewService(r repository.Repository) Service {
	return new(productService{repo: r})
}

func (s *productService) Create(ctx context.Context, p *entity.Product) (*entity.Product, error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return s.repo.Save(ctx, p)
}

func (s *productService) FindByID(ctx context.Context, id uuid.UUID) (*entity.Product, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *productService) FindAll(ctx context.Context) ([]entity.Product, error) {
	return s.repo.FindAll(ctx)
}

func (s *productService) Update(ctx context.Context, p *entity.Product) error {
	return s.repo.Update(ctx, p)
}

func (s *productService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}
