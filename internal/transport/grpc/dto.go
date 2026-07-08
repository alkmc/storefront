package grpc

import (
	catalogv1 "github.com/alkmc/storefront/api/gen/catalog/v1"
	"github.com/alkmc/storefront/internal/domain"
)

func toProto(p domain.Product) *catalogv1.Product {
	return catalogv1.Product_builder{
		Id:    p.ID.String(),
		Name:  p.Name,
		Stock: p.Stock,
		Price: catalogv1.Money_builder{
			MinorAmount: p.Price.MinorAmount,
			Currency:    string(p.Price.Currency),
		}.Build(),
	}.Build()
}

func toProtos(ps []domain.Product) []*catalogv1.Product {
	out := make([]*catalogv1.Product, len(ps))
	for i, p := range ps {
		out[i] = toProto(p)
	}
	return out
}

func toDomainMoney(m *catalogv1.Money) domain.Money {
	return domain.Money{
		MinorAmount: m.GetMinorAmount(),
		Currency:    domain.Currency(m.GetCurrency()),
	}
}
