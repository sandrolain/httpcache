package httpcache

import (
	"sync/atomic"
)

// BufferPoolMetrics tracks buffer pool usage statistics.
// All fields use atomic operations for thread-safe updates and reads.
type BufferPoolMetrics struct {
	// Gets tracks the total number of buffer retrievals from the pool
	gets atomic.Int64

	// Puts tracks the total number of buffers returned to the pool
	puts atomic.Int64

	// PoolHits tracks buffers that were reused from the pool (had capacity > 0)
	poolHits atomic.Int64

	// PoolMiss tracks buffers that were newly allocated (capacity == 0)
	poolMiss atomic.Int64

	// Discarded tracks buffers that were too large to pool (> maxSize)
	discarded atomic.Int64
}

// bufferPoolMetrics is the global metrics instance for buffer pool monitoring
var bufferPoolMetrics = &BufferPoolMetrics{}

// GetBufferPoolMetrics returns the current buffer pool metrics.
// This function is safe to call concurrently and provides a snapshot
// of the current buffer pool usage statistics.
func GetBufferPoolMetrics() BufferPoolMetricsSnapshot {
	return BufferPoolMetricsSnapshot{
		Gets:      bufferPoolMetrics.gets.Load(),
		Puts:      bufferPoolMetrics.puts.Load(),
		PoolHits:  bufferPoolMetrics.poolHits.Load(),
		PoolMiss:  bufferPoolMetrics.poolMiss.Load(),
		Discarded: bufferPoolMetrics.discarded.Load(),
	}
}

// BufferPoolMetricsSnapshot represents a point-in-time snapshot of buffer pool metrics.
// Unlike BufferPoolMetrics which uses atomic fields, this struct uses plain int64
// fields for easier consumption by monitoring systems.
type BufferPoolMetricsSnapshot struct {
	// Gets is the total number of buffer retrievals from the pool
	Gets int64

	// Puts is the total number of buffers returned to the pool
	Puts int64

	// PoolHits is the number of buffers that were reused from the pool
	PoolHits int64

	// PoolMiss is the number of buffers that were newly allocated
	PoolMiss int64

	// Discarded is the number of buffers that were too large to pool
	Discarded int64
}

// PoolHitRate returns the pool hit rate as a percentage (0-100).
// Returns 0 if no Gets have occurred yet.
func (s BufferPoolMetricsSnapshot) PoolHitRate() float64 {
	if s.Gets == 0 {
		return 0
	}
	return float64(s.PoolHits) / float64(s.Gets) * 100
}

// DiscardRate returns the discard rate as a percentage (0-100) of total Puts.
// Returns 0 if no Puts have occurred yet.
func (s BufferPoolMetricsSnapshot) DiscardRate() float64 {
	if s.Puts == 0 {
		return 0
	}
	return float64(s.Discarded) / float64(s.Puts) * 100
}

// ResetBufferPoolMetrics resets all buffer pool metrics to zero.
// This is primarily useful for testing scenarios where you want to
// start with clean metrics. Use with caution in production.
func ResetBufferPoolMetrics() {
	bufferPoolMetrics.gets.Store(0)
	bufferPoolMetrics.puts.Store(0)
	bufferPoolMetrics.poolHits.Store(0)
	bufferPoolMetrics.poolMiss.Store(0)
	bufferPoolMetrics.discarded.Store(0)
}
