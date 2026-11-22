package config

import (
	"fmt"
	"io"
)

const redacted = "***"

// Secret holds a sensitive string that is redacted on serialization and printing.
type Secret string

// Reveal returns the underlying value.
func (s Secret) Reveal() string {
	return string(s)
}

// String is for direct callers, since fmt uses Format.
func (Secret) String() string {
	return redacted
}

// Format catches verbs like %#v that would skip String.
func (Secret) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, redacted)
}

// MarshalText covers encoders like encoding/xml.
func (Secret) MarshalText() ([]byte, error) {
	return []byte(redacted), nil
}

// MarshalJSON covers json.Marshal and slog.JSONHandler.
func (Secret) MarshalJSON() ([]byte, error) {
	return []byte(`"` + redacted + `"`), nil
}
