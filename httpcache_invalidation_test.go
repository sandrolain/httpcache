package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestInvalidateOnPOST tests that POST requests invalidate the request URI
func TestInvalidateOnPOST(t *testing.T) {
	resetTest()
	clock = &fakeClock{}

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.Method {
		case methodGET:
			w.Header().Set("Cache-Control", "max-age=3600")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("GET response"))
		case methodPOST:
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("POST response"))
		}
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()

	// First GET - should cache
	req1, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp1, err := tp.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp1.Body)
	defer resp1.Body.Close()

	if callCount != 1 {
		t.Errorf("Expected 1 request after first GET, got %d", callCount)
	}

	// Second GET - should serve from cache
	req2, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp2, err := tp.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	defer resp2.Body.Close()

	if callCount != 1 {
		t.Errorf("Expected 1 request after second GET (cached), got %d", callCount)
	}

	if resp2.Header.Get(XFromCache) != "1" {
		t.Error("Second GET should be from cache")
	}

	// POST request - should invalidate cache
	req3, _ := http.NewRequest(methodPOST, ts.URL, nil)
	resp3, err := tp.RoundTrip(req3)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp3.Body)
	defer resp3.Body.Close()

	if callCount != 2 {
		t.Errorf("Expected 2 requests after POST, got %d", callCount)
	}

	// Third GET - cache should be invalidated, fetch from server
	req4, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp4, err := tp.RoundTrip(req4)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp4.Body)
	defer resp4.Body.Close()

	if callCount != 3 {
		t.Errorf("Expected 3 requests after third GET (cache invalidated), got %d", callCount)
	}

	if resp4.Header.Get(XFromCache) == "1" {
		t.Error("Third GET should not be from cache after POST invalidation")
	}
}

// TestInvalidateOnPUT tests that PUT requests invalidate the request URI
func TestInvalidateOnPUT(t *testing.T) {
	resetTest()
	clock = &fakeClock{}

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.Method {
		case methodGET:
			w.Header().Set("Cache-Control", "max-age=3600")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("GET response"))
		case methodPUT:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("PUT response"))
		}
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()

	// Cache a GET response
	req1, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp1, err := tp.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp1.Body)
	defer resp1.Body.Close()

	// PUT request - should invalidate
	req2, _ := http.NewRequest(methodPUT, ts.URL, nil)
	resp2, err := tp.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	defer resp2.Body.Close()

	// GET should not be cached
	req3, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp3, err := tp.RoundTrip(req3)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp3.Body)
	defer resp3.Body.Close()

	if callCount != 3 {
		t.Errorf("Expected 3 requests (GET, PUT, GET), got %d", callCount)
	}

	if resp3.Header.Get(XFromCache) == "1" {
		t.Error("GET after PUT should not be from cache")
	}
}

// TestInvalidateOnDELETE tests that DELETE requests invalidate the request URI
func TestInvalidateOnDELETE(t *testing.T) {
	resetTest()
	clock = &fakeClock{}

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.Method {
		case methodGET:
			w.Header().Set("Cache-Control", "max-age=3600")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("GET response"))
		case methodDELETE:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()

	// Cache a GET response
	req1, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp1, err := tp.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp1.Body)
	defer resp1.Body.Close()

	// DELETE request - should invalidate
	req2, _ := http.NewRequest(methodDELETE, ts.URL, nil)
	resp2, err := tp.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	defer resp2.Body.Close()

	// GET should not be cached
	req3, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp3, err := tp.RoundTrip(req3)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp3.Body)
	defer resp3.Body.Close()

	if callCount != 3 {
		t.Errorf("Expected 3 requests (GET, DELETE, GET), got %d", callCount)
	}

	if resp3.Header.Get(XFromCache) == "1" {
		t.Error("GET after DELETE should not be from cache")
	}
}

// TestInvalidateOnPATCH tests that PATCH requests invalidate the request URI
func TestInvalidateOnPATCH(t *testing.T) {
	resetTest()
	clock = &fakeClock{}

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.Method {
		case methodGET:
			w.Header().Set("Cache-Control", "max-age=3600")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("GET response"))
		case methodPATCH:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("PATCH response"))
		}
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()

	// Cache a GET response
	req1, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp1, err := tp.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp1.Body)
	defer resp1.Body.Close()

	// PATCH request - should invalidate
	req2, _ := http.NewRequest(methodPATCH, ts.URL, nil)
	resp2, err := tp.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	defer resp2.Body.Close()

	// GET should not be cached
	req3, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp3, err := tp.RoundTrip(req3)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp3.Body)
	defer resp3.Body.Close()

	if callCount != 3 {
		t.Errorf("Expected 3 requests (GET, PATCH, GET), got %d", callCount)
	}

	if resp3.Header.Get(XFromCache) == "1" {
		t.Error("GET after PATCH should not be from cache")
	}
}

