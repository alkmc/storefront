package config

import (
	"fmt"
	"io"
	"log/slog"
	"time"
)

// msRFC3339 is RFC3339 with millisecond precision and fixed width.
const msRFC3339 = "2006-01-02T15:04:05.000Z07:00"

// NewLogger builds a JSON slog.Logger that writes to w using the configured level.
func (l Log) NewLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       l.Level,
		ReplaceAttr: replaceAttr,
	}))
}

func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	switch a.Value.Kind() {
	case slog.KindTime:
		return slog.String(a.Key, a.Value.Time().UTC().Format(msRFC3339))
	case slog.KindDuration:
		return slog.String(a.Key, fmt.Sprintf("%.2fms", float64(a.Value.Duration())/float64(time.Millisecond)))
	}
	return a
}
