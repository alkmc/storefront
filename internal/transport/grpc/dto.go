package grpc

import (
	"fmt"

	catalogv1 "github.com/alkmc/storefront/api/gen/catalog/v1"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

const (
	defaultLimit = 50
	maxLimit     = 200
)

func toProto(p domain.Product) *catalogv1.Product {
	return catalogv1.Product_builder{
		Id:   p.ID.String(),
		Name: p.Name,
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

// normalizeLimit clamps a requested page size into (0, maxLimit].
func normalizeLimit(n int) int {
	if n <= 0 {
		return defaultLimit
	}
	return min(n, maxLimit)
}

// parseCursor turns an optional id string into a keyset cursor.
func parseCursor(raw string) (uuid.NullUUID, error) {
	if raw == "" {
		return uuid.NullUUID{}, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.NullUUID{}, fmt.Errorf("invalid cursor: %q", raw)
	}
	return uuid.NullUUID{UUID: id, Valid: true}, nil
}

// nextCursor returns the id to resume after, or "" when the page is last.
func nextCursor(page domain.ProductPage) string {
	if !page.HasMore || len(page.Items) == 0 {
		return ""
	}
	return page.Items[len(page.Items)-1].ID.String()
}
