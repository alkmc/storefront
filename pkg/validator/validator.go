package validator

import (
	"github.com/alkmc/restClean/pkg/entity"

	"github.com/google/uuid"
)

//Validator is responsible for Product entity validation
type Validator interface {
	Product(p *entity.Product) error
	UUID(uidStr string) (uuid.UUID, error)
	Body(err error) error
}
