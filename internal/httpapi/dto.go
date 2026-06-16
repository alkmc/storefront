package httpapi

import (
	"github.com/alkmc/storefront/internal/entity"
	"github.com/google/uuid"
)

type (
	productResponse struct {
		ID    uuid.UUID `json:"id"`
		Name  string    `json:"name"`
		Price moneyDTO  `json:"price"`
	}
	moneyDTO struct {
		MinorAmount int64           `json:"minorAmount"`
		Currency    entity.Currency `json:"currency"`
	}
	productsPage struct {
		Items      []productResponse `json:"items"`
		NextCursor string            `json:"nextCursor,omitempty"`
	}
)

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

func toProductsPage(page entity.ProductPage) productsPage {
	return productsPage{
		Items:      toProductsResponse(page.Items),
		NextCursor: nextCursor(page),
	}
}

func nextCursor(page entity.ProductPage) string {
	if !page.HasMore || len(page.Items) == 0 {
		return ""
	}
	return page.Items[len(page.Items)-1].ID.String()
}

func toMoney(in moneyInput) entity.Money {
	return entity.Money{MinorAmount: in.MinorAmount, Currency: in.Currency}
}

func toMoneyDTO(m entity.Money) moneyDTO {
	return moneyDTO{MinorAmount: m.MinorAmount, Currency: m.Currency}
}
