package repository

import (
	"github.com/alkmc/restClean/pkg/entity"

	"github.com/google/uuid"
)

//Repository is responsible for DB operation on Product entity
type Repository interface {
	Save(p *entity.Product) (*entity.Product, error)
	FindByID(id uuid.UUID) (*entity.Product, error)
	FindAll() ([]entity.Product, error)
	Update(p *entity.Product) error
	Delete(id uuid.UUID) error
	CloseDB()
}
