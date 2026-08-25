// Package event defines the domain events published through the outbox.
package event

import (
	"errors"
	"time"
	"uuid"

	"github.com/alkmc/storefront/internal/domain"
)

const (
	TypeCreated   = "product.created"
	TypeUpdated   = "product.updated"
	TypeDeleted   = "product.deleted"
	TypePurchased = "product.purchased"
)

// ErrUndeliverable marks a permanent, non-retryable publish rejection (poison).
var ErrUndeliverable = errors.New("undeliverable")

type (
	// Event describes a product change. EventID (v7) lets consumers deduplicate.
	Event struct {
		EventID    uuid.UUID `json:"eventId"`
		Type       string    `json:"type"`
		ProductID  uuid.UUID `json:"productId"`
		Version    int64     `json:"version"`
		OccurredAt time.Time `json:"occurredAt"`
		Quantity   int64     `json:"quantity,omitzero"`
		Stock      int64     `json:"stock"`
	}
	// Record is a stored outbox row ready to publish without re-parsing its payload.
	Record struct {
		MessageID uuid.UUID
		Type      string
		Payload   []byte
	}
)

// New builds an event with a generated v7 id and the current time.
func New(eventType string, p domain.Product) Event {
	return Event{
		EventID:    uuid.NewV7(),
		Type:       eventType,
		ProductID:  uuid.UUID(p.ID),
		Version:    p.Version,
		OccurredAt: time.Now(),
		Stock:      p.Stock,
	}
}

// NewPurchased builds a purchase event carrying the purchased quantity.
func NewPurchased(p domain.Product, qty int64) Event {
	e := New(TypePurchased, p)
	e.Quantity = qty
	return e
}
