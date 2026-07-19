package domain

import (
	"time"

	"github.com/google/uuid"
)

type (
	// Order is a purchase snapshot owned by the buying user.
	Order struct {
		ID        uuid.UUID
		UserID    uuid.UUID
		ProductID uuid.UUID
		Quantity  int64
		UnitPrice Money
		CreatedAt time.Time
	}
	// OrderPage is a single keyset page of orders.
	OrderPage struct {
		Items   []Order
		HasMore bool
	}
)

// NextCursor returns the id to resume after, or "" when this is the last page.
func (p OrderPage) NextCursor() string {
	if !p.HasMore || len(p.Items) == 0 {
		return ""
	}
	return p.Items[len(p.Items)-1].ID.String()
}
