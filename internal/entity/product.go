package entity

import (
	"errors"

	"github.com/google/uuid"
)

// ErrNotFound signals a missing aggregate at the domain boundary.
var ErrNotFound = errors.New("entity: not found")

// Product represents a purchasable item in the system
type Product struct {
	ID    uuid.UUID
	Name  string
	Price Money
}

// Validate ensures the product meets basic business rules before processing
func (p *Product) Validate() error {
	if p.Name == "" {
		return errors.New("the product name is empty")
	}
	if err := p.Price.Validate(); err != nil {
		return err
	}
	return nil
}
