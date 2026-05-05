package repository

import (
	"context"

	"github.com/alkmc/restClean/pkg/entity"
	"github.com/google/uuid"
)

// Repository is responsible for DB operation on Product entity
type Repository interface {
	Save(context.Context, *entity.Product) (*entity.Product, error)
	FindByID(context.Context, uuid.UUID) (*entity.Product, error)
	FindAll(context.Context) ([]entity.Product, error)
	Update(context.Context, *entity.Product) error
	Delete(context.Context, uuid.UUID) error
	CloseDB()
}
