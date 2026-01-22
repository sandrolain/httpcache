package httpcache

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// failureCache is a mock cache that can simulate various failure scenarios.
type failureCache struct {
	mu           sync.RWMutex
	items        map[string][]byte
	stales       map[string]bool
	failOnGet    bool
	failOnSet    bool
	failOnDelete bool
	getError     error
	setError     error
	deleteError  error
}

func newFailureCache() *failureCache {
	return &failureCache{
		items:       make(map[string][]byte),
		stales:      make(map[string]bool),
		getError:    errors.New("simulated get failure"),
		setError:    errors.New("simulated set failure"),
		deleteError: errors.New("simulated delete failure"),
	}
}

func (c *failureCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.failOnGet {
		return nil, false, c.getError
	}

	data, ok := c.items[key]
	return data, ok, nil
}

func (c *failureCache) Set(_ context.Context, key string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.failOnSet {
		return c.setError
	}

	c.items[key] = data
	return nil
}

func (c *failureCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.failOnDelete {
		return c.deleteError
	}

	delete(c.items, key)
	delete(c.stales, key)
	return nil
}

func (c *failureCache) MarkStale(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.items[key]; exists {
		c.stales[key] = true
	}
	return nil
}

func (c *failureCache) IsStale(_ context.Context, key string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.stales[key], nil
}

func (c *failureCache) GetStale(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.stales[key] {
		return nil, false, nil
	}

	data, ok := c.items[key]
	return data, ok, nil
}

// TestCacheSetFailure verifies that the client receives the response
// even if cache.Set() fails. The response should be streamed to the client
// and the error should be logged but not propagated.
func TestCacheSetFailure(t *testing.T) {
	cache := newFailureCache()
	cache.failOnSet = true

	transport := NewTransport(cache)

	responseBody := "response data from server"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responseBody))
	}))
	defer ts.Close()

	req, err := http.NewRequest("GET", ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Request should succeed despite cache failure
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("expected response despite cache Set failure, got error: %v", err)
	}
	defer resp.Body.Close()

	// Verify response body is correct
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(body) != responseBody {
		t.Errorf("expected body %q, got %q", responseBody, string(body))
	}

	// Verify response was not cached due to Set failure
	cache.mu.RLock()
	itemCount := len(cache.items)
	cache.mu.RUnlock()

	if itemCount != 0 {
		t.Errorf("expected cache to be empty after Set failure, got %d items", itemCount)
	}
}

// TestCacheGetFailure verifies that when cache.Get() fails,
// the transport falls back to fetching from the upstream server.
func TestCacheGetFailure(t *testing.T) {
	cache := newFailureCache()

	transport := NewTransport(cache)

	requestCount := 0
	responseBody := "fresh response"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responseBody))
	}))
	defer ts.Close()

	req, err := http.NewRequest("GET", ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	// First request - should succeed and cache
	resp1, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	if requestCount != 1 {
		t.Fatalf("expected 1 upstream request, got %d", requestCount)
	}

	// Verify response was cached
	cache.mu.RLock()
	itemCount := len(cache.items)
	cache.mu.RUnlock()

	if itemCount != 1 {
		t.Fatalf("expected 1 cached item, got %d", itemCount)
	}

	// Enable Get failure
	cache.failOnGet = true

	// Second request - should fail to get from cache and fetch fresh
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatalf("expected fallback to upstream when cache Get fails, got error: %v", err)
	}
	defer resp2.Body.Close()

	body, _ := io.ReadAll(resp2.Body)
	if string(body) != responseBody {
		t.Errorf("expected body %q, got %q", responseBody, string(body))
	}

	// Should have made another upstream request due to Get failure
	if requestCount != 2 {
		t.Errorf("expected 2 upstream requests after Get failure, got %d", requestCount)
	}
}

