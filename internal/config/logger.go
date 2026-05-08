package config

import (
	"fmt"
	"io"
	"log/slog"
)

// NewLogger builds a JSON slog.Logger that writes to w using the configured level.
func (l Log) NewLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       l.Level,
		ReplaceAttr: durationToMillis,
	}))
}

func durationToMillis(_ []string, a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindDuration {
		return slog.String(a.Key, fmt.Sprintf("%dms", a.Value.Duration().Milliseconds()))
	}
	return a
}
