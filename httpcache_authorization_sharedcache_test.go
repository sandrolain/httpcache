//nolint:goconst // Test file with acceptable string duplication for readability
package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAuthorizationPrivateCache tests that private caches CAN cache Authorization responses
// RFC 9111 Section 3.5: Private caches (browsers, API clients) can cache authenticated responses
func TestAuthorizationPrivateCache(t *testing.T) {
	resetTest()
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}
		// Response without special Cache-Control directives
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Write([]byte("Private response for: " + auth))
	}))
	defer testServer.Close()

	// Create private cache (default)
	tp := NewMemoryCacheTransport()
	tp.MarkCachedResponses = true
	// IsPublicCache is false by default
	client := tp.Client()

	// First request with Authorization
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
	resp1.Body.Close()

	if requestCount != 1 {
		t.Fatalf("Expected 1 request, got %d", requestCount)
	}

	// Second request with same Authorization (should be cached in private cache)
	req2, err := http.NewRequest("GET", testServer.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Authorization", "Bearer token1")

	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp2.StatusCode)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if requestCount != 1 {
		t.Fatalf("Expected 1 request (second should be cached), got %d", requestCount)
	}

	// Verify response was served from cache
	if resp2.Header.Get(XFromCache) != "1" {
		t.Fatal("Expected response to be served from cache in private cache mode")
	}
}

// TestAuthorizationSharedCacheNoDirective tests that shared caches MUST NOT cache
// Authorization responses without public/must-revalidate/s-maxage directives
// RFC 9111 Section 3.5
func TestAuthorizationSharedCacheNoDirective(t *testing.T) {
	resetTest()
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}
		// Response WITHOUT public/must-revalidate/s-maxage
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Write([]byte("Response for: " + auth))
	}))
	defer testServer.Close()

	// Create shared/public cache
	tp := NewMemoryCacheTransport()
	tp.MarkCachedResponses = true
	tp.IsPublicCache = true // Enable shared cache mode
	client := tp.Client()

	// First request with Authorization
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
	resp1.Body.Close()

	if requestCount != 1 {
		t.Fatalf("Expected 1 request, got %d", requestCount)
	}

	// Second request with same Authorization
	// Should NOT be cached (shared cache + no public/must-revalidate/s-maxage)
	req2, err := http.NewRequest("GET", testServer.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Authorization", "Bearer token1")

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
		t.Fatalf("Expected 2 requests (should NOT be cached), got %d", requestCount)
	}

	// Verify response was NOT served from cache
	if resp2.Header.Get(XFromCache) == "1" {
		t.Fatal("Expected response NOT to be cached in shared cache without public/must-revalidate/s-maxage")
	}
}

// TestAuthorizationSharedCacheWithPublic tests that shared caches CAN cache
// Authorization responses when Cache-Control: public is present
// RFC 9111 Section 3.5
func TestAuthorizationSharedCacheWithPublic(t *testing.T) {
	resetTest()
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}
		// Response WITH public directive
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write([]byte("Public response for: " + auth))
	}))
	defer testServer.Close()

	// Create shared/public cache
	tp := NewMemoryCacheTransport()
	tp.MarkCachedResponses = true
	tp.IsPublicCache = true // Enable shared cache mode
	client := tp.Client()

	// First request with Authorization
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
	resp1.Body.Close()

	if requestCount != 1 {
		t.Fatalf("Expected 1 request, got %d", requestCount)
	}

	// Second request with same Authorization
	// Should be cached (shared cache + public directive)
	req2, err := http.NewRequest("GET", testServer.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Authorization", "Bearer token1")

	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp2.StatusCode)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if requestCount != 1 {
		t.Fatalf("Expected 1 request (second should be cached with public), got %d", requestCount)
	}

	// Verify response was served from cache
	if resp2.Header.Get(XFromCache) != "1" {
		t.Fatal("Expected response to be cached in shared cache with public directive")
	}
}

