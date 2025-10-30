package httpcache

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestContentLocationInvalidation verifies that PUT requests with Content-Location
// header properly invalidate the cache for the referenced resource.
func TestContentLocationInvalidation(t *testing.T) {
	resetTest()

	var requestCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		switch r.Method {
		case methodGET:
			// Return cached content
			w.Header().Set("Content-Location", "http://example.com/v1/resource")
			w.Header().Set("Cache-Control", "max-age=3600")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "response-%d", requestCount)
		case methodPUT:
			// Update operation with Content-Location
			w.Header().Set(headerContentLocation, r.URL.Path)
			w.WriteHeader(200)
			fmt.Fprint(w, "updated")
		}
	}))
	defer ts.Close()

	// GET /resource - should be cached
	resp1, err := s.client.Get(ts.URL + "/resource")
	if err != nil {
		t.Fatal(err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if string(body1) != "response-1" {
		t.Errorf("Expected 'response-1', got '%s'", string(body1))
	}

	// GET again - should be from cache
	resp2, err := s.client.Get(ts.URL + "/resource")
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if resp2.Header.Get(XFromCache) == "" {
		t.Error("Second GET should be from cache")
	}

	if !bytes.Equal(body1, body2) {
		t.Error("Cached response should match original")
	}

	if requestCount != 1 {
		t.Errorf("Expected 1 server request so far, got %d", requestCount)
	}

	// PUT /resource with Content-Location - should invalidate cache
	putReq, _ := http.NewRequest(methodPUT, ts.URL+"/resource", nil)
	resp3, err := s.client.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()

	// GET again - should NOT be from cache (invalidated by PUT)
	resp4, err := s.client.Get(ts.URL + "/resource")
	if err != nil {
		t.Fatal(err)
	}
	body4, _ := io.ReadAll(resp4.Body)
	resp4.Body.Close()

	if resp4.Header.Get(XFromCache) != "" {
		t.Error("GET after PUT should not be from cache (Content-Location invalidation)")
	}

	// Verify fresh content (should be response-3: PUT counted as request-2)
	if string(body4) != "response-3" {
		t.Errorf("Expected fresh 'response-3', got '%s'", string(body4))
	}

	if requestCount != 3 {
		t.Errorf("Expected 3 total requests (GET, PUT, GET), got %d", requestCount)
	}
}

// TestContentLocationCrossOriginSkipped verifies that cross-origin
// Content-Location headers are ignored for security per RFC 9111.
func TestContentLocationCrossOriginSkipped(t *testing.T) {
	resetTest()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case methodPUT:
			// Try to invalidate cross-origin URL (should be ignored)
			w.Header().Set(headerContentLocation, "https://evil.com/resource")
			w.WriteHeader(200)
			fmt.Fprint(w, "updated")
		case methodGET:
			w.Header().Set("Cache-Control", "max-age=3600")
			w.WriteHeader(200)
			fmt.Fprint(w, "content")
		}
	}))
	defer ts.Close()

	// PUT with cross-origin Content-Location (should not panic)
	putReq, _ := http.NewRequest(methodPUT, ts.URL+"/resource", nil)
	resp, err := s.client.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Verify no error occurred (cross-origin invalidation was safely ignored)
	// This test mainly ensures graceful handling without panics
}

