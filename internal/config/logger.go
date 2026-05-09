package config

import (
	"fmt"
	"io"
	"log/slog"
	"time"
)

// NewLogger builds a JSON slog.Logger that writes to w using the configured level.
func (l Log) NewLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       l.Level,
		ReplaceAttr: replaceAttr,
	}))
}

func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	switch {
	case a.Key == slog.TimeKey:
		return slog.String(a.Key, a.Value.Time().UTC().Round(time.Millisecond).Format(time.RFC3339Nano))
	case a.Value.Kind() == slog.KindDuration:
		return slog.String(a.Key, fmt.Sprintf("%.2fms", float64(a.Value.Duration())/float64(time.Millisecond)))
	}
	return a
}
