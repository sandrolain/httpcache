package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPragmaNoCacheRequest tests that Pragma: no-cache in request bypasses cache
// when Cache-Control is not present (HTTP/1.0 compatibility)
func TestPragmaNoCacheRequest(t *testing.T) {
	resetTest()
	clock = &fakeClock{}

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()

	// First request - should cache
	req1, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp1, err := tp.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp1.Body)
	defer resp1.Body.Close()

	if callCount != 1 {
		t.Errorf("Expected 1 request to server, got %d", callCount)
	}

	// Second request with Pragma: no-cache and no Cache-Control - should bypass cache
	req2, _ := http.NewRequest(methodGET, ts.URL, nil)
	req2.Header.Set("Pragma", "no-cache")
	resp2, err := tp.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	defer resp2.Body.Close()

	if callCount != 2 {
		t.Errorf("Pragma: no-cache should bypass cache. Expected 2 requests, got %d", callCount)
	}

	if resp2.Header.Get(XFromCache) == "1" {
		t.Error("Response should not be from cache when Pragma: no-cache is set")
	}
}

// TestPragmaNoCacheIgnoredWithCacheControl tests that Pragma: no-cache is ignored
// when Cache-Control is present (RFC 7234 Section 5.4)
func TestPragmaNoCacheIgnoredWithCacheControl(t *testing.T) {
	resetTest()
	clock = &fakeClock{}

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()

	// First request - should cache
	req1, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp1, err := tp.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp1.Body)
	defer resp1.Body.Close()

	if callCount != 1 {
		t.Errorf("Expected 1 request to server, got %d", callCount)
	}

	// Second request with both Pragma: no-cache and Cache-Control: max-age=3600
	// Pragma should be ignored, Cache-Control takes precedence
	// Request max-age=3600 allows cached response to be served
	req2, _ := http.NewRequest(methodGET, ts.URL, nil)
	req2.Header.Set("Pragma", "no-cache")
	req2.Header.Set("Cache-Control", "max-age=3600")
	resp2, err := tp.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	defer resp2.Body.Close()

	// Cache-Control: max-age=3600 with fresh response should serve from cache
	if callCount != 1 {
		t.Errorf("Cache-Control should take precedence over Pragma. Expected 1 request, got %d", callCount)
	}

	if resp2.Header.Get(XFromCache) != "1" {
		t.Error("Response should be from cache when Cache-Control overrides Pragma")
	}
}

// TestPragmaNoCacheOnlyInRequest tests that Pragma: no-cache only affects requests
func TestPragmaNoCacheOnlyInRequest(t *testing.T) {
	resetTest()
	clock = &fakeClock{}

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// Response has Pragma: no-cache but also max-age
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()

	// First request - should cache (Pragma in response is ignored)
	req1, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp1, err := tp.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp1.Body)
	defer resp1.Body.Close()

	if callCount != 1 {
		t.Errorf("Expected 1 request to server, got %d", callCount)
	}

	// Second request - should serve from cache
	req2, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp2, err := tp.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	defer resp2.Body.Close()

	if callCount != 1 {
		t.Errorf("Response Pragma should be ignored. Expected 1 request, got %d", callCount)
	}

	if resp2.Header.Get(XFromCache) != "1" {
		t.Error("Response should be from cache when response has Pragma: no-cache")
	}
}

// TestPragmaOtherValuesIgnored tests that Pragma values other than "no-cache" are ignored
func TestPragmaOtherValuesIgnored(t *testing.T) {
	resetTest()
	clock = &fakeClock{}

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()

	// First request - should cache
	req1, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp1, err := tp.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp1.Body)
	defer resp1.Body.Close()

	if callCount != 1 {
		t.Errorf("Expected 1 request to server, got %d", callCount)
	}

	// Second request with Pragma: some-other-value - should serve from cache
	req2, _ := http.NewRequest(methodGET, ts.URL, nil)
	req2.Header.Set("Pragma", "some-other-value")
	resp2, err := tp.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	defer resp2.Body.Close()

	if callCount != 1 {
		t.Errorf("Other Pragma values should be ignored. Expected 1 request, got %d", callCount)
	}

	if resp2.Header.Get(XFromCache) != "1" {
		t.Error("Response should be from cache when Pragma has value other than no-cache")
	}
}

// TestPragmaNoCacheCaseInsensitive tests that Pragma: no-cache is case-insensitive
func TestPragmaNoCacheCaseInsensitive(t *testing.T) {
	resetTest()
	clock = &fakeClock{}

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()

	// First request - should cache
	req1, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp1, err := tp.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp1.Body)
	defer resp1.Body.Close()

	if callCount != 1 {
		t.Errorf("Expected 1 request to server, got %d", callCount)
	}

	// Test various case variations
	testCases := []string{
		"no-cache",
		"No-Cache",
		"NO-CACHE",
		"No-cache",
	}

	for i, pragmaValue := range testCases {
		req, _ := http.NewRequest(methodGET, ts.URL, nil)
		req.Header.Set("Pragma", pragmaValue)
		resp, err := tp.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		io.ReadAll(resp.Body)
		defer resp.Body.Close()

		expectedCallCount := 2 + i
		if callCount != expectedCallCount {
			t.Errorf("Pragma: %s should bypass cache. Expected %d requests, got %d", pragmaValue, expectedCallCount, callCount)
		}
	}
}