// TestInvalidateLocationHeader tests that Location header URI is invalidated
func TestInvalidateLocationHeader(t *testing.T) {
	resetTest()
	clock = &fakeClock{}

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.Method {
		case methodGET:
			w.Header().Set("Cache-Control", "max-age=3600")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("GET response"))
		case methodPOST:
			// Return Location header pointing to a different resource
			w.Header().Set(headerLocation, "/created-resource")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("POST response"))
		}
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()

	// Cache both the base URL and the created resource URL
	req1, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp1, err := tp.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp1.Body)
	defer resp1.Body.Close()

	req2, _ := http.NewRequest(methodGET, ts.URL+"/created-resource", nil)
	resp2, err := tp.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	defer resp2.Body.Close()

	if callCount != 2 {
		t.Errorf("Expected 2 requests for initial GETs, got %d", callCount)
	}

	// Verify both are cached
	req3, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp3, err := tp.RoundTrip(req3)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp3.Body)
	defer resp3.Body.Close()

	req4, _ := http.NewRequest(methodGET, ts.URL+"/created-resource", nil)
	resp4, err := tp.RoundTrip(req4)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp4.Body)
	defer resp4.Body.Close()

	if callCount != 2 {
		t.Errorf("Expected still 2 requests (both cached), got %d", callCount)
	}

	// POST with Location header - should invalidate both URIs
	req5, _ := http.NewRequest(methodPOST, ts.URL, nil)
	resp5, err := tp.RoundTrip(req5)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp5.Body)
	defer resp5.Body.Close()

	if callCount != 3 {
		t.Errorf("Expected 3 requests after POST, got %d", callCount)
	}

	// Both GETs should now fetch from server (cache invalidated)
	req6, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp6, err := tp.RoundTrip(req6)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp6.Body)
	defer resp6.Body.Close()

	req7, _ := http.NewRequest(methodGET, ts.URL+"/created-resource", nil)
	resp7, err := tp.RoundTrip(req7)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp7.Body)
	defer resp7.Body.Close()

	if callCount != 5 {
		t.Errorf("Expected 5 requests (both caches invalidated), got %d", callCount)
	}

	if resp6.Header.Get(XFromCache) == "1" {
		t.Error("Base URL should not be cached after POST with Location header")
	}

	if resp7.Header.Get(XFromCache) == "1" {
		t.Error("Location URL should not be cached after POST with Location header")
	}
}

// TestInvalidateContentLocationHeader tests that Content-Location header URI is invalidated
func TestInvalidateContentLocationHeader(t *testing.T) {
	resetTest()
	clock = &fakeClock{}

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.Method {
		case methodGET:
			w.Header().Set("Cache-Control", "max-age=3600")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("GET response"))
		case methodPUT:
			// Return Content-Location header
			w.Header().Set(headerContentLocation, "/updated-resource")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("PUT response"))
		}
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()

	// Cache the resource that will be in Content-Location
	req1, _ := http.NewRequest(methodGET, ts.URL+"/updated-resource", nil)
	resp1, err := tp.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp1.Body)
	defer resp1.Body.Close()

	// Verify it's cached
	req2, _ := http.NewRequest(methodGET, ts.URL+"/updated-resource", nil)
	resp2, err := tp.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	defer resp2.Body.Close()

	if callCount != 1 {
		t.Errorf("Expected 1 request (second GET cached), got %d", callCount)
	}

	if resp2.Header.Get(XFromCache) != "1" {
		t.Error("Second GET should be from cache")
	}

	// PUT with Content-Location header - should invalidate the Content-Location URI
	req3, _ := http.NewRequest(methodPUT, ts.URL, nil)
	resp3, err := tp.RoundTrip(req3)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp3.Body)
	defer resp3.Body.Close()

	if callCount != 2 {
		t.Errorf("Expected 2 requests after PUT, got %d", callCount)
	}

	// GET the Content-Location URI - should fetch from server (cache invalidated)
	req4, _ := http.NewRequest(methodGET, ts.URL+"/updated-resource", nil)
	resp4, err := tp.RoundTrip(req4)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp4.Body)
	defer resp4.Body.Close()

	if callCount != 3 {
		t.Errorf("Expected 3 requests (cache invalidated), got %d", callCount)
	}

	if resp4.Header.Get(XFromCache) == "1" {
		t.Error("Content-Location URL should not be cached after PUT")
	}
}

// TestNoInvalidateOnErrorResponse tests that error responses (4xx, 5xx) don't invalidate cache
func TestNoInvalidateOnErrorResponse(t *testing.T) {
	resetTest()
	clock = &fakeClock{}

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.Method {
		case methodGET:
			w.Header().Set("Cache-Control", "max-age=3600")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("GET response"))
		case methodPOST:
			// Return error response
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("POST error"))
		}
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()

	// Cache a GET response
	req1, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp1, err := tp.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp1.Body)
	defer resp1.Body.Close()

	// Second GET should be cached
	req2, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp2, err := tp.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	defer resp2.Body.Close()

	if callCount != 1 {
		t.Errorf("Expected 1 request (second GET cached), got %d", callCount)
	}

	// POST with error response - should NOT invalidate cache
	req3, _ := http.NewRequest(methodPOST, ts.URL, nil)
	resp3, err := tp.RoundTrip(req3)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp3.Body)
	defer resp3.Body.Close()

	if callCount != 2 {
		t.Errorf("Expected 2 requests after POST error, got %d", callCount)
	}

	// Third GET should still be cached (not invalidated by error response)
	req4, _ := http.NewRequest(methodGET, ts.URL, nil)
	resp4, err := tp.RoundTrip(req4)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp4.Body)
	defer resp4.Body.Close()

	if callCount != 2 {
		t.Errorf("Expected 2 requests (third GET still cached), got %d", callCount)
	}

	if resp4.Header.Get(XFromCache) != "1" {
		t.Error("Third GET should still be from cache after POST error response")
	}
}
