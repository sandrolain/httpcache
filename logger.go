package httpcache

import (
	"log/slog"
)

// log returns the logger for the Transport.
// If a logger is configured on the Transport, it returns that logger.
// Otherwise, it falls back to the default slog logger.
func (t *Transport) log() *slog.Logger {
	if t != nil && t.logger != nil {
		return t.logger
	}
	return slog.Default()
}
