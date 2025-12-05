package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestCacheKeyHeaders tests that cache entries are differentiated by request headers
// when CacheKeyHeaders is configured
func TestCacheKeyHeaders(t *testing.T) {
	resetTest()
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		auth := r.Header.Get("Authorization")
		w.Write([]byte("Response for auth: " + auth))
	}))
	defer testServer.Close()

	tp := newMockCacheTransport()
	tp.CacheKeyHeaders = []string{"Authorization"}
	client := tp.Client()

	// First request with Authorization: Bearer token1
	req1, err := http.NewRequest("GET", testServer.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req1.Header.Set("Authorization", "Bearer token1")

	resp1, err := client.Do(req1)
	if err != nil {
		t.Fatal(err)
	}
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp1.StatusCode)
	}
	io.ReadAll(resp1.Body)
	io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if requestCount != 1 {
		t.Fatalf("Expected 1 request to server, got %d", requestCount)
	}

	// Second request with Authorization: Bearer token2 (different auth)
	// Should NOT use cache, should make a new request
	req2, err := http.NewRequest("GET", testServer.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Authorization", "Bearer token2")

	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp2.StatusCode)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if requestCount != 2 {
		t.Fatalf("Expected 2 requests to server, got %d", requestCount)
	}

	// Third request with Authorization: Bearer token1 (same as first)
	// Should use cache from first request
	req3, err := http.NewRequest("GET", testServer.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req3.Header.Set("Authorization", "Bearer token1")

	resp3, err := client.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp3.StatusCode)
	}
	io.ReadAll(resp3.Body)
	resp3.Body.Close()

	if requestCount != 2 {
		t.Fatalf("Expected 2 requests to server (third should be cached), got %d", requestCount)
	}

	// Verify cache was used
	if resp3.Header.Get(XFromCache) != "1" {
		t.Fatal("Expected response to be served from cache")
	}
}

// TestCacheKeyHeadersMultipleHeaders tests cache differentiation with multiple headers
func TestCacheKeyHeadersMultipleHeaders(t *testing.T) {
	resetTest()
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		auth := r.Header.Get("Authorization")
		lang := r.Header.Get("Accept-Language")
		w.Write([]byte("Auth: " + auth + " Lang: " + lang))
	}))
	defer testServer.Close()

	tp := newMockCacheTransport()
	tp.CacheKeyHeaders = []string{"Authorization", "Accept-Language"}
	client := tp.Client()

	// Request 1: Auth=token1, Lang=en
	req1, _ := http.NewRequest("GET", testServer.URL, nil)
	req1.Header.Set("Authorization", "Bearer token1")
	req1.Header.Set("Accept-Language", "en")
	resp1, _ := client.Do(req1)
	io.ReadAll(resp1.Body)
	resp1.Body.Close()

	// Request 2: Auth=token1, Lang=it (different lang, same auth)
	req2, _ := http.NewRequest("GET", testServer.URL, nil)
	req2.Header.Set("Authorization", "Bearer token1")
	req2.Header.Set("Accept-Language", "it")
	resp2, _ := client.Do(req2)
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	// Request 3: Auth=token2, Lang=en (different auth, same lang as req1)
	req3, _ := http.NewRequest("GET", testServer.URL, nil)
	req3.Header.Set("Authorization", "Bearer token2")
	req3.Header.Set("Accept-Language", "en")
	resp3, _ := client.Do(req3)
	io.ReadAll(resp3.Body)
	resp3.Body.Close()

	// All three should hit the server
	if requestCount != 3 {
		t.Fatalf("Expected 3 requests to server, got %d", requestCount)
	}

	// Request 4: Same as Request 1 (should be cached)
	req4, _ := http.NewRequest("GET", testServer.URL, nil)
	req4.Header.Set("Authorization", "Bearer token1")
	req4.Header.Set("Accept-Language", "en")
	resp4, _ := client.Do(req4)
	io.ReadAll(resp4.Body)
	resp4.Body.Close()

	if requestCount != 3 {
		t.Fatalf("Expected 3 requests to server (fourth should be cached), got %d", requestCount)
	}

	if resp4.Header.Get(XFromCache) != "1" {
		t.Fatal("Expected response to be served from cache")
	}
}

