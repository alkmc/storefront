package entity

import (
	"net/http"

	"github.com/alkmc/restClean/internal/renderer"

	"github.com/google/uuid"
)

// Product entity is the core business object
type Product struct {
	ID    uuid.UUID `json:"id" db:"uid"`
	Name  string    `json:"name" db:"name"`
	Price float64   `json:"price" db:"price"`
}

// JSON serializes the Product entity into the response body
func (p *Product) JSON(w http.ResponseWriter) {
	renderer.JSON(w, http.StatusOK, p)
}
