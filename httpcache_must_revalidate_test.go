package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestMustRevalidateEnforcement verifies that must-revalidate directive prevents serving stale responses
func TestMustRevalidateEnforcement(t *testing.T) {
	resetTest()

	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		w.Header().Set("Cache-Control", "max-age=1, must-revalidate")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()
	tp.MarkCachedResponses = true
	client := &http.Client{Transport: tp}

	// First request - not cached
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.Header.Get(XFromCache) != "" {
		t.Fatal("First request should not be from cache")
	}

	// Wait for response to become stale
	time.Sleep(2 * time.Second)

	// Second request with max-stale - should revalidate despite max-stale due to must-revalidate
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	req2.Header.Set("Cache-Control", "max-stale=3600")
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	// Should have hit server again because must-revalidate enforced
	if counter != 2 {
		t.Fatalf("Expected 2 server hits due to must-revalidate, got %d", counter)
	}
}

// TestMustRevalidateWithoutMaxStale verifies normal stale behavior with must-revalidate
func TestMustRevalidateWithoutMaxStale(t *testing.T) {
	resetTest()

	counter := 0
	etag := `"test-etag"`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		if r.Header.Get("If-None-Match") == etag {
			// Return 304 Not Modified
			w.WriteHeader(http.StatusNotModified)
			return
		}
		// First request
		w.Header().Set("Cache-Control", "max-age=1, must-revalidate")
		w.Header().Set("ETag", etag)
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()
	tp.MarkCachedResponses = true
	client := &http.Client{Transport: tp}

	// First request
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if counter != 1 {
		t.Fatalf("Expected 1 server hit, got %d", counter)
	}

	// Wait for response to become stale
	time.Sleep(2 * time.Second)

	// Second request - should revalidate with 304
	clock = &fakeClock{elapsed: 2 * time.Second}
	defer func() { clock = &realClock{} }()

	resp2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if resp2.Header.Get(XFromCache) != "1" {
		t.Fatal("Second request should be from cache")
	}

	if resp2.Header.Get(XRevalidated) != "1" {
		t.Fatal("Second request should be revalidated")
	}

	// Should have hit server twice (initial + revalidation)
	if counter != 2 {
		t.Fatalf("Expected 2 server hits (initial + revalidation), got %d", counter)
	}
}

// TestWithoutMustRevalidateAllowsStale verifies that without must-revalidate, max-stale works
func TestWithoutMustRevalidateAllowsStale(t *testing.T) {
	resetTest()

	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		// No must-revalidate directive
		w.Header().Set("Cache-Control", "max-age=1")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()
	tp.MarkCachedResponses = true
	client := &http.Client{Transport: tp}

	// First request - not cached
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Wait for response to become stale
	time.Sleep(2 * time.Second)

	// Second request with max-stale - should serve stale without revalidation
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	req2.Header.Set("Cache-Control", "max-stale=3600")
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if resp2.Header.Get(XFromCache) != "1" {
		t.Fatal("Second request should be from cache")
	}

	// Should only have hit server once (max-stale allows serving stale)
	if counter != 1 {
		t.Fatalf("Expected 1 server hit (max-stale should serve stale), got %d", counter)
	}
}

// TestMustRevalidateWithFreshResponse verifies that must-revalidate doesn't affect fresh responses
func TestMustRevalidateWithFreshResponse(t *testing.T) {
	resetTest()

	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		w.Header().Set("Cache-Control", "max-age=3600, must-revalidate")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()
	tp.MarkCachedResponses = true
	client := &http.Client{Transport: tp}

	// First request
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Second request - response is still fresh (max-age=3600)
	resp2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if resp2.Header.Get(XFromCache) != "1" {
		t.Fatal("Second request should be from cache")
	}

	// Should only have hit server once (response is fresh)
	if counter != 1 {
		t.Fatalf("Expected 1 server hit (response is fresh), got %d", counter)
	}
}

// TestMustRevalidateOverridesMaxStaleUnlimited verifies must-revalidate with unlimited max-stale
func TestMustRevalidateOverridesMaxStaleUnlimited(t *testing.T) {
	resetTest()

	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		w.Header().Set("Cache-Control", "max-age=1, must-revalidate")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()
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

	// Second request with unlimited max-stale (no value)
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	req2.Header.Set("Cache-Control", "max-stale")
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	// Should have hit server twice (must-revalidate overrides unlimited max-stale)
	if counter != 2 {
		t.Fatalf("Expected 2 server hits (must-revalidate overrides max-stale), got %d", counter)
	}
}
