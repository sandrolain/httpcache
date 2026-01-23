package httpcache

import (
	"bytes"
	"testing"
)

// TestBufferPoolMetricsTracking verifies that buffer pool metrics are tracked correctly.
func TestBufferPoolMetricsTracking(t *testing.T) {
	// Reset metrics before test
	ResetBufferPoolMetrics()

	// Get initial snapshot
	initial := GetBufferPoolMetrics()
	if initial.Gets != 0 || initial.Puts != 0 {
		t.Fatalf("expected zero metrics after reset, got Gets=%d Puts=%d", initial.Gets, initial.Puts)
	}

	// Get a buffer
	buf := getBuffer()

	// Check metrics after get
	metrics := GetBufferPoolMetrics()
	if metrics.Gets != 1 {
		t.Errorf("expected Gets=1, got %d", metrics.Gets)
	}

	// First get should be either a hit (if pool had buffers) or miss (if empty)
	if metrics.PoolHits+metrics.PoolMiss != 1 {
		t.Errorf("expected PoolHits+PoolMiss=1, got %d+%d=%d",
			metrics.PoolHits, metrics.PoolMiss, metrics.PoolHits+metrics.PoolMiss)
	}

	// Write some data
	buf.WriteString("test data")

	// Put buffer back
	putBuffer(buf)

	// Check metrics after put
	metrics = GetBufferPoolMetrics()
	if metrics.Puts != 1 {
		t.Errorf("expected Puts=1, got %d", metrics.Puts)
	}

	// Buffer should not be discarded (small size)
	if metrics.Discarded != 0 {
		t.Errorf("expected Discarded=0 for small buffer, got %d", metrics.Discarded)
	}

	// Get another buffer (should be a pool hit now)
	buf2 := getBuffer()

	metrics = GetBufferPoolMetrics()
	if metrics.Gets != 2 {
		t.Errorf("expected Gets=2, got %d", metrics.Gets)
	}

	if metrics.PoolHits < 1 {
		t.Errorf("expected at least 1 PoolHit after reusing buffer, got %d", metrics.PoolHits)
	}

	// Clean up
	putBuffer(buf2)
}

// TestBufferPoolMetricsLargeBuffer verifies that large buffers are tracked as discarded.
func TestBufferPoolMetricsLargeBuffer(t *testing.T) {
	ResetBufferPoolMetrics()

	// Get a buffer
	buf := getBuffer()

	// Write a lot of data to make it large
	largeData := make([]byte, 100*1024) // 100KB
	buf.Write(largeData)

	// Put it back with default limit (64KB)
	putBuffer(buf)

	// Check metrics
	metrics := GetBufferPoolMetrics()
	if metrics.Discarded != 1 {
		t.Errorf("expected Discarded=1 for large buffer, got %d", metrics.Discarded)
	}
}

// TestBufferPoolMetricsWithCustomLimit verifies metrics with custom size limits.
func TestBufferPoolMetricsWithCustomLimit(t *testing.T) {
	ResetBufferPoolMetrics()

	// Get a buffer
	buf := getBuffer()

	// Write moderate amount of data
	buf.Write(make([]byte, 50*1024)) // 50KB

	// Put with small custom limit (should discard)
	putBufferWithLimit(buf, 10*1024) // 10KB limit

	metrics := GetBufferPoolMetrics()
	if metrics.Discarded != 1 {
		t.Errorf("expected Discarded=1 with small custom limit, got %d", metrics.Discarded)
	}

	// Get another buffer
	buf2 := getBuffer()
	buf2.Write(make([]byte, 5*1024)) // 5KB

	// Put with large custom limit (should pool)
	putBufferWithLimit(buf2, 100*1024) // 100KB limit

	metrics = GetBufferPoolMetrics()
	if metrics.Puts != 2 {
		t.Errorf("expected Puts=2, got %d", metrics.Puts)
	}
	if metrics.Discarded != 1 {
		t.Errorf("expected Discarded=1 (only first buffer), got %d", metrics.Discarded)
	}
}

