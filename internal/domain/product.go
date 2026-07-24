package domain

import (
	"errors"

	"github.com/google/uuid"
)

var (
	// ErrNotFound signals a missing aggregate at the domain boundary.
	ErrNotFound = errors.New("domain: not found")
	// ErrUnavailable signals that a backing dependency is temporarily unavailable.
	ErrUnavailable = errors.New("domain: dependency unavailable")
	// ErrInsufficientStock signals a purchase for more units than are in stock.
	ErrInsufficientStock = errors.New("domain: insufficient stock")
	// ErrProductInUse signals a delete of a product that has recorded orders.
	ErrProductInUse = errors.New("domain: product in use")
)

type (
	// ProductID identifies a product.
	ProductID uuid.UUID
	// Product represents a purchasable item in the system
	Product struct {
		ID      ProductID
		Name    string
		Price   Money
		Stock   int64
		Version int64
	}
	// ProductPage is a single keyset page
	ProductPage struct {
		Items   []Product
		HasMore bool
	}
)

// String returns the canonical UUID text of the product id.
func (id ProductID) String() string {
	return uuid.UUID(id).String()
}

const (
	minPurchaseQuantity = 1
	// maxPurchaseQuantity is an arbitrary per-request sanity cap.
	// It rejects absurd quantities as invalid input rather than as insufficient stock.
	maxPurchaseQuantity = 10000
)

// ValidPurchaseQuantity reports whether qty falls within the purchasable range.
func ValidPurchaseQuantity(qty int64) bool {
	return qty >= minPurchaseQuantity && qty <= maxPurchaseQuantity
}

// Validate ensures the product meets basic business rules before processing
func (p *Product) Validate() error {
	if p.Name == "" {
		return errors.New("the product name is empty")
	}
	if err := p.Price.Validate(); err != nil {
		return err
	}
	if p.Stock < 0 {
		return errors.New("the product stock must not be negative")
	}
	return nil
}

// NextCursor returns the id to resume after, or "" when this is the last page.
func (p ProductPage) NextCursor() string {
	if !p.HasMore || len(p.Items) == 0 {
		return ""
	}
	return p.Items[len(p.Items)-1].ID.String()
}