// TestAuthorizationSharedCacheWithMustRevalidate tests that shared caches CAN cache
// Authorization responses when Cache-Control: must-revalidate is present
// RFC 9111 Section 3.5
func TestAuthorizationSharedCacheWithMustRevalidate(t *testing.T) {
	resetTest()
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}
		// Response WITH must-revalidate directive
		w.Header().Set("Cache-Control", "must-revalidate, max-age=3600")
		w.Write([]byte("Must-revalidate response for: " + auth))
	}))
	defer testServer.Close()

	// Create shared/public cache
	tp := NewMemoryCacheTransport()
	tp.MarkCachedResponses = true
	tp.IsPublicCache = true // Enable shared cache mode
	client := tp.Client()

	// First request with Authorization
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
	resp1.Body.Close()

	if requestCount != 1 {
		t.Fatalf("Expected 1 request, got %d", requestCount)
	}

	// Second request with same Authorization
	// Should be cached (shared cache + must-revalidate directive)
	req2, err := http.NewRequest("GET", testServer.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Authorization", "Bearer token1")

	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp2.StatusCode)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if requestCount != 1 {
		t.Fatalf("Expected 1 request (second should be cached with must-revalidate), got %d", requestCount)
	}

	// Verify response was served from cache
	if resp2.Header.Get(XFromCache) != "1" {
		t.Fatal("Expected response to be cached in shared cache with must-revalidate directive")
	}
}

// TestAuthorizationSharedCacheWithSMaxAge tests that shared caches CAN cache
// Authorization responses when Cache-Control: s-maxage is present
// RFC 9111 Section 3.5
func TestAuthorizationSharedCacheWithSMaxAge(t *testing.T) {
	resetTest()
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}
		// Response WITH s-maxage directive
		w.Header().Set("Cache-Control", "s-maxage=3600, max-age=1800")
		w.Write([]byte("S-maxage response for: " + auth))
	}))
	defer testServer.Close()

	// Create shared/public cache
	tp := NewMemoryCacheTransport()
	tp.MarkCachedResponses = true
	tp.IsPublicCache = true // Enable shared cache mode
	client := tp.Client()

	// First request with Authorization
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
	resp1.Body.Close()

	if requestCount != 1 {
		t.Fatalf("Expected 1 request, got %d", requestCount)
	}

	// Second request with same Authorization
	// Should be cached (shared cache + s-maxage directive)
	req2, err := http.NewRequest("GET", testServer.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Authorization", "Bearer token1")

	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp2.StatusCode)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if requestCount != 1 {
		t.Fatalf("Expected 1 request (second should be cached with s-maxage), got %d", requestCount)
	}

	// Verify response was served from cache
	if resp2.Header.Get(XFromCache) != "1" {
		t.Fatal("Expected response to be cached in shared cache with s-maxage directive")
	}
}

// TestAuthorizationSharedCacheMultipleDirectives tests combinations of directives
func TestAuthorizationSharedCacheMultipleDirectives(t *testing.T) {
	tests := []struct {
		name          string
		cacheControl  string
		shouldCache   bool
		isPublicCache bool
	}{
		{
			name:          "private_cache_no_directive",
			cacheControl:  "max-age=3600",
			shouldCache:   true,
			isPublicCache: false,
		},
		{
			name:          "shared_cache_no_directive",
			cacheControl:  "max-age=3600",
			shouldCache:   false,
			isPublicCache: true,
		},
		{
			name:          "shared_cache_with_public",
			cacheControl:  "public, max-age=3600",
			shouldCache:   true,
			isPublicCache: true,
		},
		{
			name:          "shared_cache_with_must_revalidate",
			cacheControl:  "must-revalidate, max-age=3600",
			shouldCache:   true,
			isPublicCache: true,
		},
		{
			name:          "shared_cache_with_s_maxage",
			cacheControl:  "s-maxage=3600, max-age=1800",
			shouldCache:   true,
			isPublicCache: true,
		},
		{
			name:          "shared_cache_public_and_must_revalidate",
			cacheControl:  "public, must-revalidate, max-age=3600",
			shouldCache:   true,
			isPublicCache: true,
		},
		{
			name:          "shared_cache_all_three_directives",
			cacheControl:  "public, must-revalidate, s-maxage=3600, max-age=1800",
			shouldCache:   true,
			isPublicCache: true,
		},
		{
			name:          "private_cache_with_public",
			cacheControl:  "public, max-age=3600",
			shouldCache:   true,
			isPublicCache: false,
		},
		{
			name:          "shared_cache_with_no_store",
			cacheControl:  "no-store",
			shouldCache:   false,
			isPublicCache: true,
		},
		{
			name:          "shared_cache_public_with_no_store",
			cacheControl:  "public, no-store",
			shouldCache:   false,
			isPublicCache: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetTest()
			requestCount := 0
			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				w.Header().Set("Cache-Control", tt.cacheControl)
				w.Write([]byte("Response"))
			}))
			defer testServer.Close()

			tp := NewMemoryCacheTransport()
			tp.MarkCachedResponses = true
			tp.IsPublicCache = tt.isPublicCache
			client := tp.Client()

			// First request with Authorization
			req1, _ := http.NewRequest("GET", testServer.URL, nil)
			req1.Header.Set("Authorization", "Bearer token1")
			resp1, _ := client.Do(req1)
			io.ReadAll(resp1.Body)
			resp1.Body.Close()

			// Second request with same Authorization
			req2, _ := http.NewRequest("GET", testServer.URL, nil)
			req2.Header.Set("Authorization", "Bearer token1")
			resp2, _ := client.Do(req2)
			io.ReadAll(resp2.Body)
			resp2.Body.Close()

			expectedRequests := 2
			if tt.shouldCache {
				expectedRequests = 1
			}

			if requestCount != expectedRequests {
				t.Errorf("Expected %d requests, got %d (shouldCache=%v)", expectedRequests, requestCount, tt.shouldCache)
			}

			cacheHeaderExpected := "1"
			if !tt.shouldCache {
				cacheHeaderExpected = ""
			}

			if resp2.Header.Get(XFromCache) != cacheHeaderExpected {
				t.Errorf("Expected X-From-Cache=%q, got %q", cacheHeaderExpected, resp2.Header.Get(XFromCache))
			}
		})
	}
}