// TestBufferPoolMetricsConcurrent verifies metrics are thread-safe.
func TestBufferPoolMetricsConcurrent(t *testing.T) {
	ResetBufferPoolMetrics()

	const goroutines = 50
	const iterations = 100

	done := make(chan bool)

	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < iterations; j++ {
				buf := getBuffer()
				buf.WriteString("test")
				putBuffer(buf)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < goroutines; i++ {
		<-done
	}

	metrics := GetBufferPoolMetrics()

	expectedOps := int64(goroutines * iterations)
	if metrics.Gets != expectedOps {
		t.Errorf("expected Gets=%d, got %d", expectedOps, metrics.Gets)
	}
	if metrics.Puts != expectedOps {
		t.Errorf("expected Puts=%d, got %d", expectedOps, metrics.Puts)
	}

	// All operations should be either hits or misses
	if metrics.PoolHits+metrics.PoolMiss != expectedOps {
		t.Errorf("expected PoolHits+PoolMiss=%d, got %d+%d=%d",
			expectedOps, metrics.PoolHits, metrics.PoolMiss, metrics.PoolHits+metrics.PoolMiss)
	}

	// No buffers should be discarded (all small)
	if metrics.Discarded != 0 {
		t.Errorf("expected Discarded=0, got %d", metrics.Discarded)
	}
}

// TestBufferPoolMetricsSnapshot verifies snapshot methods.
func TestBufferPoolMetricsSnapshot(t *testing.T) {
	ResetBufferPoolMetrics()

	// Perform some operations
	for i := 0; i < 10; i++ {
		buf := getBuffer()
		putBuffer(buf)
	}

	snapshot := GetBufferPoolMetrics()

	// Test PoolHitRate
	hitRate := snapshot.PoolHitRate()
	if hitRate < 0 || hitRate > 100 {
		t.Errorf("PoolHitRate should be 0-100, got %f", hitRate)
	}

	// Test DiscardRate
	discardRate := snapshot.DiscardRate()
	if discardRate < 0 || discardRate > 100 {
		t.Errorf("DiscardRate should be 0-100, got %f", discardRate)
	}

	// For small buffers, discard rate should be 0
	if discardRate != 0 {
		t.Errorf("expected DiscardRate=0 for small buffers, got %f", discardRate)
	}
}

// TestBufferPoolMetricsSnapshotEdgeCases tests edge cases for snapshot calculations.
func TestBufferPoolMetricsSnapshotEdgeCases(t *testing.T) {
	// Test with zero Gets
	snapshot := BufferPoolMetricsSnapshot{
		Gets:      0,
		PoolHits:  0,
		PoolMiss:  0,
		Puts:      0,
		Discarded: 0,
	}

	if rate := snapshot.PoolHitRate(); rate != 0 {
		t.Errorf("PoolHitRate with zero Gets should be 0, got %f", rate)
	}

	if rate := snapshot.DiscardRate(); rate != 0 {
		t.Errorf("DiscardRate with zero Puts should be 0, got %f", rate)
	}

	// Test with some operations
	snapshot = BufferPoolMetricsSnapshot{
		Gets:      100,
		PoolHits:  80,
		PoolMiss:  20,
		Puts:      100,
		Discarded: 5,
	}

	expectedHitRate := 80.0
	if rate := snapshot.PoolHitRate(); rate != expectedHitRate {
		t.Errorf("expected PoolHitRate=%f, got %f", expectedHitRate, rate)
	}

	expectedDiscardRate := 5.0
	if rate := snapshot.DiscardRate(); rate != expectedDiscardRate {
		t.Errorf("expected DiscardRate=%f, got %f", expectedDiscardRate, rate)
	}
}

// TestBufferPoolMetricsReset verifies that ResetBufferPoolMetrics works correctly.
func TestBufferPoolMetricsReset(t *testing.T) {
	// Perform some operations
	for i := 0; i < 5; i++ {
		buf := getBuffer()
		putBuffer(buf)
	}

	// Verify metrics are non-zero
	metrics := GetBufferPoolMetrics()
	if metrics.Gets == 0 || metrics.Puts == 0 {
		t.Fatal("expected non-zero metrics before reset")
	}

	// Reset
	ResetBufferPoolMetrics()

	// Verify all metrics are zero
	metrics = GetBufferPoolMetrics()
	if metrics.Gets != 0 {
		t.Errorf("expected Gets=0 after reset, got %d", metrics.Gets)
	}
	if metrics.Puts != 0 {
		t.Errorf("expected Puts=0 after reset, got %d", metrics.Puts)
	}
	if metrics.PoolHits != 0 {
		t.Errorf("expected PoolHits=0 after reset, got %d", metrics.PoolHits)
	}
	if metrics.PoolMiss != 0 {
		t.Errorf("expected PoolMiss=0 after reset, got %d", metrics.PoolMiss)
	}
	if metrics.Discarded != 0 {
		t.Errorf("expected Discarded=0 after reset, got %d", metrics.Discarded)
	}
}

// TestBufferPoolMetricsMultipleGetsAndPuts verifies accounting with multiple operations.
func TestBufferPoolMetricsMultipleGetsAndPuts(t *testing.T) {
	ResetBufferPoolMetrics()

	buffers := make([]*bytes.Buffer, 10)

	// Get multiple buffers
	for i := 0; i < 10; i++ {
		buffers[i] = getBuffer()
		buffers[i].WriteString("data")
	}

	metrics := GetBufferPoolMetrics()
	if metrics.Gets != 10 {
		t.Errorf("expected Gets=10, got %d", metrics.Gets)
	}

	// Put them all back
	for i := 0; i < 10; i++ {
		putBuffer(buffers[i])
	}

	metrics = GetBufferPoolMetrics()
	if metrics.Puts != 10 {
		t.Errorf("expected Puts=10, got %d", metrics.Puts)
	}

	// Get them again (should all be hits now)
	for i := 0; i < 10; i++ {
		buf := getBuffer()
		putBuffer(buf)
	}

	metrics = GetBufferPoolMetrics()
	if metrics.Gets != 20 {
		t.Errorf("expected Gets=20, got %d", metrics.Gets)
	}

	// Should have at least 10 pool hits from second round
	if metrics.PoolHits < 10 {
		t.Errorf("expected at least 10 PoolHits, got %d", metrics.PoolHits)
	}
}
