//nolint:goconst // Test file with acceptable string duplication for readability
package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAuthorizationCaching tests RFC 9111 Section 3.5 Authorization caching rules
// using table-driven tests to reduce code duplication
func TestAuthorizationCaching(t *testing.T) {
	tests := []struct {
		name          string
		isPublicCache bool
		authHeader    string
		cacheControl  string
		shouldCache   bool
		description   string
	}{
		{
			name:          "Private cache with auth",
			isPublicCache: false,
			authHeader:    "Bearer token1",
			cacheControl:  "max-age=3600",
			shouldCache:   true,
			description:   "Private caches can cache authenticated responses",
		},
		{
			name:          "Shared cache no directive",
			isPublicCache: true,
			authHeader:    "Bearer token1",
			cacheControl:  "max-age=3600",
			shouldCache:   false,
			description:   "Shared caches must not cache without public/must-revalidate/s-maxage",
		},
		{
			name:          "Shared cache with public",
			isPublicCache: true,
			authHeader:    "Bearer token1",
			cacheControl:  "public, max-age=3600",
			shouldCache:   true,
			description:   "Shared caches can cache with public directive",
		},
		{
			name:          "Shared cache with must-revalidate",
			isPublicCache: true,
			authHeader:    "Bearer token1",
			cacheControl:  "must-revalidate, max-age=3600",
			shouldCache:   true,
			description:   "Shared caches can cache with must-revalidate",
		},
		{
			name:          "Shared cache with s-maxage",
			isPublicCache: true,
			authHeader:    "Bearer token1",
			cacheControl:  "s-maxage=3600, max-age=1800",
			shouldCache:   true,
			description:   "Shared caches can cache with s-maxage",
		},
		{
			name:          "Shared cache with multiple directives",
			isPublicCache: true,
			authHeader:    "Bearer token1",
			cacheControl:  "public, must-revalidate, max-age=3600",
			shouldCache:   true,
			description:   "Multiple directives allow shared cache",
		},
		{
			name:          "No auth header",
			isPublicCache: true,
			authHeader:    "",
			cacheControl:  "max-age=3600",
			shouldCache:   true,
			description:   "Responses without Authorization header are cacheable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetTest()
			requestCount := 0

			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				auth := r.Header.Get("Authorization")
				if tt.authHeader != "" && auth == "" {
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte("Unauthorized"))
					return
				}
				w.Header().Set("Cache-Control", tt.cacheControl)
				w.Write([]byte("Response for: " + auth))
			}))
			defer testServer.Close()

			// Create transport with specified cache type
			tp := newMockCacheTransport()
			tp.MarkCachedResponses = true
			tp.IsPublicCache = tt.isPublicCache
			client := tp.Client()

			// First request
			req1, err := http.NewRequest("GET", testServer.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tt.authHeader != "" {
				req1.Header.Set("Authorization", tt.authHeader)
			}

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

			// Second identical request
			req2, err := http.NewRequest("GET", testServer.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tt.authHeader != "" {
				req2.Header.Set("Authorization", tt.authHeader)
			}

			resp2, err := client.Do(req2)
			if err != nil {
				t.Fatal(err)
			}
			if resp2.StatusCode != http.StatusOK {
				t.Fatalf("Expected status 200, got %d", resp2.StatusCode)
			}
			io.ReadAll(resp2.Body)
			resp2.Body.Close()

			// Verify caching behavior
			expectedRequests := 1
			if !tt.shouldCache {
				expectedRequests = 2
			}

			if requestCount != expectedRequests {
				t.Fatalf("%s: Expected %d requests, got %d", tt.description, expectedRequests, requestCount)
			}

			// Verify cache header
			isCached := resp2.Header.Get(XFromCache) == "1"
			if tt.shouldCache && !isCached {
				t.Fatalf("%s: Expected response to be cached", tt.description)
			}
			if !tt.shouldCache && isCached {
				t.Fatalf("%s: Expected response NOT to be cached", tt.description)
			}
		})
	}
}

// TestAuthorizationCacheKeyHeaders tests Authorization caching with cache key headers
// This is a specialized test that combines Authorization with cache key customization
func TestAuthorizationCacheKeyHeaders(t *testing.T) {
	resetTest()
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		auth := r.Header.Get("Authorization")
		lang := r.Header.Get("Accept-Language")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write([]byte("Response for auth:" + auth + " lang:" + lang))
	}))
	defer testServer.Close()

	// Create shared cache with cache key headers
	cache := &mockCache{items: make(map[string][]byte)}
	tp := NewTransport(cache,
		WithPublicCache(true),
		WithCacheKeyHeaders([]string{"Accept-Language"}),
	)
	tp.MarkCachedResponses = true
	client := tp.Client()

	// First request with Authorization + Accept-Language
	req1, err := http.NewRequest("GET", testServer.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req1.Header.Set("Authorization", "Bearer token1")
	req1.Header.Set("Accept-Language", "en")

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

	// Second request same Authorization but different Accept-Language
	// Should NOT be cached (different cache key due to Accept-Language)
	req2, err := http.NewRequest("GET", testServer.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Authorization", "Bearer token1")
	req2.Header.Set("Accept-Language", "fr")

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
		t.Fatalf("Expected 2 requests (different Accept-Language), got %d", requestCount)
	}

	// Third request same as first (same Authorization + Accept-Language)
	// Should be cached
	req3, err := http.NewRequest("GET", testServer.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req3.Header.Set("Authorization", "Bearer token1")
	req3.Header.Set("Accept-Language", "en")

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
		t.Fatalf("Expected 2 requests (third should be cached), got %d", requestCount)
	}

	// Verify third response was cached
	if resp3.Header.Get(XFromCache) != "1" {
		t.Fatal("Expected third response to be cached")
	}
}

// TestAuthorizationEdgeCases tests edge cases for Authorization header handling
func TestAuthorizationEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		authHeader   string
		shouldAccept bool
	}{
		{
			name:         "Empty Authorization",
			authHeader:   "",
			shouldAccept: false,
		},
		{
			name:         "Whitespace only",
			authHeader:   "   ",
			shouldAccept: false,
		},
		{
			name:         "Valid Bearer token",
			authHeader:   "Bearer token123",
			shouldAccept: true,
		},
		{
			name:         "Valid Basic auth",
			authHeader:   "Basic dXNlcjpwYXNz",
			shouldAccept: true,
		},
		{
			name:         "Malformed auth",
			authHeader:   "InvalidFormat",
			shouldAccept: true, // Accept but treat as valid header
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetTest()
			requestCount := 0

			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				auth := r.Header.Get("Authorization")
				// Treat empty/whitespace as no auth
				if auth == "" || len([]rune(auth)) == 0 || isWhitespace(auth) {
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte("Unauthorized"))
					return
				}
				w.Header().Set("Cache-Control", "public, max-age=3600")
				w.Write([]byte("Authorized"))
			}))
			defer testServer.Close()

			tp := newMockCacheTransport()
			tp.IsPublicCache = true
			client := tp.Client()

			req, err := http.NewRequest("GET", testServer.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			io.ReadAll(resp.Body)

			if tt.shouldAccept && resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200 for valid auth, got %d", resp.StatusCode)
			}
			if !tt.shouldAccept && resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("Expected status 401 for invalid auth, got %d", resp.StatusCode)
			}
		})
	}
}

// Helper function to check if string is all whitespace
func isWhitespace(s string) bool {
	for _, c := range s {
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
	}
	return true
}
