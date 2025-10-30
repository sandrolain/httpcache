package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// drainAndClose reads the body completely and closes it to trigger caching
func drainAndClose(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

// TestMustUnderstandKnownStatus tests that responses with must-understand and known status codes are cached
func TestMustUnderstandKnownStatus(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		shouldCache bool
	}{
		{"200 OK", 200, true},
		{"203 Non-Authoritative", 203, true},
		{"204 No Content", 204, true},
		{"206 Partial Content", 206, true},
		{"300 Multiple Choices", 300, true},
		{"301 Moved Permanently", 301, true},
		{"404 Not Found", 404, true},
		{"405 Method Not Allowed", 405, true},
		{"410 Gone", 410, true},
		{"414 URI Too Long", 414, true},
		{"501 Not Implemented", 501, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetTest()
			counter := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				counter++
				w.Header().Set("Cache-Control", "must-understand, max-age=3600")
				w.WriteHeader(tt.statusCode)
			}))
			defer s.Close()

			tp := NewMemoryCacheTransport()
			client := &http.Client{Transport: tp}

			// First request - should be cached
			resp, err := client.Get(s.URL)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tt.statusCode {
				t.Fatalf("expected status %d, got %d", tt.statusCode, resp.StatusCode)
			}
			drainAndClose(resp)

			if counter != 1 {
				t.Fatalf("expected 1 request, got %d", counter)
			}

			// Second request - should be served from cache if shouldCache is true
			resp, err = client.Get(s.URL)
			if err != nil {
				t.Fatal(err)
			}
			drainAndClose(resp)

			if tt.shouldCache {
				if counter != 1 {
					t.Fatalf("expected response to be cached (1 request total), got %d requests", counter)
				}
				if resp.Header.Get(XFromCache) != "1" {
					t.Fatal("expected response from cache")
				}
			}
		})
	}
}

// TestMustUnderstandUnknownStatus tests that responses with must-understand and unknown status codes are NOT cached
func TestMustUnderstandUnknownStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"201 Created", 201},
		{"202 Accepted", 202},
		{"302 Found", 302},
		{"400 Bad Request", 400},
		{"403 Forbidden", 403},
		{"418 I'm a teapot", 418},
		{"500 Internal Server Error", 500},
		{"502 Bad Gateway", 502},
		{"503 Service Unavailable", 503},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetTest()
			counter := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				counter++
				w.Header().Set("Cache-Control", "must-understand, max-age=3600")
				w.WriteHeader(tt.statusCode)
			}))
			defer s.Close()

			tp := NewMemoryCacheTransport()
			client := &http.Client{Transport: tp}

			// First request
			resp, err := client.Get(s.URL)
			if err != nil {
				t.Fatal(err)
			}
			drainAndClose(resp)

			if counter != 1 {
				t.Fatalf("expected 1 request, got %d", counter)
			}

			// Second request - should NOT be cached (unknown status code with must-understand)
			resp, err = client.Get(s.URL)
			if err != nil {
				t.Fatal(err)
			}
			drainAndClose(resp)

			if counter != 2 {
				t.Fatalf("expected response NOT to be cached (2 requests total), got %d requests", counter)
			}

			if resp.Header.Get(XFromCache) == "1" {
				t.Fatal("expected response NOT from cache")
			}
		})
	}
}

// TestMustUnderstandOverridesNoStore tests that must-understand overrides no-store for known status codes
func TestMustUnderstandOverridesNoStore(t *testing.T) {
	resetTest()
	counter := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		// Both must-understand and no-store present
		// For known status code (200), must-understand should allow caching
		w.Header().Set("Cache-Control", "must-understand, no-store, max-age=3600")
	}))
	defer s.Close()

	tp := NewMemoryCacheTransport()
	client := &http.Client{Transport: tp}

	// First request
	resp, err := client.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	drainAndClose(resp)

	if counter != 1 {
		t.Fatalf("expected 1 request, got %d", counter)
	}

	// Second request - should be served from cache (must-understand overrides no-store)
	resp, err = client.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	drainAndClose(resp)

	if counter != 1 {
		t.Fatalf("expected response to be cached (must-understand overrides no-store), got %d requests", counter)
	}

	if resp.Header.Get(XFromCache) != "1" {
		t.Fatal("expected response from cache")
	}
}

// TestMustUnderstandWithNoStoreUnknownStatus tests that no-store is NOT overridden for unknown status codes
func TestMustUnderstandWithNoStoreUnknownStatus(t *testing.T) {
	resetTest()
	counter := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		w.Header().Set("Cache-Control", "must-understand, no-store, max-age=3600")
		w.WriteHeader(418) // Unknown status code
	}))
	defer s.Close()

	tp := NewMemoryCacheTransport()
	client := &http.Client{Transport: tp}

	// First request
	resp, err := client.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	drainAndClose(resp)

	// Second request - should NOT be cached (unknown status + must-understand)
	resp, err = client.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	drainAndClose(resp)

	if counter != 2 {
		t.Fatalf("expected 2 requests (unknown status code should not be cached), got %d", counter)
	}
}

