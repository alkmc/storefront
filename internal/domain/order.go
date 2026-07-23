package domain

import (
	"time"

	"github.com/google/uuid"
)

type (
	// UserID identifies the user who owns an order.
	UserID uuid.UUID
	// OrderID identifies an order.
	OrderID uuid.UUID
	// Order is a purchase snapshot owned by the buying user.
	Order struct {
		ID        OrderID
		UserID    UserID
		ProductID ProductID
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

// String returns the canonical UUID text of the user id.
func (id UserID) String() string {
	return uuid.UUID(id).String()
}

// String returns the canonical UUID text of the order id.
func (id OrderID) String() string {
	return uuid.UUID(id).String()
}

// NextCursor returns the id to resume after, or "" when this is the last page.
func (p OrderPage) NextCursor() string {
	if !p.HasMore || len(p.Items) == 0 {
		return ""
	}
	last := p.Items[len(p.Items)-1]
	return last.ID.String()
}
