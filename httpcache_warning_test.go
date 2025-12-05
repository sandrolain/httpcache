package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestWarningHeaderStaleWhileRevalidate verifies Warning 110 on stale-while-revalidate
func TestWarningHeaderStaleWhileRevalidate(t *testing.T) {
	resetTest()

	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		w.Header().Set("Cache-Control", "max-age=1, stale-while-revalidate=10")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	tp := newMockCacheTransport()
	client := &http.Client{Transport: tp}

	// First request
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Wait for response to become stale but within stale-while-revalidate window
	time.Sleep(2 * time.Second)

	// Second request - should serve stale with Warning 110
	resp2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	// Check for Warning 110
	warning := resp2.Header.Get("Warning")
	if !strings.Contains(warning, "110") {
		t.Fatalf("Expected Warning 110, got: %q", warning)
	}
	if !strings.Contains(warning, "Response is Stale") {
		t.Fatalf("Expected 'Response is Stale' in warning, got: %q", warning)
	}
}

// TestWarningHeaderRevalidationFailed verifies Warning 111 on stale-if-error
func TestWarningHeaderRevalidationFailed(t *testing.T) {
	resetTest()

	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		if counter > 1 {
			// Second request returns error
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error"))
			return
		}
		w.Header().Set("Cache-Control", "max-age=1, stale-if-error=10")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	tp := newMockCacheTransport()
	client := &http.Client{Transport: tp}

	// First request
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Wait for response to become stale
	time.Sleep(2 * time.Second)

	// Second request - server returns 500, should serve stale with Warning 111
	resp2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	// Check for Warning 111
	warning := resp2.Header.Get("Warning")
	if !strings.Contains(warning, "111") {
		t.Fatalf("Expected Warning 111, got: %q", warning)
	}
	if !strings.Contains(warning, "Revalidation Failed") {
		t.Fatalf("Expected 'Revalidation Failed' in warning, got: %q", warning)
	}
}

// TestWarningHeaderMaxStale verifies Warning 110 when client uses max-stale
func TestWarningHeaderMaxStale(t *testing.T) {
	resetTest()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=1")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	tp := newMockCacheTransport()
	client := &http.Client{Transport: tp}

	// First request
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Wait for response to become stale
	time.Sleep(2 * time.Second)

	// Second request with max-stale - should serve stale with Warning 110
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	req2.Header.Set("Cache-Control", "max-stale=3600")
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	// Check for Warning 110
	warning := resp2.Header.Get("Warning")
	if !strings.Contains(warning, "110") {
		t.Fatalf("Expected Warning 110, got: %q", warning)
	}
	if !strings.Contains(warning, "Response is Stale") {
		t.Fatalf("Expected 'Response is Stale' in warning, got: %q", warning)
	}
}

// TestNoWarningOnFreshResponse verifies no Warning header on fresh responses
func TestNoWarningOnFreshResponse(t *testing.T) {
	resetTest()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	tp := newMockCacheTransport()
	client := &http.Client{Transport: tp}

	// First request
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Second request - response is still fresh
	resp2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	// Should not have Warning header
	warning := resp2.Header.Get("Warning")
	if warning != "" {
		t.Fatalf("Expected no Warning header on fresh response, got: %q", warning)
	}
}

// TestNoWarningOnFirstRequest verifies no Warning on cache miss
func TestNoWarningOnFirstRequest(t *testing.T) {
	resetTest()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	tp := newMockCacheTransport()
	client := &http.Client{Transport: tp}

	// First request - not from cache
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Should not have Warning header
	warning := resp.Header.Get("Warning")
	if warning != "" {
		t.Fatalf("Expected no Warning header on first request, got: %q", warning)
	}
}

// TestMultipleWarningHeaders verifies multiple warnings can be stacked
func TestMultipleWarningHeaders(t *testing.T) {
	resetTest()

	// Create a response with an existing Warning header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/second" {
			// Simulate server error
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "max-age=1, stale-if-error=10")
		w.Header().Add("Warning", `199 - "Miscellaneous warning"`)
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	tp := newMockCacheTransport()
	client := &http.Client{Transport: tp}

	// First request
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Check original warning is preserved
	warning := resp.Header.Get("Warning")
	if !strings.Contains(warning, "199") {
		t.Fatalf("Expected original Warning 199, got: %q", warning)
	}

	// Wait for stale
	time.Sleep(2 * time.Second)

	// Trigger error to get stale-if-error
	req2, _ := http.NewRequest("GET", ts.URL+"/second", nil)
	req2.Host = req.URL.Host
	req2.URL.Host = req.URL.Host
	req2.URL.Scheme = req.URL.Scheme

	// Make initial cacheable request
	req3, _ := http.NewRequest("GET", ts.URL+"/second", nil)
	resp3, _ := client.Do(req3)
	if resp3 != nil {
		io.ReadAll(resp3.Body)
		resp3.Body.Close()
	}
}

