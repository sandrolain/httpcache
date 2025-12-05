package prometheus

import (
	"context"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/wrapper/metrics"
)

// Metric result constants.
const (
	resultHit     = "hit"
	resultMiss    = "miss"
	resultSuccess = "success"
	resultError   = "error"
)

// InstrumentedCache wraps an httpcache.Cache with Prometheus metrics
type InstrumentedCache struct {
	underlying httpcache.Cache
	collector  metrics.Collector
	backend    string // backend name: "memory", "redis", "leveldb", etc.
}

// NewInstrumentedCache creates a new instrumented cache that records metrics
// for all cache operations.
//
// Parameters:
//   - cache: the underlying cache implementation to wrap
//   - backend: the name of the cache backend (e.g., "disk", "redis", "leveldb")
//   - collector: the metrics collector (if nil, uses metrics.DefaultCollector)
//
// Example:
//
//	collector := prometheus.NewCollector()
//	cache := prometheus.NewInstrumentedCache(
//	    diskcache.New("/tmp/cache"),
//	    "disk",
//	    collector,
//	)
func NewInstrumentedCache(cache httpcache.Cache, backend string, collector metrics.Collector) *InstrumentedCache {
	if collector == nil {
		collector = metrics.DefaultCollector
	}

	return &InstrumentedCache{
		underlying: cache,
		collector:  collector,
		backend:    backend,
	}
}

// Get retrieves a value from the cache with metrics recording.
// Uses the provided context for cache operations.
func (c *InstrumentedCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	start := time.Now()
	value, ok, err := c.underlying.Get(ctx, key)
	duration := time.Since(start)

	result := resultMiss
	if err != nil {
		result = resultError
	} else if ok {
		result = resultHit
	}

	c.collector.RecordCacheOperation("get", c.backend, result, duration)

	return value, ok, err
}

// Set stores a value in the cache with metrics recording.
// Uses the provided context for cache operations.
func (c *InstrumentedCache) Set(ctx context.Context, key string, value []byte) error {
	start := time.Now()
	err := c.underlying.Set(ctx, key, value)
	duration := time.Since(start)

	result := resultSuccess
	if err != nil {
		result = resultError
	}

	c.collector.RecordCacheOperation("set", c.backend, result, duration)

	return err
}

// Delete removes a value from the cache with metrics recording.
// Uses the provided context for cache operations.
func (c *InstrumentedCache) Delete(ctx context.Context, key string) error {
	start := time.Now()
	err := c.underlying.Delete(ctx, key)
	duration := time.Since(start)

	result := resultSuccess
	if err != nil {
		result = resultError
	}

	c.collector.RecordCacheOperation("delete", c.backend, result, duration)

	return err
}

// Verify interface implementation at compile time
var _ httpcache.Cache = (*InstrumentedCache)(nil)