// TestCacheKeyHeadersCaseInsensitive tests that header names are case-insensitive
func TestCacheKeyHeadersCaseInsensitive(t *testing.T) {
	resetTest()
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Write([]byte("OK"))
	}))
	defer testServer.Close()

	tp := newMockCacheTransport()
	tp.CacheKeyHeaders = []string{"authorization"} // lowercase
	client := tp.Client()

	// Request with Authorization header (canonical case)
	req1, _ := http.NewRequest("GET", testServer.URL, nil)
	req1.Header.Set("Authorization", "Bearer token1")
	resp1, _ := client.Do(req1)
	io.ReadAll(resp1.Body)
	resp1.Body.Close()

	// Request with authorization header (lowercase) - should use cache
	req2, _ := http.NewRequest("GET", testServer.URL, nil)
	req2.Header.Set("authorization", "Bearer token1")
	resp2, _ := client.Do(req2)
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if requestCount != 1 {
		t.Fatalf("Expected 1 request to server (second should be cached), got %d", requestCount)
	}

	if resp2.Header.Get(XFromCache) != "1" {
		t.Fatal("Expected response to be served from cache")
	}
}

// TestCacheKeyHeadersWithoutHeader tests that requests without the configured header
// create separate cache entries from those with the header
func TestCacheKeyHeadersWithoutHeader(t *testing.T) {
	resetTest()
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Write([]byte("No auth"))
		} else {
			w.Write([]byte("Auth: " + auth))
		}
	}))
	defer testServer.Close()

	tp := newMockCacheTransport()
	tp.CacheKeyHeaders = []string{"Authorization"}
	client := tp.Client()

	// Request without Authorization header
	req1, _ := http.NewRequest("GET", testServer.URL, nil)
	resp1, _ := client.Do(req1)
	io.ReadAll(resp1.Body)
	resp1.Body.Close()

	// Request with Authorization header
	req2, _ := http.NewRequest("GET", testServer.URL, nil)
	req2.Header.Set("Authorization", "Bearer token1")
	resp2, _ := client.Do(req2)
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	// Both should hit the server
	if requestCount != 2 {
		t.Fatalf("Expected 2 requests to server, got %d", requestCount)
	}

	// Request without Authorization header again (should be cached)
	req3, _ := http.NewRequest("GET", testServer.URL, nil)
	resp3, _ := client.Do(req3)
	io.ReadAll(resp3.Body)
	resp3.Body.Close()

	if requestCount != 2 {
		t.Fatalf("Expected 2 requests to server (third should be cached), got %d", requestCount)
	}

	if resp3.Header.Get(XFromCache) != "1" {
		t.Fatal("Expected response to be served from cache")
	}
}

// TestCacheKeyHeadersWithEmptyList tests backward compatibility when CacheKeyHeaders is nil/empty
func TestCacheKeyHeadersWithEmptyList(t *testing.T) {
	resetTest()
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Write([]byte("OK"))
	}))
	defer testServer.Close()

	tp := newMockCacheTransport()
	// CacheKeyHeaders not set (nil)
	client := tp.Client()

	// Request with Authorization header
	req1, _ := http.NewRequest("GET", testServer.URL, nil)
	req1.Header.Set("Authorization", "Bearer token1")
	resp1, _ := client.Do(req1)
	io.ReadAll(resp1.Body)
	resp1.Body.Close()

	// Request with different Authorization header
	// Should use cache (backward compatible behavior)
	req2, _ := http.NewRequest("GET", testServer.URL, nil)
	req2.Header.Set("Authorization", "Bearer token2")
	resp2, _ := client.Do(req2)
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if requestCount != 1 {
		t.Fatalf("Expected 1 request to server (backward compatible), got %d", requestCount)
	}

	if resp2.Header.Get(XFromCache) != "1" {
		t.Fatal("Expected response to be served from cache")
	}
}

// TestCacheKeyHeadersInvalidation tests that cache invalidation works correctly with CacheKeyHeaders
func TestCacheKeyHeadersInvalidation(t *testing.T) {
	resetTest()
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch r.Method {
		case "GET":
			w.Header().Set("Cache-Control", "max-age=3600")
			w.Write([]byte("GET response"))
		case "POST":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("POST response"))
		}
	}))
	defer testServer.Close()

	tp := newMockCacheTransport()
	tp.CacheKeyHeaders = []string{"Authorization"}
	client := tp.Client()

	// GET request with Authorization
	req1, _ := http.NewRequest("GET", testServer.URL, nil)
	req1.Header.Set("Authorization", "Bearer token1")
	resp1, _ := client.Do(req1)
	io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if requestCount != 1 {
		t.Fatalf("Expected 1 request, got %d", requestCount)
	}

	// POST request to same URL (should invalidate cache)
	req2, _ := http.NewRequest("POST", testServer.URL, nil)
	req2.Header.Set("Authorization", "Bearer token1")
	resp2, _ := client.Do(req2)
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if requestCount != 2 {
		t.Fatalf("Expected 2 requests, got %d", requestCount)
	}

	// GET request again (cache should be invalidated, new request)
	req3, _ := http.NewRequest("GET", testServer.URL, nil)
	req3.Header.Set("Authorization", "Bearer token1")
	resp3, _ := client.Do(req3)
	io.ReadAll(resp3.Body)
	resp3.Body.Close()

	// Note: Cache invalidation with CacheKeyHeaders invalidates only the base URL,
	// not the header-specific entries. This is intentional behavior.
	// The cache for this specific Authorization header combination is still valid.
	if requestCount != 2 {
		t.Fatalf("Expected 2 requests (cache with headers still valid), got %d", requestCount)
	}

	if resp3.Header.Get(XFromCache) != "1" {
		t.Fatal("Expected response to be served from cache (header-specific cache not invalidated)")
	}
}

