package http

import (
	"time"

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
		OrderID        uuid.UUID `json:"orderId"`
	}
	orderResponse struct {
		ID        uuid.UUID `json:"id"`
		ProductID uuid.UUID `json:"productId"`
		Quantity  int64     `json:"quantity"`
		UnitPrice moneyDTO  `json:"unitPrice"`
		CreatedAt time.Time `json:"createdAt"`
	}
	ordersPage struct {
		Items      []orderResponse `json:"items"`
		NextCursor string          `json:"nextCursor,omitempty"`
	}
)

func toProductResponse(p domain.Product) productResponse {
	return productResponse{ID: p.ID, Name: p.Name, Stock: p.Stock, Price: toMoneyDTO(p.Price)}
}

func toPurchaseResponse(p domain.Product, o domain.Order) purchaseResponse {
	return purchaseResponse{
		ProductID:      p.ID,
		Quantity:       o.Quantity,
		RemainingStock: p.Stock,
		OrderID:        uuid.UUID(o.ID),
	}
}

func toProductsPage(page domain.ProductPage) productsPage {
	return productsPage{
		Items:      mapSlice(page.Items, toProductResponse),
		NextCursor: page.NextCursor(),
	}
}

func toMoney(in moneyInput) domain.Money {
	return domain.Money{MinorAmount: in.MinorAmount, Currency: in.Currency}
}

func toMoneyDTO(m domain.Money) moneyDTO {
	return moneyDTO{MinorAmount: m.MinorAmount, Currency: m.Currency}
}

func toOrderResponse(o domain.Order) orderResponse {
	return orderResponse{
		ID:        uuid.UUID(o.ID),
		ProductID: o.ProductID,
		Quantity:  o.Quantity,
		UnitPrice: toMoneyDTO(o.UnitPrice),
		CreatedAt: o.CreatedAt,
	}
}

func toOrdersPage(page domain.OrderPage) ordersPage {
	return ordersPage{
		Items:      mapSlice(page.Items, toOrderResponse),
		NextCursor: page.NextCursor(),
	}
}

// mapSlice converts every element of in through f.
func mapSlice[T, U any](in []T, f func(T) U) []U {
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}
