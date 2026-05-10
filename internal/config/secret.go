package config

const redacted = "***"

// Secret holds a sensitive string that is redacted on serialization and printing.
type Secret string

// Reveal returns the underlying secret value.
func (s Secret) Reveal() string { return string(s) }

func (Secret) String() string { return redacted }

func (Secret) MarshalText() ([]byte, error) { return []byte(redacted), nil }

func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }
