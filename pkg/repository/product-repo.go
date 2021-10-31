package repository

import (
	"github.com/alkmc/restClean/pkg/entity"

	"github.com/google/uuid"
)

// Repository is responsible for DB operation on Product entity
type Repository interface {
	Save(*entity.Product) (*entity.Product, error)
	FindByID(uuid.UUID) (*entity.Product, error)
	FindAll() ([]entity.Product, error)
	Update(*entity.Product) error
	Delete(uuid.UUID) error
	CloseDB()
}
