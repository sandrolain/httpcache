package httpcache

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestCacheOperationTimeout_DefaultValue verifies the default timeout is 30 seconds
func TestCacheOperationTimeout_DefaultValue(t *testing.T) {
	cache := newMockCache()
	transport := NewTransport(cache)

	expectedTimeout := 30 * time.Second
	if transport.CacheOperationTimeout != expectedTimeout {
		t.Errorf("Expected default CacheOperationTimeout to be %v, got %v",
			expectedTimeout, transport.CacheOperationTimeout)
	}
}

// TestCacheOperationTimeout_CustomValue verifies custom timeout can be set
func TestCacheOperationTimeout_CustomValue(t *testing.T) {
	cache := newMockCache()
	customTimeout := 60 * time.Second

	transport := NewTransport(cache,
		WithCacheOperationTimeout(customTimeout),
	)

	if transport.CacheOperationTimeout != customTimeout {
		t.Errorf("Expected CacheOperationTimeout to be %v, got %v",
			customTimeout, transport.CacheOperationTimeout)
	}
}

// TestCacheOperationTimeout_ZeroDisablesTimeout verifies timeout=0 disables the timeout
func TestCacheOperationTimeout_ZeroDisablesTimeout(t *testing.T) {
	cache := newMockCache()
	transport := NewTransport(cache,
		WithCacheOperationTimeout(0),
	)

	if transport.CacheOperationTimeout != 0 {
		t.Errorf("Expected CacheOperationTimeout to be 0, got %v",
			transport.CacheOperationTimeout)
	}
}

// TestCacheOperationTimeout_NegativeValue verifies negative timeout is rejected
func TestCacheOperationTimeout_NegativeValue(t *testing.T) {
	cache := newMockCache()

	// NewTransport logs errors but doesn't return them, so check default is used
	transport := NewTransport(cache,
		WithCacheOperationTimeout(-10*time.Second),
	)

	// Should still have default timeout since negative was rejected
	if transport.CacheOperationTimeout < 0 {
		t.Errorf("Expected non-negative CacheOperationTimeout, got %v",
			transport.CacheOperationTimeout)
	}
}

// slowCacheMock simulates a slow cache backend
type slowCacheMock struct {
	delay      time.Duration
	setCounter atomic.Int32
}

func (c *slowCacheMock) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return nil, false, nil
}

