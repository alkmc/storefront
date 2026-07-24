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

// Cursor is an optional keyset position: when valid, the page resumes strictly after id.
type Cursor struct {
	id    uuid.UUID
	valid bool
}

// NewCursor returns a cursor that resumes after id.
func NewCursor(id uuid.UUID) Cursor { return Cursor{id: id, valid: true} }

// After returns the id to resume after and whether the cursor is set.
func (c Cursor) After() (uuid.UUID, bool) { return c.id, c.valid }

// NormalizePageSize bounds n into (0, MaxPageSize], defaulting non-positive values.
func NormalizePageSize(n int) int {
	if n <= 0 {
		return DefaultPageSize
	}
	return min(n, MaxPageSize)
}

// ParseCursor turns an optional id string into a keyset cursor.
func ParseCursor(raw string) (Cursor, error) {
	if raw == "" {
		return Cursor{}, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return Cursor{}, fmt.Errorf("invalid cursor: %q", raw)
	}
	return NewCursor(id), nil
}
