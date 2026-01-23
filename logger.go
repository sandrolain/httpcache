package httpcache

import (
	"log/slog"
)

// log returns the logger for the Transport.
// The logger is always initialized in NewTransport to slog.Default(),
// so for properly initialized transports no nil check is needed at runtime.
// We keep a minimal nil check for edge cases (e.g., zero-value Transport).
func (t *Transport) log() *slog.Logger {
	if t == nil || t.logger == nil {
		return slog.Default()
	}
	return t.logger
}
