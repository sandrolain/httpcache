// Package metrics provides an interface for collecting HTTP cache metrics.
// This package defines a generic interface that can be implemented by various
// metrics systems (Prometheus, OpenTelemetry, Datadog, etc.) without adding
// dependencies to the core httpcache package.
package metrics

import (
	"time"
)

// Collector defines the interface for metrics collection.
// Implementations of this interface can collect metrics for various
// monitoring systems without requiring changes to the httpcache core.
type Collector interface {
	// RecordCacheOperation records a cache operation (get, set, delete)
	// Parameters:
	//   - operation: "get", "set", or "delete"
	//   - backend: cache backend name (e.g., "memory", "redis", "leveldb")
	//   - result: operation result (e.g., "hit", "miss", "success", "error")
	//   - duration: operation duration
	RecordCacheOperation(operation, backend, result string, duration time.Duration)

	// RecordCacheSize records the current size of the cache in bytes
	// Parameters:
	//   - backend: cache backend name
	//   - sizeBytes: current cache size in bytes
	RecordCacheSize(backend string, sizeBytes int64)

	// RecordCacheEntries records the current number of entries in cache
	// Parameters:
	//   - backend: cache backend name
	//   - count: number of entries
	RecordCacheEntries(backend string, count int64)

	// RecordHTTPRequest records an HTTP request through the cache transport
	// Parameters:
	//   - method: HTTP method (GET, HEAD, etc.)
	//   - cacheStatus: "hit", "miss", "revalidated", or "bypass"
	//   - statusCode: HTTP status code
	//   - duration: request duration
	RecordHTTPRequest(method, cacheStatus string, statusCode int, duration time.Duration)

	// RecordHTTPResponseSize records the size of an HTTP response
	// Parameters:
	//   - cacheStatus: "hit" or "miss"
	//   - sizeBytes: response size in bytes
	RecordHTTPResponseSize(cacheStatus string, sizeBytes int64)

	// RecordStaleResponse records when a stale response is served on error
	// Parameters:
	//   - errorType: type of error (e.g., "network", "server_error", "timeout")
	RecordStaleResponse(errorType string)
}

// NoOpCollector implements Collector with no-op operations.
// This is used as the default collector when metrics are not enabled,
// ensuring zero overhead for users who don't need metrics.
type NoOpCollector struct{}

// RecordCacheOperation does nothing (no-op implementation)
func (n *NoOpCollector) RecordCacheOperation(operation, backend, result string, duration time.Duration) {
}

// RecordCacheSize does nothing (no-op implementation)
func (n *NoOpCollector) RecordCacheSize(backend string, sizeBytes int64) {}

// RecordCacheEntries does nothing (no-op implementation)
func (n *NoOpCollector) RecordCacheEntries(backend string, count int64) {}

// RecordHTTPRequest does nothing (no-op implementation)
func (n *NoOpCollector) RecordHTTPRequest(method, cacheStatus string, statusCode int, duration time.Duration) {
}

// RecordHTTPResponseSize does nothing (no-op implementation)
func (n *NoOpCollector) RecordHTTPResponseSize(cacheStatus string, sizeBytes int64) {}

// RecordStaleResponse does nothing (no-op implementation)
func (n *NoOpCollector) RecordStaleResponse(errorType string) {}

// DefaultCollector is the default no-op collector used when metrics are not enabled
var DefaultCollector Collector = &NoOpCollector{}

// Verify that NoOpCollector implements Collector interface
var _ Collector = (*NoOpCollector)(nil)
