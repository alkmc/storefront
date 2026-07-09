package http

import (
	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

type (
	productResponse struct {
		ID    uuid.UUID `json:"id"`
		Name  string    `json:"name"`
		Stock int64     `json:"stock"`
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
	purchaseResponse struct {
		ProductID      uuid.UUID `json:"productId"`
		Quantity       int64     `json:"quantity"`
		RemainingStock int64     `json:"remainingStock"`
	}
)

func toProductResponse(p domain.Product) productResponse {
	return productResponse{ID: p.ID, Name: p.Name, Stock: p.Stock, Price: toMoneyDTO(p.Price)}
}

func toPurchaseResponse(p domain.Product, qty int64) purchaseResponse {
	return purchaseResponse{ProductID: p.ID, Quantity: qty, RemainingStock: p.Stock}
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
		NextCursor: page.NextCursor(),
	}
}

func toMoney(in moneyInput) domain.Money {
	return domain.Money{MinorAmount: in.MinorAmount, Currency: in.Currency}
}

func toMoneyDTO(m domain.Money) moneyDTO {
	return moneyDTO{MinorAmount: m.MinorAmount, Currency: m.Currency}
}
