package prometheus

import (
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/wrapper/metrics"
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
//   - backend: the name of the cache backend (e.g., "memory", "redis", "leveldb")
//   - collector: the metrics collector (if nil, uses metrics.DefaultCollector)
//
// Example:
//
//	collector := prometheus.NewCollector()
//	cache := prometheus.NewInstrumentedCache(
//	    httpcache.NewMemoryCache(),
//	    "memory",
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

// Get retrieves a value from the cache with metrics recording
func (c *InstrumentedCache) Get(key string) ([]byte, bool) {
	start := time.Now()
	value, ok := c.underlying.Get(key)
	duration := time.Since(start)

	result := "miss"
	if ok {
		result = "hit"
	}

	c.collector.RecordCacheOperation("get", c.backend, result, duration)

	return value, ok
}

// Set stores a value in the cache with metrics recording
func (c *InstrumentedCache) Set(key string, value []byte) {
	start := time.Now()
	c.underlying.Set(key, value)
	duration := time.Since(start)

	c.collector.RecordCacheOperation("set", c.backend, "success", duration)
}

// Delete removes a value from the cache with metrics recording
func (c *InstrumentedCache) Delete(key string) {
	start := time.Now()
	c.underlying.Delete(key)
	duration := time.Since(start)

	c.collector.RecordCacheOperation("delete", c.backend, "success", duration)
}

// Verify interface implementation at compile time
var _ httpcache.Cache = (*InstrumentedCache)(nil)
