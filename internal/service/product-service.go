package service

import (
	"context"

	"github.com/alkmc/restClean/internal/entity"
	"github.com/google/uuid"
)

type productRepository interface {
	Save(context.Context, *entity.Product) (*entity.Product, error)
	FindByID(context.Context, uuid.UUID) (*entity.Product, error)
	FindAll(context.Context) ([]entity.Product, error)
	Update(context.Context, *entity.Product) error
	Delete(context.Context, uuid.UUID) error
}

type productService struct {
	repo productRepository
}

// NewService returns new Product Service
func NewService(r productRepository) *productService {
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
