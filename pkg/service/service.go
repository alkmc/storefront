package service

import (
	"context"

	"github.com/alkmc/restClean/pkg/entity"
	"github.com/google/uuid"
)

// Service is responsible for interaction with Repository interface
type Service interface {
	Create(context.Context, *entity.Product) (*entity.Product, error)
	FindByID(context.Context, uuid.UUID) (*entity.Product, error)
	FindAll(context.Context) ([]entity.Product, error)
	Update(context.Context, *entity.Product) error
	Delete(context.Context, uuid.UUID) error
}
