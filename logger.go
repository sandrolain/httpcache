package httpcache

import (
	"log/slog"
	"sync"
)

var (
	logger     *slog.Logger
	loggerOnce sync.Once
)

// SetLogger sets a custom slog.Logger instance to be used by the httpcache package.
// If not set, the default slog logger will be used.
func SetLogger(l *slog.Logger) {
	logger = l
}

// GetLogger returns the configured logger or the default slog logger.
func GetLogger() *slog.Logger {
	loggerOnce.Do(func() {
		if logger == nil {
			logger = slog.Default()
		}
	})
	return logger
}
