package service

import (
	"github.com/alkmc/restClean/product/entity"

	"github.com/google/uuid"
)

//Service is responsible for interaction with Repository interface
type Service interface {
	Create(p *entity.Product) (*entity.Product, error)
	FindByID(id uuid.UUID) (*entity.Product, error)
	FindAll() ([]entity.Product, error)
	Update(p *entity.Product) error
	Delete(id uuid.UUID) error
}