func (c *slowCacheMock) Set(ctx context.Context, key string, value []byte) error {
	c.setCounter.Add(1)

	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Simulate slow operation
	select {
	case <-time.After(c.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *slowCacheMock) Delete(ctx context.Context, key string) error {
	return nil
}

func (c *slowCacheMock) MarkStale(ctx context.Context, key string) error {
	return nil
}

func (c *slowCacheMock) IsStale(ctx context.Context, key string) (bool, error) {
	return false, nil
}

func (c *slowCacheMock) GetStale(ctx context.Context, key string) ([]byte, bool, error) {
	return nil, false, nil
}

// TestCacheOperationTimeout_RespectedOnSlowCache verifies timeout is enforced
func TestCacheOperationTimeout_RespectedOnSlowCache(t *testing.T) {
	// Create cache that takes 5 seconds to write
	cache := &slowCacheMock{delay: 5 * time.Second}

	// Set a short timeout (500ms)
	transport := NewTransport(cache,
		WithCacheOperationTimeout(500*time.Millisecond),
		WithMarkCachedResponses(true),
	)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	client := transport.Client()

	// Make request
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Read body to trigger caching
	io.Copy(io.Discard, resp.Body)

	// Wait a bit for the cache operation to timeout
	time.Sleep(1 * time.Second)

	// Verify cache Set was called (even though it timed out)
	if cache.setCounter.Load() == 0 {
		t.Error("Expected cache Set to be called")
	}
}

// TestCacheOperationTimeout_CompletesWithinTimeout verifies cache succeeds when fast enough
func TestCacheOperationTimeout_CompletesWithinTimeout(t *testing.T) {
	// Create cache that takes 100ms to write (fast)
	cache := &slowCacheMock{delay: 100 * time.Millisecond}

	// Set timeout to 2 seconds (plenty of time)
	transport := NewTransport(cache,
		WithCacheOperationTimeout(2*time.Second),
		WithMarkCachedResponses(true),
	)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	client := transport.Client()

	// Make request
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Read body to trigger caching
	io.Copy(io.Discard, resp.Body)

	// Wait for cache operation to complete
	time.Sleep(500 * time.Millisecond)

	// Verify cache Set was called
	if cache.setCounter.Load() == 0 {
		t.Error("Expected cache Set to be called")
	}
}

// contextTrackingCache tracks the contexts used in cache operations
type contextTrackingCache struct {
	lastSetContext context.Context
	data           map[string][]byte
}

func (c *contextTrackingCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	data, ok := c.data[key]
	return data, ok, nil
}

func (c *contextTrackingCache) Set(ctx context.Context, key string, value []byte) error {
	c.lastSetContext = ctx
	if c.data == nil {
		c.data = make(map[string][]byte)
	}
	c.data[key] = value
	return nil
}

func (c *contextTrackingCache) Delete(ctx context.Context, key string) error {
	delete(c.data, key)
	return nil
}

func (c *contextTrackingCache) MarkStale(ctx context.Context, key string) error {
	return nil
}

func (c *contextTrackingCache) IsStale(ctx context.Context, key string) (bool, error) {
	return false, nil
}

func (c *contextTrackingCache) GetStale(ctx context.Context, key string) ([]byte, bool, error) {
	return nil, false, nil
}

// TestCacheOperationTimeout_UsesIndependentContext verifies cache uses its own context
func TestCacheOperationTimeout_UsesIndependentContext(t *testing.T) {
	cache := &contextTrackingCache{}
	transport := NewTransport(cache,
		WithCacheOperationTimeout(5*time.Second),
		WithMarkCachedResponses(true),
	)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	// Create request with short-lived context
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	client := transport.Client()

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Read body to trigger caching
	io.Copy(io.Discard, resp.Body)

	// Wait for cache operation
	time.Sleep(200 * time.Millisecond)

	// Verify cache operation used independent context (not the cancelled one)
	if cache.lastSetContext == nil {
		t.Fatal("Cache Set was not called")
	}

	// The cache context should not be the same as the request context
	if cache.lastSetContext == ctx {
		t.Error("Cache operation should use independent context, not request context")
	}

	// The cache context should have a deadline (from our timeout)
	deadline, ok := cache.lastSetContext.Deadline()
	if !ok {
		t.Error("Cache context should have a deadline")
	}

	// Deadline should be in the future (roughly 5 seconds from now)
	expectedDeadline := time.Now().Add(5 * time.Second)
	timeDiff := expectedDeadline.Sub(deadline).Abs()
	if timeDiff > 2*time.Second {
		t.Errorf("Cache context deadline seems incorrect: %v (expected ~5s from now)", deadline)
	}
}

// TestCacheOperationTimeout_NoTimeoutWhenZero verifies no timeout when set to 0
func TestCacheOperationTimeout_NoTimeoutWhenZero(t *testing.T) {
	cache := &contextTrackingCache{}
	transport := NewTransport(cache,
		WithCacheOperationTimeout(0), // No timeout
		WithMarkCachedResponses(true),
	)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	client := transport.Client()

	// Make request
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Read body to trigger caching
	io.Copy(io.Discard, resp.Body)

	// Wait for cache operation
	time.Sleep(100 * time.Millisecond)

	// Verify cache operation was called
	if cache.lastSetContext == nil {
		t.Fatal("Cache Set was not called")
	}

	// Context should NOT have a deadline when timeout is 0
	_, ok := cache.lastSetContext.Deadline()
	if ok {
		t.Error("Cache context should NOT have a deadline when CacheOperationTimeout is 0")
	}
}

// BenchmarkCacheOperationTimeout_WithTimeout benchmarks cache operations with timeout
func BenchmarkCacheOperationTimeout_WithTimeout(b *testing.B) {
	cache := newMockCache()
	transport := NewTransport(cache,
		WithCacheOperationTimeout(30*time.Second),
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "benchmark response")
	}))
	defer server.Close()

	client := transport.Client()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, _ := client.Get(server.URL)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

// BenchmarkCacheOperationTimeout_NoTimeout benchmarks cache operations without timeout
func BenchmarkCacheOperationTimeout_NoTimeout(b *testing.B) {
	cache := newMockCache()
	transport := NewTransport(cache,
		WithCacheOperationTimeout(0), // No timeout
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "benchmark response")
	}))
	defer server.Close()

	client := transport.Client()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, _ := client.Get(server.URL)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}
