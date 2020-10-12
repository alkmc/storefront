package service

import (
	"errors"

	"github.com/alkmc/restClean/product/entity"
	"github.com/alkmc/restClean/product/repository"

	"github.com/google/uuid"
)

type productService struct {
	repo repository.Repository
}

//NewService returns new Product Service
func NewService(r repository.Repository) Service {
	return &productService{repo: r}
}

func (s *productService) Validate(p *entity.Product) error {
	if p == nil {
		err := errors.New("the product is empty")
		return err
	}
	if p.Name == "" {
		err := errors.New("the product name is empty")
		return err
	}
	if p.Price <= 0 {
		err := errors.New("the product price must be positive")
		return err
	}
	return nil
}

func (s *productService) Create(p *entity.Product) (*entity.Product, error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return s.repo.Save(p)
}

func (s *productService) FindByID(id uuid.UUID) (*entity.Product, error) {
	return s.repo.FindByID(id)
}

func (s *productService) FindAll() ([]entity.Product, error) {
	return s.repo.FindAll()
}

func (s *productService) Update(p *entity.Product) error {
	return s.repo.Update(p)
}

func (s *productService) Delete(id uuid.UUID) error {
	return s.repo.Delete(id)
}
