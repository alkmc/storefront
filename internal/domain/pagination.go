package domain

import (
	"fmt"

	"github.com/google/uuid"
)

const (
	// DefaultPageSize is used when a client omits or under-specifies a page limit.
	DefaultPageSize = 50
	// MaxPageSize caps a client-requested page size.
	MaxPageSize = 200
)

// NormalizePageSize bounds n into (0, MaxPageSize], defaulting non-positive values.
func NormalizePageSize(n int) int {
	if n <= 0 {
		return DefaultPageSize
	}
	return min(n, MaxPageSize)
}

// ParseCursor turns an optional id string into a keyset cursor.
func ParseCursor(raw string) (uuid.NullUUID, error) {
	if raw == "" {
		return uuid.NullUUID{}, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.NullUUID{}, fmt.Errorf("invalid cursor: %q", raw)
	}
	return uuid.NullUUID{UUID: id, Valid: true}, nil
}

// NextCursor returns the id to resume after, or "" when this is the last page.
func (p ProductPage) NextCursor() string {
	if !p.HasMore || len(p.Items) == 0 {
		return ""
	}
	return p.Items[len(p.Items)-1].ID.String()
}
