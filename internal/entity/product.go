package entity

import (
	"github.com/google/uuid"
)

// Product entity is the core business object
type Product struct {
	ID    uuid.UUID `json:"id" db:"uid"`
	Name  string    `json:"name" db:"name"`
	Price float64   `json:"price" db:"price"`
}
