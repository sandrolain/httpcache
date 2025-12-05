package httpcache

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	cacheControlPrivateMaxAge  = "private, max-age=3600"
	cacheControlPublicMaxAge   = "public, max-age=3600"
	cacheControlOnlyMaxAge     = "max-age=3600"
	cacheControlPrivateNoStore = "private, no-store"
	cacheControlPublicNoStore  = "public, no-store"
	cacheControlHeader         = "Cache-Control"
	pathPrivate                = "/private"
	pathPublic                 = "/public"
	pathPrivateMaxAge          = "/private-maxage"
	privateResponse1           = "private-response-1"
	privateResponse2           = "private-response-2"
	publicResponse1            = "public-response-1"
	response1                  = "response-1"
	response2                  = "response-2"
	responseFormat             = "response-%d"
	privateResponseFormat      = "private-response-%d"
	publicResponseFormat       = "public-response-%d"
	errExpectedFormat          = "Expected '%s', got '%s'"
)

// checkCacheBehavior verifies cache behavior based on expected caching state.
func checkCacheBehavior(t *testing.T, shouldCache bool, resp *http.Response, body string, requestCount int, description string) {
	t.Helper()
	if shouldCache {
		if resp.Header.Get(XFromCache) == "" {
			t.Errorf("%s: Expected response to be cached", description)
		}
		if body != response1 {
			t.Errorf("%s: Expected cached '%s', got '%s'", description, response1, body)
		}
		if requestCount != 1 {
			t.Errorf("%s: Expected 1 request (cached), got %d", description, requestCount)
		}
	} else {
		if resp.Header.Get(XFromCache) != "" {
			t.Errorf("%s: Expected response NOT to be cached", description)
		}
		if body != response2 {
			t.Errorf("%s: Expected fresh '%s', got '%s'", description, response2, body)
		}
		if requestCount != 2 {
			t.Errorf("%s: Expected 2 requests (not cached), got %d", description, requestCount)
		}
	}
}

// TestCacheControlPrivate verifies that responses with Cache-Control: private are cached.
// are properly cached in a private cache implementation.
// RFC 9111: Private caches (like httpcache) CAN cache responses with private directive.
func TestCacheControlPrivate(t *testing.T) {
	resetTest()

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Response with Cache-Control: private
		w.Header().Set(cacheControlHeader, cacheControlPrivateMaxAge)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, privateResponseFormat, requestCount)
	}))
	defer ts.Close()

	// First request - should be cached
	resp1, err := s.client.Get(ts.URL + pathPrivate)
	if err != nil {
		t.Fatal(err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if string(body1) != privateResponse1 {
		t.Errorf(errExpectedFormat, privateResponse1, string(body1))
	}

	if resp1.Header.Get(XFromCache) != "" {
		t.Error("First request should not be from cache")
	}

	// Second request - should be from cache (private caches CAN cache private responses)
	resp2, err := s.client.Get(ts.URL + pathPrivate)
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if string(body2) != privateResponse1 {
		t.Errorf(errExpectedFormat, privateResponse1, string(body2))
	}

	if resp2.Header.Get(XFromCache) == "" {
		t.Error("Second request should be from cache (private caches CAN cache private responses)")
	}

	if requestCount != 1 {
		t.Errorf("Expected 1 server request, got %d (private responses should be cached)", requestCount)
	}
}

// TestCacheControlPrivateWithNoStore verifies that no-store prevents caching
// even when the private directive is present.
func TestCacheControlPrivateWithNoStore(t *testing.T) {
	resetTest()

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// no-store should prevent caching even with private
		w.Header().Set("Cache-Control", cacheControlPrivateNoStore)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, privateResponseFormat, requestCount)
	}))
	defer ts.Close()

	// First request
	resp1, err := s.client.Get(ts.URL + pathPrivate)
	if err != nil {
		t.Fatal(err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if string(body1) != privateResponse1 {
		t.Errorf(errExpectedFormat, privateResponse1, string(body1))
	}

	// Second request - should NOT be from cache (no-store)
	resp2, err := s.client.Get(ts.URL + pathPrivate)
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if string(body2) != privateResponse2 {
		t.Errorf(errExpectedFormat, privateResponse2, string(body2))
	}

	if resp2.Header.Get(XFromCache) != "" {
		t.Error("Response with no-store should not be cached, even with private directive")
	}

	if requestCount != 2 {
		t.Errorf("Expected 2 server requests (not cached), got %d", requestCount)
	}
}

// TestCacheControlPublic verifies that Cache-Control: public responses
// are cached in private cache (public has no special effect in private caches).
func TestCacheControlPublic(t *testing.T) {
	resetTest()

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Response with Cache-Control: public
		w.Header().Set(cacheControlHeader, cacheControlPublicMaxAge)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, publicResponseFormat, requestCount)
	}))
	defer ts.Close()

	// First request
	resp1, err := s.client.Get(ts.URL + pathPublic)
	if err != nil {
		t.Fatal(err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if string(body1) != publicResponse1 {
		t.Errorf(errExpectedFormat, publicResponse1, string(body1))
	}

	// Second request - should be from cache (public doesn't prevent caching in private caches)
	resp2, err := s.client.Get(ts.URL + pathPublic)
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if string(body2) != publicResponse1 {
		t.Errorf(errExpectedFormat, publicResponse1, string(body2))
	}

	if resp2.Header.Get(XFromCache) == "" {
		t.Error("Second request should be from cache")
	}

	if requestCount != 1 {
		t.Errorf("Expected 1 server request, got %d", requestCount)
	}
}

// TestCacheControlPrivateMaxAge verifies that max-age is respected
// with Cache-Control: private directive.
func TestCacheControlPrivateMaxAge(t *testing.T) {
	resetTest()

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Response with private and max-age
		w.Header().Set(cacheControlHeader, "private, max-age=60")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, responseFormat, requestCount)
	}))
	defer ts.Close()

	// First request
	resp1, _ := s.client.Get(ts.URL + pathPrivateMaxAge)
	io.ReadAll(resp1.Body)
	resp1.Body.Close()

	// Second request - should be from cache (within max-age)
	resp2, _ := s.client.Get(ts.URL + pathPrivateMaxAge)
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if string(body2) != response1 {
		t.Errorf("Expected cached response, got '%s'", string(body2))
	}

	if resp2.Header.Get(XFromCache) == "" {
		t.Error("Response should be from cache")
	}

	if requestCount != 1 {
		t.Errorf("Expected 1 server request (cached), got %d", requestCount)
	}
}

