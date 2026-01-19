package httpcache

import (
	"sync/atomic"
	"time"
)

// TransportMetrics collects statistics about cache operations.
// All fields are safe for concurrent access using atomic operations.
type TransportMetrics struct {
	// Cache operation counters
	CacheHits     atomic.Int64 // Number of successful cache hits
	CacheMisses   atomic.Int64 // Number of cache misses (not found in cache)
	CacheErrors   atomic.Int64 // Number of errors during cache operations
	StaleServed   atomic.Int64 // Number of stale responses served
	Deduplication atomic.Int64 // Number of requests deduplicated via singleflight

	// Latency histogram buckets (in microseconds)
	// Buckets: <1ms, 1-5ms, 5-10ms, 10-25ms, 25-50ms, 50-100ms, 100-250ms, 250-500ms, 500-1000ms, >1000ms
	CacheLatencyBuckets [10]atomic.Int64

	// Gauge metrics
	CachedBytes atomic.Int64 // Total bytes currently cached (approximate)
}

// latency bucket boundaries in microseconds
var latencyBuckets = []int64{
	1000,    // 1ms
	5000,    // 5ms
	10000,   // 10ms
	25000,   // 25ms
	50000,   // 50ms
	100000,  // 100ms
	250000,  // 250ms
	500000,  // 500ms
	1000000, // 1000ms
	// >1000ms falls into bucket 9
}

// NewTransportMetrics creates a new TransportMetrics instance.
func NewTransportMetrics() *TransportMetrics {
	return &TransportMetrics{}
}

// HitRate returns the cache hit rate as a float between 0 and 1.
// Returns 0 if no requests have been made yet.
func (m *TransportMetrics) HitRate() float64 {
	hits := m.CacheHits.Load()
	misses := m.CacheMisses.Load()
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

// TotalRequests returns the total number of cache requests (hits + misses).
func (m *TransportMetrics) TotalRequests() int64 {
	return m.CacheHits.Load() + m.CacheMisses.Load()
}

// recordLatency records a cache operation latency in the appropriate bucket.
func (m *TransportMetrics) recordLatency(d time.Duration) {
	micros := d.Microseconds()

	for i, boundary := range latencyBuckets {
		if micros < boundary {
			m.CacheLatencyBuckets[i].Add(1)
			return
		}
	}
	// >1000ms
	m.CacheLatencyBuckets[9].Add(1)
}

// GetLatencyBucket returns the count for a specific latency bucket.
func (m *TransportMetrics) GetLatencyBucket(bucket int) int64 {
	if bucket < 0 || bucket >= len(m.CacheLatencyBuckets) {
		return 0
	}
	return m.CacheLatencyBuckets[bucket].Load()
}

// GetLatencyBucketBoundary returns the upper boundary in microseconds for a bucket.
// Returns -1 for the last bucket (>1000ms) which has no upper boundary.
func (m *TransportMetrics) GetLatencyBucketBoundary(bucket int) int64 {
	if bucket < 0 || bucket >= len(latencyBuckets) {
		return -1
	}
	return latencyBuckets[bucket]
}

// Reset resets all metrics to zero. Primarily useful for testing.
func (m *TransportMetrics) Reset() {
	m.CacheHits.Store(0)
	m.CacheMisses.Store(0)
	m.CacheErrors.Store(0)
	m.StaleServed.Store(0)
	m.Deduplication.Store(0)
	m.CachedBytes.Store(0)

	for i := range m.CacheLatencyBuckets {
		m.CacheLatencyBuckets[i].Store(0)
	}
}

// Snapshot returns a point-in-time snapshot of all metrics.
type MetricsSnapshot struct {
	CacheHits       int64
	CacheMisses     int64
	CacheErrors     int64
	StaleServed     int64
	Deduplication   int64
	CachedBytes     int64
	LatencyBuckets  [10]int64
	HitRate         float64
	TotalRequests   int64
	TimestampMillis int64
}

// Snapshot returns a consistent snapshot of all metrics at a point in time.
func (m *TransportMetrics) Snapshot() MetricsSnapshot {
	s := MetricsSnapshot{
		CacheHits:       m.CacheHits.Load(),
		CacheMisses:     m.CacheMisses.Load(),
		CacheErrors:     m.CacheErrors.Load(),
		StaleServed:     m.StaleServed.Load(),
		Deduplication:   m.Deduplication.Load(),
		CachedBytes:     m.CachedBytes.Load(),
		TimestampMillis: time.Now().UnixMilli(),
	}

	for i := range m.CacheLatencyBuckets {
		s.LatencyBuckets[i] = m.CacheLatencyBuckets[i].Load()
	}

	s.TotalRequests = s.CacheHits + s.CacheMisses
	if s.TotalRequests > 0 {
		s.HitRate = float64(s.CacheHits) / float64(s.TotalRequests)
	}

	return s
}
