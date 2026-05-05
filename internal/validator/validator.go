package validator

import (
	"github.com/alkmc/restClean/internal/entity"
	"github.com/google/uuid"
)

// Validator is responsible for validating Product entity
type Validator interface {
	Product(*entity.Product) error
	Body(error) error
	UUID(string) (uuid.UUID, error)
}
