package httpcache

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	cacheControlMaxAge3600 = "max-age=3600"
	acceptLanguageHeader   = "Accept-Language"
	testResourcePath       = "/resource"
	varyHeader             = "Vary"
)

// TestVarySeparation verifies that responses with different Vary header values
// are stored as separate cache entries (RFC 9111 vary separation).
func TestVarySeparation(t *testing.T) {
	resetTest()
	s.transport.EnableVarySeparation = true // Enable vary separation for this test

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set(cacheControlHeader, cacheControlMaxAge3600)
		w.Header().Set(varyHeader, acceptLanguageHeader)

		// Return different content based on Accept-Language
		lang := r.Header.Get(acceptLanguageHeader)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "content-for-%s-%d", lang, requestCount)
	}))
	defer ts.Close()

	// First request with Accept-Language: en
	req1, _ := http.NewRequest(methodGET, ts.URL+testResourcePath, nil)
	req1.Header.Set(acceptLanguageHeader, "en")
	resp1, _ := s.client.Do(req1)
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if string(body1) != "content-for-en-1" {
		t.Errorf("Expected 'content-for-en-1', got '%s'", string(body1))
	}

	// Second request with Accept-Language: fr (different variant)
	req2, _ := http.NewRequest(methodGET, ts.URL+testResourcePath, nil)
	req2.Header.Set(acceptLanguageHeader, "fr")
	resp2, _ := s.client.Do(req2)
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if string(body2) != "content-for-fr-2" {
		t.Errorf("Expected 'content-for-fr-2', got '%s'", string(body2))
	}

	// Third request with Accept-Language: en again (should hit cache)
	req3, _ := http.NewRequest(methodGET, ts.URL+testResourcePath, nil)
	req3.Header.Set(acceptLanguageHeader, "en")
	resp3, _ := s.client.Do(req3)
	body3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()

	if string(body3) != "content-for-en-1" {
		t.Errorf("Expected cached 'content-for-en-1', got '%s'", string(body3))
	}

	if resp3.Header.Get(XFromCache) == "" {
		t.Error("Third request should be from cache (same Accept-Language as first)")
	}

	// Fourth request with Accept-Language: fr again (should hit cache)
	req4, _ := http.NewRequest(methodGET, ts.URL+testResourcePath, nil)
	req4.Header.Set(acceptLanguageHeader, "fr")
	resp4, _ := s.client.Do(req4)
	body4, _ := io.ReadAll(resp4.Body)
	resp4.Body.Close()

	if string(body4) != "content-for-fr-2" {
		t.Errorf("Expected cached 'content-for-fr-2', got '%s'", string(body4))
	}

	if resp4.Header.Get(XFromCache) == "" {
		t.Error("Fourth request should be from cache (same Accept-Language as second)")
	}

	// Verify we only made 2 requests to the server (one for each variant)
	if requestCount != 2 {
		t.Errorf("Expected 2 server requests (one per variant), got %d", requestCount)
	}
}

// TestVarySeparationMultipleHeaders verifies that vary separation works with multiple headers.
func TestVarySeparationMultipleHeaders(t *testing.T) {
	resetTest()
	s.transport.EnableVarySeparation = true // Enable vary separation for this test

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set(cacheControlHeader, cacheControlMaxAge3600)
		w.Header().Set(varyHeader, "Accept, Accept-Language")

		accept := r.Header.Get("Accept")
		lang := r.Header.Get(acceptLanguageHeader)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "content-%s-%s-%d", accept, lang, requestCount)
	}))
	defer ts.Close()

	// First request: Accept=text/html, Accept-Language=en
	req1, _ := http.NewRequest(methodGET, ts.URL+testResourcePath, nil)
	req1.Header.Set("Accept", "text/html")
	req1.Header.Set(acceptLanguageHeader, "en")
	resp1, _ := s.client.Do(req1)
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if string(body1) != "content-text/html-en-1" {
		t.Errorf("Expected 'content-text/html-en-1', got '%s'", string(body1))
	}

	// Second request: Accept=application/json, Accept-Language=en (different variant)
	req2, _ := http.NewRequest(methodGET, ts.URL+testResourcePath, nil)
	req2.Header.Set("Accept", "application/json")
	req2.Header.Set(acceptLanguageHeader, "en")
	resp2, _ := s.client.Do(req2)
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if string(body2) != "content-application/json-en-2" {
		t.Errorf("Expected 'content-application/json-en-2', got '%s'", string(body2))
	}

	// Third request: Accept=text/html, Accept-Language=en (should hit cache)
	req3, _ := http.NewRequest(methodGET, ts.URL+testResourcePath, nil)
	req3.Header.Set("Accept", "text/html")
	req3.Header.Set(acceptLanguageHeader, "en")
	resp3, _ := s.client.Do(req3)
	body3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()

	if string(body3) != "content-text/html-en-1" {
		t.Errorf("Expected cached 'content-text/html-en-1', got '%s'", string(body3))
	}

	if resp3.Header.Get(XFromCache) == "" {
		t.Error("Third request should be from cache (matches first variant)")
	}

	if requestCount != 2 {
		t.Errorf("Expected 2 server requests, got %d", requestCount)
	}
}

