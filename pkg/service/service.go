package service

import (
	"github.com/alkmc/restClean/pkg/entity"

	"github.com/google/uuid"
)

// Service is responsible for interaction with Repository interface
type Service interface {
	Create(*entity.Product) (*entity.Product, error)
	FindByID(uuid.UUID) (*entity.Product, error)
	FindAll() ([]entity.Product, error)
	Update(*entity.Product) error
	Delete(uuid.UUID) error
}
