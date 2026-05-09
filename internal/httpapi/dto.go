package httpapi

import (
	"github.com/alkmc/restClean/internal/entity"
	"github.com/google/uuid"
)

type productResponse struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Price float64   `json:"price"`
}

func toProductResponse(p entity.Product) productResponse {
	return productResponse{ID: p.ID, Name: p.Name, Price: p.Price}
}

func toProductsResponse(ps []entity.Product) []productResponse {
	out := make([]productResponse, len(ps))
	for i, p := range ps {
		out[i] = toProductResponse(p)
	}
	return out
}