// TestVarySeparationWithEmptyHeader verifies that empty vary header values are handled correctly.
func TestVarySeparationWithEmptyHeader(t *testing.T) {
	resetTest()
	s.transport.EnableVarySeparation = true // Enable vary separation for this test

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set(cacheControlHeader, cacheControlMaxAge3600)
		w.Header().Set(varyHeader, acceptLanguageHeader)

		lang := r.Header.Get(acceptLanguageHeader)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "content-%s-%d", lang, requestCount)
	}))
	defer ts.Close()

	// First request with Accept-Language: en
	req1, _ := http.NewRequest(methodGET, ts.URL+testResourcePath, nil)
	req1.Header.Set(acceptLanguageHeader, "en")
	resp1, _ := s.client.Do(req1)
	io.ReadAll(resp1.Body)
	resp1.Body.Close()

	// Second request without Accept-Language header (different variant)
	req2, _ := http.NewRequest(methodGET, ts.URL+testResourcePath, nil)
	// No Accept-Language header
	resp2, _ := s.client.Do(req2)
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if string(body2) != "content--2" {
		t.Errorf("Expected 'content--2', got '%s'", string(body2))
	}

	// Third request with Accept-Language: en again (should hit cache)
	req3, _ := http.NewRequest(methodGET, ts.URL+testResourcePath, nil)
	req3.Header.Set(acceptLanguageHeader, "en")
	resp3, _ := s.client.Do(req3)
	body3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()

	if string(body3) != "content-en-1" {
		t.Errorf("Expected cached 'content-en-1', got '%s'", string(body3))
	}

	if resp3.Header.Get(XFromCache) == "" {
		t.Error("Third request should be from cache")
	}

	if requestCount != 2 {
		t.Errorf("Expected 2 server requests, got %d", requestCount)
	}
}

// TestNoVarySeparation verifies that responses without Vary headers
// are NOT separated (single cache entry per URL).
func TestNoVarySeparation(t *testing.T) {
	resetTest()

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set(cacheControlHeader, cacheControlMaxAge3600)
		// No Vary header

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "content-%d", requestCount)
	}))
	defer ts.Close()

	// First request with Accept-Language: en
	req1, _ := http.NewRequest(methodGET, ts.URL+testResourcePath, nil)
	req1.Header.Set(acceptLanguageHeader, "en")
	resp1, _ := s.client.Do(req1)
	io.ReadAll(resp1.Body)
	resp1.Body.Close()

	// Second request with different Accept-Language (should still hit cache)
	req2, _ := http.NewRequest(methodGET, ts.URL+testResourcePath, nil)
	req2.Header.Set(acceptLanguageHeader, "fr")
	resp2, _ := s.client.Do(req2)
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if string(body2) != "content-1" {
		t.Errorf("Expected cached 'content-1', got '%s'", string(body2))
	}

	if resp2.Header.Get(XFromCache) == "" {
		t.Error("Second request should be from cache (no Vary header)")
	}

	if requestCount != 1 {
		t.Errorf("Expected 1 server request (no vary separation), got %d", requestCount)
	}
}
