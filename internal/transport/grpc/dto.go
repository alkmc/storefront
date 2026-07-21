package grpc

import (
	catalogv1 "github.com/alkmc/storefront/api/gen/catalog/v1"
	orderv1 "github.com/alkmc/storefront/api/gen/order/v1"
	"github.com/alkmc/storefront/internal/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProductProto(p domain.Product) *catalogv1.Product {
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

func toDomainMoney(m *catalogv1.Money) domain.Money {
	return domain.Money{
		MinorAmount: m.GetMinorAmount(),
		Currency:    domain.Currency(m.GetCurrency()),
	}
}

func toOrderProto(o domain.Order) *orderv1.Order {
	return orderv1.Order_builder{
		Id:        o.ID.String(),
		ProductId: o.ProductID.String(),
		Quantity:  o.Quantity,
		UnitPrice: orderv1.Money_builder{
			MinorAmount: o.UnitPrice.MinorAmount,
			Currency:    string(o.UnitPrice.Currency),
		}.Build(),
		CreatedAt: timestamppb.New(o.CreatedAt),
	}.Build()
}

// mapSlice converts every element of in through f.
func mapSlice[T, U any](in []T, f func(T) U) []U {
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}