// TestAuthorizationWithNoAuthHeader tests that responses without Authorization header
// are cached normally in both private and shared caches
func TestAuthorizationWithNoAuthHeader(t *testing.T) {
	tests := []struct {
		name          string
		isPublicCache bool
	}{
		{
			name:          "private_cache",
			isPublicCache: false,
		},
		{
			name:          "shared_cache",
			isPublicCache: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetTest()
			requestCount := 0
			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				w.Header().Set("Cache-Control", "max-age=3600")
				w.Write([]byte("Public response"))
			}))
			defer testServer.Close()

			tp := NewMemoryCacheTransport()
			tp.MarkCachedResponses = true
			tp.IsPublicCache = tt.isPublicCache
			client := tp.Client()

			// First request without Authorization header
			req1, _ := http.NewRequest("GET", testServer.URL, nil)
			resp1, _ := client.Do(req1)
			io.ReadAll(resp1.Body)
			resp1.Body.Close()

			// Second request without Authorization header
			// Should be cached in both private and shared caches
			req2, _ := http.NewRequest("GET", testServer.URL, nil)
			resp2, _ := client.Do(req2)
			io.ReadAll(resp2.Body)
			resp2.Body.Close()

			if requestCount != 1 {
				t.Errorf("Expected 1 request (should be cached), got %d", requestCount)
			}

			if resp2.Header.Get(XFromCache) != "1" {
				t.Error("Expected response to be cached when no Authorization header present")
			}
		})
	}
}

// TestAuthorizationSharedCacheWithCacheKeyHeaders tests that Authorization header
// in CacheKeyHeaders creates separate cache entries per token in shared cache
func TestAuthorizationSharedCacheWithCacheKeyHeaders(t *testing.T) {
	resetTest()
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		auth := r.Header.Get("Authorization")
		// Response with public directive to allow shared cache
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write([]byte("Response for: " + auth))
	}))
	defer testServer.Close()

	// Create shared cache with CacheKeyHeaders
	tp := NewMemoryCacheTransport()
	tp.MarkCachedResponses = true
	tp.IsPublicCache = true // Shared cache
	tp.CacheKeyHeaders = []string{"Authorization"}
	client := tp.Client()

	// Request 1: token1
	req1, _ := http.NewRequest("GET", testServer.URL, nil)
	req1.Header.Set("Authorization", "Bearer token1")
	resp1, _ := client.Do(req1)
	io.ReadAll(resp1.Body)
	resp1.Body.Close()

	// Request 2: token2 (different token, should NOT use cache)
	req2, _ := http.NewRequest("GET", testServer.URL, nil)
	req2.Header.Set("Authorization", "Bearer token2")
	resp2, _ := client.Do(req2)
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	// Request 3: token1 (same as req1, should use cache)
	req3, _ := http.NewRequest("GET", testServer.URL, nil)
	req3.Header.Set("Authorization", "Bearer token1")
	resp3, _ := client.Do(req3)
	io.ReadAll(resp3.Body)
	resp3.Body.Close()

	if requestCount != 2 {
		t.Fatalf("Expected 2 requests (req1 and req2, req3 cached), got %d", requestCount)
	}

	// Verify req3 was served from cache
	if resp3.Header.Get(XFromCache) != "1" {
		t.Fatal("Expected response to be cached with CacheKeyHeaders in shared cache")
	}
}
