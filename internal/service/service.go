package service

import (
	"context"
	"fmt"

	"github.com/alkmc/restClean/internal/entity"
	"github.com/google/uuid"
)

type repository interface {
	Save(context.Context, *entity.Product) (*entity.Product, error)
	FindByID(context.Context, uuid.UUID) (*entity.Product, error)
	FindAll(ctx context.Context, limit, offset int) ([]entity.Product, error)
	Update(context.Context, *entity.Product) error
	Delete(context.Context, uuid.UUID) error
}

type Service struct {
	repo repository
}

// NewService initializes the business logic layer backed by the provided repository
func NewService(r repository) *Service {
	return new(Service{repo: r})
}

func (s *Service) Create(ctx context.Context, p *entity.Product) (*entity.Product, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate uuid: %w", err)
	}
	p.ID = id
	return s.repo.Save(ctx, p)
}

func (s *Service) FindByID(ctx context.Context, id uuid.UUID) (*entity.Product, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) FindAll(ctx context.Context, limit, offset int) ([]entity.Product, error) {
	return s.repo.FindAll(ctx, limit, offset)
}

func (s *Service) Update(ctx context.Context, p *entity.Product) error {
	return s.repo.Update(ctx, p)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}