// TestCacheControlPrivateVsPublicBehavior documents that in a private cache,
// both 'private' and 'public' directives are treated the same way.
func TestCacheControlPrivateVsPublicBehavior(t *testing.T) {
	resetTest()

	tests := []struct {
		name         string
		cacheControl string
		shouldCache  bool
		description  string
	}{
		{
			name:         "private directive",
			cacheControl: cacheControlPrivateMaxAge,
			shouldCache:  true,
			description:  "Private caches CAN cache responses with 'private'",
		},
		{
			name:         "public directive",
			cacheControl: cacheControlPublicMaxAge,
			shouldCache:  true,
			description:  "Private caches cache responses with 'public' (no special effect)",
		},
		{
			name:         "no directive (just max-age)",
			cacheControl: cacheControlOnlyMaxAge,
			shouldCache:  true,
			description:  "Responses with max-age are cached regardless of private/public",
		},
		{
			name:         "private with no-store",
			cacheControl: cacheControlPrivateNoStore,
			shouldCache:  false,
			description:  "no-store prevents caching even with private",
		},
		{
			name:         "public with no-store",
			cacheControl: cacheControlPublicNoStore,
			shouldCache:  false,
			description:  "no-store prevents caching even with public",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetTest()

			requestCount := 0
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				w.Header().Set(cacheControlHeader, tt.cacheControl)
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, responseFormat, requestCount)
			}))
			defer ts.Close()

			// First request
			resp1, _ := s.client.Get(ts.URL)
			io.ReadAll(resp1.Body)
			resp1.Body.Close()

			// Second request
			resp2, _ := s.client.Get(ts.URL)
			body2, _ := io.ReadAll(resp2.Body)
			resp2.Body.Close()

			checkCacheBehavior(t, tt.shouldCache, resp2, string(body2), requestCount, tt.description)
		})
	}
}