// TestMustUnderstandWithPrivate tests that must-understand works with private directive
func TestMustUnderstandWithPrivate(t *testing.T) {
	resetTest()
	counter := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		w.Header().Set("Cache-Control", "must-understand, private, max-age=3600")
	}))
	defer s.Close()

	// Test with private cache (default)
	tp := NewMemoryCacheTransport()
	tp.IsPublicCache = false
	client := &http.Client{Transport: tp}

	// First request
	resp, err := client.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	drainAndClose(resp)

	// Second request - should be cached (private cache can cache private responses)
	resp, err = client.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	drainAndClose(resp)

	if counter != 1 {
		t.Fatalf("expected 1 request (private cache should cache private responses), got %d", counter)
	}

	if resp.Header.Get(XFromCache) != "1" {
		t.Fatal("expected response from cache")
	}
}

// TestMustUnderstandWithPrivatePublicCache tests that public cache rejects private responses even with must-understand
func TestMustUnderstandWithPrivatePublicCache(t *testing.T) {
	resetTest()
	counter := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		w.Header().Set("Cache-Control", "must-understand, private, max-age=3600")
	}))
	defer s.Close()

	// Test with public cache
	tp := NewMemoryCacheTransport()
	tp.IsPublicCache = true
	client := &http.Client{Transport: tp}

	// First request
	resp, err := client.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	drainAndClose(resp)

	// Second request - should NOT be cached (public cache cannot cache private responses)
	resp, err = client.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	drainAndClose(resp)

	if counter != 2 {
		t.Fatalf("expected 2 requests (public cache should not cache private responses), got %d", counter)
	}
}

// TestNoMustUnderstandWithNoStore tests normal behavior when must-understand is NOT present
func TestNoMustUnderstandWithNoStore(t *testing.T) {
	resetTest()
	counter := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		w.Header().Set("Cache-Control", "no-store, max-age=3600")
	}))
	defer s.Close()

	tp := NewMemoryCacheTransport()
	client := &http.Client{Transport: tp}

	// First request
	resp, err := client.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	drainAndClose(resp)

	// Second request - should NOT be cached (no-store without must-understand)
	resp, err = client.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	drainAndClose(resp)

	if counter != 2 {
		t.Fatalf("expected 2 requests (no-store should prevent caching), got %d", counter)
	}

	if resp.Header.Get(XFromCache) == "1" {
		t.Fatal("expected response NOT from cache")
	}
}

// TestMustUnderstandCombinations tests various combinations of must-understand with other directives
func TestMustUnderstandCombinations(t *testing.T) {
	tests := []struct {
		name         string
		cacheControl string
		statusCode   int
		shouldCache  bool
		description  string
	}{
		{
			name:         "must-understand + max-age (200)",
			cacheControl: "must-understand, max-age=3600",
			statusCode:   200,
			shouldCache:  true,
			description:  "Known status with must-understand should be cached",
		},
		{
			name:         "must-understand + max-age (418)",
			cacheControl: "must-understand, max-age=3600",
			statusCode:   418,
			shouldCache:  false,
			description:  "Unknown status with must-understand should NOT be cached",
		},
		{
			name:         "must-understand + public (200)",
			cacheControl: "must-understand, public, max-age=3600",
			statusCode:   200,
			shouldCache:  true,
			description:  "must-understand with public directive should cache",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetTest()
			counter := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				counter++
				w.Header().Set("Cache-Control", tt.cacheControl)
				w.WriteHeader(tt.statusCode)
			}))
			defer s.Close()

			tp := NewMemoryCacheTransport()
			client := &http.Client{Transport: tp}

			// First request
			resp, err := client.Get(s.URL)
			if err != nil {
				t.Fatal(err)
			}
			drainAndClose(resp)

			// Wait a bit to ensure caching has happened
			time.Sleep(10 * time.Millisecond)

			// Second request
			resp, err = client.Get(s.URL)
			if err != nil {
				t.Fatal(err)
			}
			drainAndClose(resp)

			if tt.shouldCache {
				if counter != 1 {
					t.Fatalf("%s: expected 1 request (cached), got %d", tt.description, counter)
				}
				if resp.Header.Get(XFromCache) != "1" {
					t.Fatalf("%s: expected response from cache", tt.description)
				}
			} else {
				if counter != 2 {
					t.Fatalf("%s: expected 2 requests (not cached), got %d", tt.description, counter)
				}
				if resp.Header.Get(XFromCache) == "1" {
					t.Fatalf("%s: expected response NOT from cache", tt.description)
				}
			}
		})
	}
}