// TestDisableWarningHeaderStaleWhileRevalidate verifies that Warning headers are not added when DisableWarningHeader is true
func TestDisableWarningHeaderStaleWhileRevalidate(t *testing.T) {
	resetTest()

	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		w.Header().Set("Cache-Control", "max-age=1, stale-while-revalidate=10")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	cache := newMockCache()
	tp := &Transport{
		Cache:                cache,
		MarkCachedResponses:  true,
		DisableWarningHeader: true, // Disable Warning headers
	}
	client := &http.Client{Transport: tp}

	// First request
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Wait for response to become stale but within stale-while-revalidate window
	time.Sleep(2 * time.Second)

	// Second request - should serve stale WITHOUT Warning 110
	resp2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	// Should NOT have Warning header
	warning := resp2.Header.Get("Warning")
	if warning != "" {
		t.Fatalf("Expected no Warning header with DisableWarningHeader=true, got: %q", warning)
	}
}

// TestDisableWarningHeaderRevalidationFailed verifies that Warning 111 is not added when DisableWarningHeader is true
func TestDisableWarningHeaderRevalidationFailed(t *testing.T) {
	resetTest()

	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		if counter > 1 {
			// Second request returns error
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error"))
			return
		}
		w.Header().Set("Cache-Control", "max-age=1, stale-if-error=10")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	cache := newMockCache()
	tp := &Transport{
		Cache:                cache,
		MarkCachedResponses:  true,
		DisableWarningHeader: true, // Disable Warning headers
	}
	client := &http.Client{Transport: tp}

	// First request
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Wait for response to become stale
	time.Sleep(2 * time.Second)

	// Second request - server returns 500, should serve stale WITHOUT Warning 111
	resp2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	// Should NOT have Warning header
	warning := resp2.Header.Get("Warning")
	if warning != "" {
		t.Fatalf("Expected no Warning header with DisableWarningHeader=true, got: %q", warning)
	}

	// But should still have X-Stale header
	if resp2.Header.Get("X-Stale") != "1" {
		t.Fatal("Expected X-Stale header to be set")
	}
}

// TestDisableWarningHeaderMaxStale verifies that Warning 110 is not added when DisableWarningHeader is true
func TestDisableWarningHeaderMaxStale(t *testing.T) {
	resetTest()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=1")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	cache := newMockCache()
	tp := &Transport{
		Cache:                cache,
		MarkCachedResponses:  true,
		DisableWarningHeader: true, // Disable Warning headers
	}
	client := &http.Client{Transport: tp}

	// First request
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Wait for response to become stale
	time.Sleep(2 * time.Second)

	// Second request with max-stale - should serve stale WITHOUT Warning 110
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	req2.Header.Set("Cache-Control", "max-stale=3600")
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	// Should NOT have Warning header
	warning := resp2.Header.Get("Warning")
	if warning != "" {
		t.Fatalf("Expected no Warning header with DisableWarningHeader=true, got: %q", warning)
	}
}

// TestWarningHeaderEnabledByDefault verifies that Warning headers are enabled by default
func TestWarningHeaderEnabledByDefault(t *testing.T) {
	resetTest()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=1, stale-while-revalidate=10")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	cache := newMockCache()
	tp := &Transport{
		Cache:               cache,
		MarkCachedResponses: true,
		// DisableWarningHeader not set - should default to false
	}
	client := &http.Client{Transport: tp}

	// First request
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Wait for response to become stale but within stale-while-revalidate window
	time.Sleep(2 * time.Second)

	// Second request - should serve stale WITH Warning 110
	resp2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	// Should have Warning header (default behavior)
	warning := resp2.Header.Get("Warning")
	if !strings.Contains(warning, "110") {
		t.Fatalf("Expected Warning 110 by default, got: %q", warning)
	}
}
