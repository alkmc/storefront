package entity

import (
	"errors"

	"github.com/google/uuid"
)

// Product represents a purchasable item in the system
type Product struct {
	ID    uuid.UUID `json:"id" db:"id"`
	Name  string    `json:"name" db:"name"`
	Price float64   `json:"price" db:"price"`
}

// Validate ensures the product meets basic business rules before processing
func (p *Product) Validate() error {
	if p.Name == "" {
		return errors.New("the product name is empty")
	}
	if p.Price <= 0 {
		return errors.New("the product price must be positive")
	}
	return nil
}