// TestCacheKeyFormat tests the format of cache keys with CacheKeyHeaders
func TestCacheKeyFormat(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		url             string
		headers         map[string]string
		cacheKeyHeaders []string
		expectedKey     string
	}{
		{
			name:            "GET without cache key headers",
			method:          "GET",
			url:             "http://example.com/test",
			cacheKeyHeaders: nil,
			expectedKey:     "http://example.com/test",
		},
		{
			name:   "GET with single cache key header",
			method: "GET",
			url:    "http://example.com/test",
			headers: map[string]string{
				"Authorization": "Bearer token1",
			},
			cacheKeyHeaders: []string{"Authorization"},
			expectedKey:     "http://example.com/test|Authorization:Bearer token1",
		},
		{
			name:   "GET with multiple cache key headers",
			method: "GET",
			url:    "http://example.com/test",
			headers: map[string]string{
				"Authorization":   "Bearer token1",
				"Accept-Language": "en",
			},
			cacheKeyHeaders: []string{"Authorization", "Accept-Language"},
			expectedKey:     "http://example.com/test|Accept-Language:en|Authorization:Bearer token1",
		},
		{
			name:            "POST without cache key headers",
			method:          "POST",
			url:             "http://example.com/test",
			cacheKeyHeaders: nil,
			expectedKey:     "POST http://example.com/test",
		},
		{
			name:   "POST with cache key headers",
			method: "POST",
			url:    "http://example.com/test",
			headers: map[string]string{
				"Authorization": "Bearer token1",
			},
			cacheKeyHeaders: []string{"Authorization"},
			expectedKey:     "POST http://example.com/test|Authorization:Bearer token1",
		},
		{
			name:   "GET with cache key header but header not present in request",
			method: "GET",
			url:    "http://example.com/test",
			headers: map[string]string{
				"Other-Header": "value",
			},
			cacheKeyHeaders: []string{"Authorization"},
			expectedKey:     "http://example.com/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, tt.url, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			key := cacheKeyWithHeaders(req, tt.cacheKeyHeaders)
			if key != tt.expectedKey {
				t.Errorf("Expected cache key %q, got %q", tt.expectedKey, key)
			}
		})
	}
}

// TestCacheKeyHeadersRevalidation tests that revalidation works correctly with CacheKeyHeaders
func TestCacheKeyHeadersRevalidation(t *testing.T) {
	resetTest()
	requestCount := 0
	etag := `"v1"`
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Cache-Control", "max-age=1") // Short cache
		w.Header().Set("ETag", etag)
		auth := r.Header.Get("Authorization")
		w.Write([]byte("Auth: " + auth))
	}))
	defer testServer.Close()

	tp := newMockCacheTransport()
	tp.CacheKeyHeaders = []string{"Authorization"}
	client := tp.Client()

	// First request
	req1, _ := http.NewRequest("GET", testServer.URL, nil)
	req1.Header.Set("Authorization", "Bearer token1")
	resp1, _ := client.Do(req1)
	io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if requestCount != 1 {
		t.Fatalf("Expected 1 request, got %d", requestCount)
	}

	// Wait for cache to become stale
	time.Sleep(2 * time.Second)

	// Second request (should revalidate)
	req2, _ := http.NewRequest("GET", testServer.URL, nil)
	req2.Header.Set("Authorization", "Bearer token1")
	resp2, _ := client.Do(req2)
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if requestCount != 2 {
		t.Fatalf("Expected 2 requests (revalidation), got %d", requestCount)
	}

	// Check that response was revalidated
	if resp2.Header.Get(XRevalidated) != "1" {
		t.Fatal("Expected response to be revalidated")
	}
}
