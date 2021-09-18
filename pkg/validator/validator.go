package validator

import (
	"github.com/alkmc/restClean/pkg/entity"

	"github.com/google/uuid"
)

// Validator is responsible for Product entity validation
type Validator interface {
	Product(*entity.Product) error
	UUID(string) (uuid.UUID, error)
	Body(error) error
}