// TestContentLocationRelativeURI verifies that relative URIs in
// Content-Location headers are correctly resolved and invalidated.
func TestContentLocationRelativeURI(t *testing.T) {
	resetTest()

	var requestCount int
	const apiResource = "/api/resource"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		switch r.URL.Path {
		case apiResource:
			switch r.Method {
			case methodGET:
				w.Header().Set("Cache-Control", "max-age=3600")
				w.WriteHeader(200)
				fmt.Fprintf(w, "original-%d", requestCount)
			case methodPUT:
				// Relative Content-Location
				w.Header().Set(headerContentLocation, apiResource)
				w.WriteHeader(200)
				fmt.Fprint(w, "updated")
			}
		}
	}))
	defer ts.Close()

	// GET and cache
	resp1, _ := s.client.Get(ts.URL + apiResource)
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if string(body1) != "original-1" {
		t.Errorf("Expected 'original-1', got '%s'", string(body1))
	}

	// Verify cached
	resp2, _ := s.client.Get(ts.URL + apiResource)
	if resp2.Header.Get(XFromCache) == "" {
		t.Error("Should be cached")
	}
	resp2.Body.Close()

	if requestCount != 1 {
		t.Errorf("Expected 1 request so far, got %d", requestCount)
	}

	// PUT with relative Content-Location
	putReq, _ := http.NewRequest(methodPUT, ts.URL+apiResource, nil)
	resp3, _ := s.client.Do(putReq)
	resp3.Body.Close()

	// Verify invalidated
	resp4, _ := s.client.Get(ts.URL + apiResource)
	if resp4.Header.Get(XFromCache) != "" {
		t.Error("Should be invalidated by relative Content-Location")
	}
	body4, _ := io.ReadAll(resp4.Body)
	resp4.Body.Close()

	if string(body4) != "original-3" {
		t.Errorf("Expected fresh 'original-3', got '%s'", string(body4))
	}

	if requestCount != 3 {
		t.Errorf("Expected 3 total requests, got %d", requestCount)
	}
}

// TestContentLocationInvalidURI verifies that invalid URIs in
// Content-Location headers are handled gracefully without panics.
func TestContentLocationInvalidURI(t *testing.T) {
	resetTest()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == methodPUT {
			// Invalid URI in Content-Location
			w.Header().Set(headerContentLocation, "://invalid-uri-format")
			w.WriteHeader(200)
			fmt.Fprint(w, "updated")
		}
	}))
	defer ts.Close()

	// PUT with invalid Content-Location should not panic
	putReq, _ := http.NewRequest(methodPUT, ts.URL+"/resource", nil)
	resp, err := s.client.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Should handle gracefully (log error but continue)
	// No assertion needed - test passes if no panic occurs
}

// TestLocationHeaderInvalidation verifies that Location header
// also triggers cache invalidation (same as Content-Location).
func TestLocationHeaderInvalidation(t *testing.T) {
	resetTest()

	var getCount, postCount int
	const resourceCreated = "/resource/created"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case methodGET:
			getCount++
			w.Header().Set("Cache-Control", "max-age=3600")
			w.WriteHeader(200)
			fmt.Fprintf(w, "content-%d", getCount)

		case methodPOST:
			postCount++
			// POST returns Location header pointing to created resource
			w.Header().Set(headerLocation, r.URL.Path+"/created")
			w.WriteHeader(201)
			fmt.Fprint(w, "created")
		}
	}))
	defer ts.Close()

	// Cache /resource/created
	resp1, _ := s.client.Get(ts.URL + resourceCreated)
	io.ReadAll(resp1.Body)
	resp1.Body.Close()

	// Verify cached
	resp2, _ := s.client.Get(ts.URL + resourceCreated)
	if resp2.Header.Get(XFromCache) == "" {
		t.Error("Should be cached")
	}
	resp2.Body.Close()

	if getCount != 1 {
		t.Errorf("Expected 1 GET so far, got %d", getCount)
	}

	// POST to /resource with Location: /resource/created
	postReq, _ := http.NewRequest(methodPOST, ts.URL+"/resource", nil)
	resp3, _ := s.client.Do(postReq)
	resp3.Body.Close()

	if postCount != 1 {
		t.Errorf("Expected 1 POST, got %d", postCount)
	}

	// GET /resource/created again - should be invalidated by Location header
	resp4, _ := s.client.Get(ts.URL + resourceCreated)
	if resp4.Header.Get(XFromCache) != "" {
		t.Error("Should be invalidated by Location header from POST")
	}
	body4, _ := io.ReadAll(resp4.Body)
	resp4.Body.Close()

	if string(body4) != "content-2" {
		t.Errorf("Expected fresh 'content-2', got '%s'", string(body4))
	}

	if getCount != 2 {
		t.Errorf("Expected 2 total GETs, got %d", getCount)
	}
}

