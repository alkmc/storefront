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

// ProductPage is a single keyset page
type ProductPage struct {
	Items   []Product
	HasMore bool
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
