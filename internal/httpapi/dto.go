package httpapi

import (
	"github.com/alkmc/restClean/internal/entity"
	"github.com/google/uuid"
)

type productResponse struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Price moneyDTO  `json:"price"`
}

type moneyDTO struct {
	MinorAmount int64           `json:"minorAmount"`
	Currency    entity.Currency `json:"currency"`
}

func toProductResponse(p entity.Product) productResponse {
	return productResponse{ID: p.ID, Name: p.Name, Price: toMoneyDTO(p.Price)}
}

func toProductsResponse(ps []entity.Product) []productResponse {
	out := make([]productResponse, len(ps))
	for i, p := range ps {
		out[i] = toProductResponse(p)
	}
	return out
}

func toMoney(in moneyInput) entity.Money {
	return entity.Money{MinorAmount: in.MinorAmount, Currency: in.Currency}
}

func toMoneyDTO(m entity.Money) moneyDTO {
	return moneyDTO{MinorAmount: m.MinorAmount, Currency: m.Currency}
}