// TestCacheDeleteFailure verifies that Delete failures during
// invalidation are handled gracefully and don't break the request flow.
func TestCacheDeleteFailure(t *testing.T) {
	cache := newFailureCache()

	transport := NewTransport(cache)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			w.Header().Set("Cache-Control", "max-age=3600")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("response"))
		case "POST":
			// POST should invalidate cache
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("created"))
		}
	}))
	defer ts.Close()

	// First GET - cache the response
	req1, _ := http.NewRequest("GET", ts.URL, nil)
	resp1, err := transport.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	// Verify cached
	cache.mu.RLock()
	itemCount := len(cache.items)
	cache.mu.RUnlock()

	if itemCount != 1 {
		t.Fatalf("expected 1 cached item, got %d", itemCount)
	}

	// Enable Delete failure
	cache.failOnDelete = true

	// POST request - should attempt invalidation but handle Delete failure
	req2, _ := http.NewRequest("POST", ts.URL, strings.NewReader("data"))
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatalf("expected POST to succeed despite Delete failure, got error: %v", err)
	}
	defer resp2.Body.Close()

	body, _ := io.ReadAll(resp2.Body)
	if string(body) != "created" {
		t.Errorf("expected POST response body, got %q", string(body))
	}

	// Cache entry should still exist due to Delete failure
	cache.mu.RLock()
	itemCount = len(cache.items)
	cache.mu.RUnlock()

	if itemCount != 1 {
		t.Errorf("expected cache entry to remain after Delete failure, got %d items", itemCount)
	}
}

// TestCacheGetErrorConcurrent verifies thread-safety when multiple
// goroutines encounter cache Get errors simultaneously.
func TestCacheGetErrorConcurrent(t *testing.T) {
	cache := newFailureCache()
	cache.failOnGet = true

	transport := NewTransport(cache)

	var requestCount sync.Map
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Track requests by goroutine
		val, _ := requestCount.LoadOrStore(r.RemoteAddr, 0)
		requestCount.Store(r.RemoteAddr, val.(int)+1)

		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response"))
	}))
	defer ts.Close()

	// Launch multiple concurrent requests
	const numGoroutines = 20
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			req, _ := http.NewRequest("GET", ts.URL, nil)
			resp, err := transport.RoundTrip(req)
			if err != nil {
				errors <- err
				return
			}
			defer resp.Body.Close()

			_, err = io.Copy(io.Discard, resp.Body)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// All requests should have succeeded despite Get failures
	for err := range errors {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestCacheSetErrorConcurrent verifies thread-safety when multiple
// goroutines encounter cache Set errors simultaneously.
func TestCacheSetErrorConcurrent(t *testing.T) {
	cache := newFailureCache()
	cache.failOnSet = true

	transport := NewTransport(cache)

	requestCount := 0
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response"))
	}))
	defer ts.Close()

	// Launch multiple concurrent requests
	const numGoroutines = 20
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			req, _ := http.NewRequest("GET", ts.URL, nil)
			resp, err := transport.RoundTrip(req)
			if err != nil {
				errors <- err
				return
			}
			defer resp.Body.Close()

			_, err = io.Copy(io.Discard, resp.Body)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// All requests should have succeeded despite Set failures
	for err := range errors {
		t.Errorf("unexpected error: %v", err)
	}

	// Should have made numGoroutines upstream requests (no caching due to Set failure)
	mu.Lock()
	finalCount := requestCount
	mu.Unlock()

	if finalCount != numGoroutines {
		t.Errorf("expected %d upstream requests, got %d", numGoroutines, finalCount)
	}
}

// TestCacheErrorRecovery verifies that after cache errors are resolved,
// normal caching behavior resumes.
func TestCacheErrorRecovery(t *testing.T) {
	cache := newFailureCache()
	cache.failOnSet = true // Start with Set failures

	transport := NewTransport(cache)

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response"))
	}))
	defer ts.Close()

	// First request with Set failure
	req1, _ := http.NewRequest("GET", ts.URL, nil)
	resp1, err := transport.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	if requestCount != 1 {
		t.Fatalf("expected 1 upstream request, got %d", requestCount)
	}

	// Fix the cache
	cache.mu.Lock()
	cache.failOnSet = false
	cache.mu.Unlock()

	// Wait a bit to ensure clean state
	time.Sleep(50 * time.Millisecond)

	// Second request - should successfully cache now
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()

	if requestCount != 2 {
		t.Fatalf("expected 2 upstream requests, got %d", requestCount)
	}

	// Third request - should be cached
	req3, _ := http.NewRequest("GET", ts.URL, nil)
	resp3, err := transport.RoundTrip(req3)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()

	// Should still be 2 requests (third was cached)
	if requestCount != 2 {
		t.Errorf("expected cache hit on third request, got %d total upstream requests", requestCount)
	}

	// Verify X-From-Cache header
	if resp3.Header.Get("X-From-Cache") != "1" {
		t.Error("expected X-From-Cache header on cached response")
	}
}
