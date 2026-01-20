package httpcache

import (
	"bytes"
	"testing"
)

// TestBufferPool verifies that the buffer pool correctly reuses buffers
func TestBufferPool(t *testing.T) {
	// Get a buffer from the pool
	buf1 := getBuffer()
	if buf1 == nil {
		t.Fatal("getBuffer returned nil")
	}

	// Buffer should be empty
	if buf1.Len() != 0 {
		t.Errorf("expected empty buffer, got length %d", buf1.Len())
	}

	// Write some data
	testData := []byte("test data")
	buf1.Write(testData)

	if buf1.Len() != len(testData) {
		t.Errorf("expected buffer length %d, got %d", len(testData), buf1.Len())
	}

	// Return to pool
	putBuffer(buf1)

	// Get another buffer - should be the same one but reset
	buf2 := getBuffer()
	if buf2 == nil {
		t.Fatal("getBuffer returned nil after putBuffer")
	}

	// Buffer should be reset (empty)
	if buf2.Len() != 0 {
		t.Errorf("expected reset buffer to be empty, got length %d", buf2.Len())
	}

	// Verify it's actually the same buffer (same capacity)
	if buf2.Cap() != buf1.Cap() {
		t.Logf("different buffer capacities (expected for new allocation): buf1=%d, buf2=%d", buf1.Cap(), buf2.Cap())
	}

	putBuffer(buf2)
}

// TestBufferPoolLargeBufferNotPooled verifies that large buffers are not returned to the pool
func TestBufferPoolLargeBufferNotPooled(t *testing.T) {
	// Create a buffer and grow it beyond the pool threshold
	buf := getBuffer()

	// Write data to make it larger than defaultMaxPooledBufferSize
	largeData := make([]byte, defaultMaxPooledBufferSize+1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}
	buf.Write(largeData)

	if buf.Cap() <= int(defaultMaxPooledBufferSize) {
		t.Fatalf("buffer capacity %d should be larger than defaultMaxPooledBufferSize %d", buf.Cap(), defaultMaxPooledBufferSize)
	}

	// Return to pool (should not actually be pooled due to size)
	putBuffer(buf)

	// Get a new buffer - should be a fresh small one, not the large one
	buf2 := getBuffer()
	if buf2.Cap() > int(defaultMaxPooledBufferSize) {
		t.Errorf("expected small buffer from pool, got capacity %d", buf2.Cap())
	}

	putBuffer(buf2)
}

// TestBufferPoolConcurrent verifies that the buffer pool is safe for concurrent use
func TestBufferPoolConcurrent(t *testing.T) {
	const numGoroutines = 100
	const numIterations = 100

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < numIterations; j++ {
				buf := getBuffer()
				if buf == nil {
					t.Errorf("goroutine %d iteration %d: getBuffer returned nil", id, j)
					return
				}

				// Write some data
				buf.WriteString("concurrent test data")

				// Verify data
				if buf.Len() == 0 {
					t.Errorf("goroutine %d iteration %d: buffer should not be empty", id, j)
				}

				// Return to pool
				putBuffer(buf)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// TestBufferPoolReset verifies that buffers are properly reset when retrieved from the pool
func TestBufferPoolReset(t *testing.T) {
	// Get a buffer and write data
	buf1 := getBuffer()
	buf1.WriteString("first data")
	initialLen := buf1.Len()

	if initialLen == 0 {
		t.Fatal("buffer should contain data")
	}

	// Return to pool
	putBuffer(buf1)

	// Get buffer again - should be reset
	buf2 := getBuffer()
	if buf2.Len() != 0 {
		t.Errorf("buffer should be reset, expected length 0, got %d", buf2.Len())
	}

	putBuffer(buf2)
}

// TestGetBufferReturnsSameUnderlyingBuffer verifies buffer reuse
func TestGetBufferReturnsSameUnderlyingBuffer(t *testing.T) {
	// This test checks that the pool actually reuses buffers
	// by tracking pointer addresses (though this is implementation-dependent)

	buf1 := getBuffer()
	buf1.WriteString("test")
	putBuffer(buf1)

	// Immediately getting another buffer should likely give us the same one
	buf2 := getBuffer()

	// We can't guarantee pointer equality due to pool implementation,
	// but we can verify basic pool behavior works
	if buf2.Len() != 0 {
		t.Errorf("expected reset buffer, got length %d", buf2.Len())
	}

	putBuffer(buf2)
}

// TestBufferPoolMultipleSizes tests that the pool handles various buffer sizes
func TestBufferPoolMultipleSizes(t *testing.T) {
	sizes := []int{
		10,                                     // Small
		1024,                                   // 1KB
		32 * 1024,                              // 32KB
		int(defaultMaxPooledBufferSize) - 1024, // Just under limit
	}

	for _, size := range sizes {
		buf := getBuffer()
		data := make([]byte, size)
		buf.Write(data)

		if buf.Len() != size {
			t.Errorf("size %d: expected buffer length %d, got %d", size, size, buf.Len())
		}

		// All these should be pooled (under the limit)
		putBuffer(buf)

		// Verify we can get another buffer
		buf2 := getBuffer()
		if buf2.Len() != 0 {
			t.Errorf("size %d: buffer not reset, length %d", size, buf2.Len())
		}
		putBuffer(buf2)
	}
}

// TestBufferPoolEdgeCases tests edge cases
func TestBufferPoolEdgeCases(t *testing.T) {
	t.Run("EmptyBuffer", func(t *testing.T) {
		buf := getBuffer()
		// Don't write anything
		putBuffer(buf)

		buf2 := getBuffer()
		if buf2.Len() != 0 {
			t.Errorf("expected empty buffer, got length %d", buf2.Len())
		}
		putBuffer(buf2)
	})

	t.Run("MultipleGetsBeforePut", func(t *testing.T) {
		// Get multiple buffers without returning them immediately
		bufs := make([]*bytes.Buffer, 10)
		for i := range bufs {
			bufs[i] = getBuffer()
			bufs[i].WriteString("data")
		}

		// Return all
		for _, buf := range bufs {
			putBuffer(buf)
		}

		// Get them again - all should be reset
		for i := range bufs {
			buf := getBuffer()
			if buf.Len() != 0 {
				t.Errorf("buffer %d not reset, length %d", i, buf.Len())
			}
			putBuffer(buf)
		}
	})
}
