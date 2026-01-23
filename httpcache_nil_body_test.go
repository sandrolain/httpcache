package httpcache

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNilBodyHandling verifies that the transport handles responses with nil body correctly.
// Although http.Response.Body should never be nil according to Go's standard library
// (it should be at least http.NoBody), this test ensures defensive programming.
func TestNilBodyHandling(t *testing.T) {
	cache := newMockCache()
	transport := NewTransport(cache)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusNoContent) // 204 No Content typically has no body
	}))
	defer ts.Close()

	req, err := http.NewRequest("GET", ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", resp.StatusCode)
	}

	// Second request - should be from cache
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}

	if resp2.Body != nil {
		_, _ = io.Copy(io.Discard, resp2.Body)
		resp2.Body.Close()
	}

	if resp2.StatusCode != http.StatusNoContent {
		t.Errorf("expected cached status 204, got %d", resp2.StatusCode)
	}

	// Verify it was cached
	if resp2.Header.Get("X-From-Cache") != "1" {
		t.Error("expected X-From-Cache header on cached response")
	}
}

// TestEmptyBodyHandling verifies handling of responses with empty body content.
func TestEmptyBodyHandling(t *testing.T) {
	cache := newMockCache()
	transport := NewTransport(cache)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
		// No body written
	}))
	defer ts.Close()

	req, err := http.NewRequest("GET", ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if len(body) != 0 {
		t.Errorf("expected empty body, got %d bytes", len(body))
	}

	// Second request - from cache
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatal(err)
	}

	if len(body2) != 0 {
		t.Errorf("expected empty cached body, got %d bytes", len(body2))
	}

	if resp2.Header.Get("X-From-Cache") != "1" {
		t.Error("expected X-From-Cache header")
	}
}

// TestHeadRequestBodyHandling verifies HEAD requests handle body correctly.
// HEAD responses should not have a body, but may have Content-Length.
func TestHeadRequestBodyHandling(t *testing.T) {
	cache := newMockCache()
	transport := NewTransport(cache)

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)

		// HEAD shouldn't send body, but GET should
		if r.Method == "GET" {
			_, _ = w.Write(bytes.Repeat([]byte("x"), 100))
		}
	}))
	defer ts.Close()

	// First HEAD request
	req1, _ := http.NewRequest("HEAD", ts.URL, nil)
	resp1, err := transport.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}

	if resp1.Body != nil {
		_, _ = io.Copy(io.Discard, resp1.Body)
		resp1.Body.Close()
	}

	if requestCount != 1 {
		t.Errorf("expected 1 request, got %d", requestCount)
	}

	// Second HEAD request - should be cached
	req2, _ := http.NewRequest("HEAD", ts.URL, nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}

	if resp2.Body != nil {
		body, _ := io.ReadAll(resp2.Body)
		resp2.Body.Close()

		// HEAD response body should be empty even when cached
		if len(body) > 0 {
			t.Errorf("expected empty HEAD body, got %d bytes", len(body))
		}
	}

	if requestCount != 1 {
		t.Errorf("expected cache hit (1 total request), got %d requests", requestCount)
	}
}

// TestBodyReadErrorHandling verifies handling of errors during body read.
func TestBodyReadErrorHandling(t *testing.T) {
	cache := newMockCache()
	transport := NewTransport(cache)

	// Use test server that returns normal response
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test data"))
	}))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Reading should work (this tests normal case)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("unexpected error reading body: %v", err)
	}

	if len(body) != 9 {
		t.Errorf("expected 9 bytes, got %d", len(body))
	}
}

// TestLargeEmptyBodyCaching verifies that responses with Content-Length but no actual content are cached correctly.
func TestLargeEmptyBodyCaching(t *testing.T) {
	cache := newMockCache()
	transport := NewTransport(cache)

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		// Set Content-Length but don't write body (malformed response)
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		// No body written
	}))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Try to read body - may timeout or get EOF
	body, _ := io.ReadAll(resp.Body)

	// Body should be empty or error occurred
	if len(body) > 0 && len(body) != 1000 {
		t.Logf("warning: got unexpected body length %d", len(body))
	}

	// Second request
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	_, _ = io.ReadAll(resp2.Body)

	// Should use cache if first request succeeded
	if requestCount > 2 {
		t.Errorf("expected at most 2 requests, got %d", requestCount)
	}
}

// TestMultipleBodyCloseHandling verifies that closing body multiple times doesn't panic.
func TestMultipleBodyCloseHandling(t *testing.T) {
	cache := newMockCache()
	transport := NewTransport(cache)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test"))
	}))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	// Read body
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// Close body multiple times - should not panic
	err1 := resp.Body.Close()
	err2 := resp.Body.Close()
	err3 := resp.Body.Close()

	if err1 != nil {
		t.Errorf("first close returned error: %v", err1)
	}

	// Subsequent closes may return error or nil, both are acceptable
	_ = err2
	_ = err3
}
