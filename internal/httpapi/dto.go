package httpapi

import (
	"github.com/alkmc/storefront/internal/domain"
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
		Currency    domain.Currency `json:"currency"`
	}
	productsPage struct {
		Items      []productResponse `json:"items"`
		NextCursor string            `json:"nextCursor,omitempty"`
	}
)

func toProductResponse(p domain.Product) productResponse {
	return productResponse{ID: p.ID, Name: p.Name, Price: toMoneyDTO(p.Price)}
}

func toProductsResponse(ps []domain.Product) []productResponse {
	out := make([]productResponse, len(ps))
	for i, p := range ps {
		out[i] = toProductResponse(p)
	}
	return out
}

func toProductsPage(page domain.ProductPage) productsPage {
	return productsPage{
		Items:      toProductsResponse(page.Items),
		NextCursor: nextCursor(page),
	}
}

func nextCursor(page domain.ProductPage) string {
	if !page.HasMore || len(page.Items) == 0 {
		return ""
	}
	return page.Items[len(page.Items)-1].ID.String()
}

func toMoney(in moneyInput) domain.Money {
	return domain.Money{MinorAmount: in.MinorAmount, Currency: in.Currency}
}

func toMoneyDTO(m domain.Money) moneyDTO {
	return moneyDTO{MinorAmount: m.MinorAmount, Currency: m.Currency}
}