// TestIsPublicCachePrivateDirective verifies that when IsPublicCache=true,
// responses with Cache-Control: private are NOT cached.
func TestIsPublicCachePrivateDirective(t *testing.T) {
	resetTest()

	// Configure as public cache
	transport := newMockCacheTransport()
	transport.IsPublicCache = true
	client := transport.Client()

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Response with Cache-Control: private
		w.Header().Set(cacheControlHeader, cacheControlPrivateMaxAge)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, privateResponseFormat, requestCount)
	}))
	defer ts.Close()

	// First request
	resp1, err := client.Get(ts.URL + pathPrivate)
	if err != nil {
		t.Fatal(err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if string(body1) != privateResponse1 {
		t.Errorf(errExpectedFormat, privateResponse1, string(body1))
	}

	// Second request - should NOT be from cache (public cache can't cache private responses)
	resp2, err := client.Get(ts.URL + pathPrivate)
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if string(body2) != privateResponse2 {
		t.Errorf(errExpectedFormat, privateResponse2, string(body2))
	}

	if resp2.Header.Get(XFromCache) != "" {
		t.Error("Public cache should NOT cache responses with Cache-Control: private")
	}

	if requestCount != 2 {
		t.Errorf("Expected 2 server requests (not cached in public cache), got %d", requestCount)
	}
}

// TestIsPublicCachePublicDirective verifies that when IsPublicCache=true,
// responses with Cache-Control: public ARE cached.
func TestIsPublicCachePublicDirective(t *testing.T) {
	resetTest()

	// Configure as public cache
	transport := newMockCacheTransport()
	transport.IsPublicCache = true
	client := transport.Client()

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Response with Cache-Control: public
		w.Header().Set(cacheControlHeader, cacheControlPublicMaxAge)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, publicResponseFormat, requestCount)
	}))
	defer ts.Close()

	// First request
	resp1, err := client.Get(ts.URL + pathPublic)
	if err != nil {
		t.Fatal(err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if string(body1) != publicResponse1 {
		t.Errorf(errExpectedFormat, publicResponse1, string(body1))
	}

	// Second request - should be from cache
	resp2, err := client.Get(ts.URL + pathPublic)
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if string(body2) != publicResponse1 {
		t.Errorf(errExpectedFormat, publicResponse1, string(body2))
	}

	if resp2.Header.Get(XFromCache) == "" {
		t.Error("Public cache should cache responses with Cache-Control: public")
	}

	if requestCount != 1 {
		t.Errorf("Expected 1 server request (cached), got %d", requestCount)
	}
}

// TestIsPublicCacheComparison compares behavior between private and public cache modes.
func TestIsPublicCacheComparison(t *testing.T) {
	tests := []struct {
		name          string
		isPublicCache bool
		cacheControl  string
		shouldCache   bool
		description   string
	}{
		{
			name:          "private cache with private directive",
			isPublicCache: false,
			cacheControl:  cacheControlPrivateMaxAge,
			shouldCache:   true,
			description:   "Private caches CAN cache private responses",
		},
		{
			name:          "public cache with private directive",
			isPublicCache: true,
			cacheControl:  cacheControlPrivateMaxAge,
			shouldCache:   false,
			description:   "Public caches MUST NOT cache private responses",
		},
		{
			name:          "private cache with public directive",
			isPublicCache: false,
			cacheControl:  cacheControlPublicMaxAge,
			shouldCache:   true,
			description:   "Private caches can cache public responses",
		},
		{
			name:          "public cache with public directive",
			isPublicCache: true,
			cacheControl:  cacheControlPublicMaxAge,
			shouldCache:   true,
			description:   "Public caches can cache public responses",
		},
		{
			name:          "private cache with no directive",
			isPublicCache: false,
			cacheControl:  cacheControlOnlyMaxAge,
			shouldCache:   true,
			description:   "Private caches cache responses without private/public",
		},
		{
			name:          "public cache with no directive",
			isPublicCache: true,
			cacheControl:  cacheControlOnlyMaxAge,
			shouldCache:   true,
			description:   "Public caches cache responses without private/public",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup transport with specific cache mode
			transport := newMockCacheTransport()
			transport.IsPublicCache = tt.isPublicCache
			client := transport.Client()

			requestCount := 0
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				w.Header().Set(cacheControlHeader, tt.cacheControl)
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, responseFormat, requestCount)
			}))
			defer ts.Close()

			// First request
			resp1, _ := client.Get(ts.URL)
			io.ReadAll(resp1.Body)
			resp1.Body.Close()

			// Second request
			resp2, _ := client.Get(ts.URL)
			body2, _ := io.ReadAll(resp2.Body)
			resp2.Body.Close()

			verifyPublicCacheBehavior(t, tt.shouldCache, tt.description, resp2, string(body2), requestCount)
		})
	}
}

// verifyPublicCacheBehavior checks expected caching behavior for public cache tests.
func verifyPublicCacheBehavior(t *testing.T, shouldCache bool, description string, resp *http.Response, body string, requestCount int) {
	t.Helper()
	if shouldCache {
		if resp.Header.Get(XFromCache) == "" {
			t.Errorf("%s: Expected response to be cached", description)
		}
		if body != response1 {
			t.Errorf("%s: Expected cached '%s', got '%s'", description, response1, body)
		}
		if requestCount != 1 {
			t.Errorf("%s: Expected 1 request (cached), got %d", description, requestCount)
		}
	} else {
		if resp.Header.Get(XFromCache) != "" {
			t.Errorf("%s: Expected response NOT to be cached", description)
		}
		if body != response2 {
			t.Errorf("%s: Expected fresh '%s', got '%s'", description, response2, body)
		}
		if requestCount != 2 {
			t.Errorf("%s: Expected 2 requests (not cached), got %d", description, requestCount)
		}
	}
}
