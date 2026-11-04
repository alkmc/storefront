package config

import (
	"fmt"
	"io"
)

const redacted = "***"

// Secret holds a sensitive string that is redacted on serialization and printing.
type Secret string

// Reveal returns the underlying secret value.
func (s Secret) Reveal() string {
	return string(s)
}

func (Secret) String() string {
	return redacted
}

func (Secret) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, redacted)
}

func (Secret) MarshalText() ([]byte, error) {
	return []byte(redacted), nil
}

func (Secret) MarshalJSON() ([]byte, error) {
	return []byte(`"` + redacted + `"`), nil
}
