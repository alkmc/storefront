package validator

import (
	"errors"

	"github.com/alkmc/restClean/product/entity"

	"github.com/google/uuid"
)

type productValidator struct {
}

//NewValidator returns new Product Validator
func NewValidator() Validator {
	return &productValidator{}
}

func (v *productValidator) Product(p *entity.Product) error {
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

func (v *productValidator) UUID(id string) (uuid.UUID, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, err
	}
	return uid, nil
}