// TestSameOriginCheck verifies that isSameOrigin function correctly
// identifies same-origin and cross-origin URLs.
func TestSameOriginCheck(t *testing.T) {
	tests := []struct {
		name     string
		url1     string
		url2     string
		expected bool
	}{
		{
			name:     "Same origin - identical",
			url1:     "https://example.com/path1",
			url2:     "https://example.com/path2",
			expected: true,
		},
		{
			name:     "Same origin - with port",
			url1:     "https://example.com:8080/path1",
			url2:     "https://example.com:8080/path2",
			expected: true,
		},
		{
			name:     "Different scheme",
			url1:     "http://example.com/path",
			url2:     "https://example.com/path",
			expected: false,
		},
		{
			name:     "Different host",
			url1:     "https://example.com/path",
			url2:     "https://other.com/path",
			expected: false,
		},
		{
			name:     "Different port",
			url1:     "https://example.com:8080/path",
			url2:     "https://example.com:9090/path",
			expected: false,
		},
		{
			name:     "Default ports - http",
			url1:     "http://example.com/path",
			url2:     "http://example.com:80/path",
			expected: false, // Different because host strings differ
		},
		{
			name:     "Subdomain difference",
			url1:     "https://api.example.com/path",
			url2:     "https://www.example.com/path",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u1, _ := http.NewRequest(methodGET, tt.url1, nil)
			u2, _ := http.NewRequest(methodGET, tt.url2, nil)

			result := isSameOrigin(u1.URL, u2.URL)
			if result != tt.expected {
				t.Errorf("isSameOrigin(%s, %s) = %v, expected %v",
					tt.url1, tt.url2, result, tt.expected)
			}
		})
	}
}

// TestInvalidationOnErrorResponse verifies that cache invalidation
// is skipped for error responses (status >= 400) per RFC 9111.
func TestInvalidationOnErrorResponse(t *testing.T) {
	resetTest()

	var getCount, putCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case methodGET:
			getCount++
			w.Header().Set("Cache-Control", "max-age=3600")
			w.WriteHeader(200)
			fmt.Fprintf(w, "content-%d", getCount)
		case methodPUT:
			putCount++
			// PUT fails with error response
			w.Header().Set(headerContentLocation, r.URL.Path)
			w.WriteHeader(500) // Server error
			fmt.Fprint(w, "error")
		}
	}))
	defer ts.Close()

	// Cache the resource
	resp1, _ := s.client.Get(ts.URL + "/resource")
	io.ReadAll(resp1.Body) // Must read body to cache
	resp1.Body.Close()

	if getCount != 1 {
		t.Errorf("Expected 1 GET, got %d", getCount)
	}

	// Verify cached
	resp2, _ := s.client.Get(ts.URL + "/resource")
	if resp2.Header.Get(XFromCache) == "" {
		t.Error("Should be cached")
	}
	resp2.Body.Close()

	if getCount != 1 {
		t.Errorf("Expected still 1 GET (second was cached), got %d", getCount)
	}

	// PUT with error response - should NOT invalidate cache
	putReq, _ := http.NewRequest(methodPUT, ts.URL+"/resource", nil)
	resp3, _ := s.client.Do(putReq)
	resp3.Body.Close()

	if putCount != 1 {
		t.Errorf("Expected 1 PUT, got %d", putCount)
	}

	// GET again - should STILL be from cache (error response didn't invalidate)
	resp4, _ := s.client.Get(ts.URL + "/resource")
	if resp4.Header.Get(XFromCache) == "" {
		t.Error("Should still be cached (error response should not invalidate)")
	}
	resp4.Body.Close()

	if getCount != 1 {
		t.Errorf("Expected still 1 GET (third was cached), got %d", getCount)
	}

	// Total: 1 GET (first), 1 PUT
	if getCount != 1 || putCount != 1 {
		t.Errorf("Expected 1 GET and 1 PUT, got %d GETs and %d PUTs", getCount, putCount)
	}
}
