package httpcache

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGetCacheKeyMemoization verifies that getCacheKey computes the key once
// and memoizes it in the request context.
func TestGetCacheKeyMemoization(t *testing.T) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache)

	req, err := http.NewRequest("GET", "http://example.com/test", nil)
	if err != nil {
		t.Fatal(err)
	}

	// First call should compute and store
	key1, req := transport.getCacheKey(req)
	if key1 == "" {
		t.Fatal("expected non-empty cache key")
	}

	// Second call should retrieve from context
	key2, _ := transport.getCacheKey(req)
	if key2 != key1 {
		t.Errorf("expected same key from context, got %q != %q", key2, key1)
	}

	// Verify key is in context
	keyFromCtx := getCacheKeyFromContext(req)
	if keyFromCtx != key1 {
		t.Errorf("expected key in context: got %q != %q", keyFromCtx, key1)
	}
}

// TestGetCacheKeyWithHeaders verifies that cache key includes configured headers.
func TestGetCacheKeyWithHeaders(t *testing.T) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache, WithCacheKeyHeaders([]string{"X-Custom"}))

	req1, _ := http.NewRequest("GET", "http://example.com/test", nil)
	req1.Header.Set("X-Custom", "value1")

	req2, _ := http.NewRequest("GET", "http://example.com/test", nil)
	req2.Header.Set("X-Custom", "value2")

	key1, _ := transport.getCacheKey(req1)
	key2, _ := transport.getCacheKey(req2)

	if key1 == key2 {
		t.Error("expected different cache keys for different header values")
	}

	// Both should contain base URL
	expectedBase := "http://example.com/test"
	if key1[:len(expectedBase)] != expectedBase {
		t.Errorf("expected key to start with %q, got %q", expectedBase, key1)
	}
}

// TestGetCacheKeyFromContextEmpty verifies behavior with empty context.
func TestGetCacheKeyFromContextEmpty(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com/test", nil)

	key := getCacheKeyFromContext(req)
	if key != "" {
		t.Errorf("expected empty key from empty context, got %q", key)
	}
}

// TestCacheKeyInRoundTrip verifies that cache key is computed and used correctly.
func TestCacheKeyInRoundTrip(t *testing.T) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write([]byte("response body"))
	}))
	defer ts.Close()

	// First request - should compute cache key and store in cache
	req1, _ := http.NewRequest("GET", ts.URL+"/test", nil)
	resp1, err := transport.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if string(body1) != "response body" {
		t.Errorf("unexpected body: %s", body1)
	}

	// Verify something was stored in cache
	if len(cache.items) == 0 {
		t.Fatal("expected cache to have entries after first request")
	}

	// Second request - should use same cache key and retrieve from cache
	req2, _ := http.NewRequest("GET", ts.URL+"/test", nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if string(body2) != "response body" {
		t.Errorf("unexpected cached body: %s", body2)
	}

	// Verify XFromCache header is set
	if resp2.Header.Get(XFromCache) != "1" {
		t.Error("expected XFromCache header on cached response")
	}
}

// TestCacheKeyWithMethodPrefixMemoization verifies POST/PUT methods include method prefix.
func TestCacheKeyWithMethodPrefixMemoization(t *testing.T) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache)

	reqGET, _ := http.NewRequest("GET", "http://example.com/test", nil)
	reqPOST, _ := http.NewRequest("POST", "http://example.com/test", nil)

	keyGET, _ := transport.getCacheKey(reqGET)
	keyPOST, _ := transport.getCacheKey(reqPOST)

	if keyGET == keyPOST {
		t.Error("expected different cache keys for GET vs POST")
	}

	// GET should not have method prefix
	if keyGET != "http://example.com/test" {
		t.Errorf("expected GET key without prefix: %q", keyGET)
	}

	// POST should have method prefix
	if keyPOST != "POST http://example.com/test" {
		t.Errorf("expected POST key with prefix: %q", keyPOST)
	}
}

// TestCacheKeyContextIsolation verifies that cache keys from different requests
// don't interfere with each other.
func TestCacheKeyContextIsolation(t *testing.T) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache)

	req1, _ := http.NewRequest("GET", "http://example.com/path1", nil)
	req2, _ := http.NewRequest("GET", "http://example.com/path2", nil)

	key1, req1 := transport.getCacheKey(req1)
	key2, req2 := transport.getCacheKey(req2)

	if key1 == key2 {
		t.Error("expected different cache keys for different URLs")
	}

	// Verify each request has its own key in context
	ctx1Key := getCacheKeyFromContext(req1)
	ctx2Key := getCacheKeyFromContext(req2)

	if ctx1Key != key1 {
		t.Errorf("req1 context key mismatch: %q != %q", ctx1Key, key1)
	}
	if ctx2Key != key2 {
		t.Errorf("req2 context key mismatch: %q != %q", ctx2Key, key2)
	}
}

// TestGetCacheKeyWithExistingContext verifies that getCacheKey preserves
// existing context values.
func TestGetCacheKeyWithExistingContext(t *testing.T) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache)

	type testKey struct{}
	existingValue := "existing"

	req, _ := http.NewRequest("GET", "http://example.com/test", nil)
	ctx := context.WithValue(req.Context(), testKey{}, existingValue)
	req = req.WithContext(ctx)

	// Get cache key
	_, req = transport.getCacheKey(req)

	// Verify existing context value is preserved
	val := req.Context().Value(testKey{})
	if val != existingValue {
		t.Errorf("expected existing context value preserved, got %v", val)
	}

	// Verify cache key was added
	cacheKey := getCacheKeyFromContext(req)
	if cacheKey == "" {
		t.Error("expected cache key in context")
	}
}
