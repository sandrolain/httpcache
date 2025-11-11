package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestVaryWildcard verifies that Vary: * always fails to match (RFC 9111 Section 4.1)
func TestVaryWildcard(t *testing.T) {
	resetTest()

	// Create a cached response with Vary: *
	cachedResp := &http.Response{
		Header: http.Header{
			"Vary":                       []string{"*"},
			headerXVariedPrefix + "Test": []string{"value1"},
		},
	}

	// Create a request with the same header value
	req, _ := http.NewRequest(methodGET, "http://example.com/resource", nil)
	req.Header.Set("Test", "value1")

	// Should NOT match because Vary: *
	if varyMatches(cachedResp, req) {
		t.Error("Vary: * should never match")
	}

	// Try with different value
	req.Header.Set("Test", "value2")
	if varyMatches(cachedResp, req) {
		t.Error("Vary: * should never match, even with different values")
	}

	// Try with no header
	req.Header.Del("Test")
	if varyMatches(cachedResp, req) {
		t.Error("Vary: * should never match, even with missing headers")
	}
}

// TestVaryWildcardMixed verifies that Vary: * with other headers still fails
func TestVaryWildcardMixed(t *testing.T) {
	resetTest()

	// Create a cached response with Vary: *, Accept-Language
	cachedResp := &http.Response{
		Header: http.Header{
			"Vary":                                  []string{"*, Accept-Language"},
			headerXVariedPrefix + "Accept-Language": []string{"en"},
		},
	}

	req, _ := http.NewRequest(methodGET, "http://example.com/resource", nil)
	req.Header.Set("Accept-Language", "en")

	// Should NOT match because of *
	if varyMatches(cachedResp, req) {
		t.Error("Vary: *, Accept-Language should never match due to *")
	}
}

// TestVaryWhitespaceNormalization verifies whitespace normalization in header values
func TestVaryWhitespaceNormalization(t *testing.T) {
	resetTest()

	tests := []struct {
		name         string
		storedValue  string
		requestValue string
		shouldMatch  bool
	}{
		{
			name:         "exact match",
			storedValue:  "en, fr",
			requestValue: "en, fr",
			shouldMatch:  true,
		},
		{
			name:         "extra spaces",
			storedValue:  "en,  fr",
			requestValue: "en, fr",
			shouldMatch:  true,
		},
		{
			name:         "leading/trailing spaces",
			storedValue:  " en, fr ",
			requestValue: "en, fr",
			shouldMatch:  true,
		},
		{
			name:         "tabs instead of spaces",
			storedValue:  "en,\tfr",
			requestValue: "en, fr",
			shouldMatch:  true,
		},
		{
			name:         "multiple internal spaces",
			storedValue:  "en,    fr",
			requestValue: "en, fr",
			shouldMatch:  true,
		},
		{
			name:         "different values",
			storedValue:  "en, fr",
			requestValue: "de, it",
			shouldMatch:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cachedResp := &http.Response{
				Header: http.Header{
					"Vary":                                  []string{"Accept-Language"},
					headerXVariedPrefix + "Accept-Language": []string{tt.storedValue},
				},
			}

			req, _ := http.NewRequest(methodGET, "http://example.com/resource", nil)
			req.Header.Set("Accept-Language", tt.requestValue)

			match := varyMatches(cachedResp, req)
			if match != tt.shouldMatch {
				t.Errorf("Expected match=%v, got %v (stored=%q, request=%q)",
					tt.shouldMatch, match, tt.storedValue, tt.requestValue)
			}
		})
	}
}

// TestVaryCaseInsensitiveHeaderNames verifies case-insensitive header name matching
func TestVaryCaseInsensitiveHeaderNames(t *testing.T) {
	resetTest()

	tests := []struct {
		name          string
		varyHeader    string
		storedHeader  string
		requestHeader string
		shouldMatch   bool
	}{
		{
			name:          "lowercase vary, lowercase request",
			varyHeader:    "accept-language",
			storedHeader:  "Accept-Language",
			requestHeader: "accept-language",
			shouldMatch:   true,
		},
		{
			name:          "UPPERCASE vary, lowercase request",
			varyHeader:    "ACCEPT-LANGUAGE",
			storedHeader:  "Accept-Language",
			requestHeader: "accept-language",
			shouldMatch:   true,
		},
		{
			name:          "mixed case vary",
			varyHeader:    "AcCePt-LaNgUaGe",
			storedHeader:  "Accept-Language",
			requestHeader: "accept-language",
			shouldMatch:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canonicalName := http.CanonicalHeaderKey(tt.storedHeader)

			cachedResp := &http.Response{
				Header: http.Header{
					"Vary":                              []string{tt.varyHeader},
					headerXVariedPrefix + canonicalName: []string{"en"},
				},
			}

			req, _ := http.NewRequest(methodGET, "http://example.com/resource", nil)
			req.Header.Set(tt.requestHeader, "en")

			match := varyMatches(cachedResp, req)
			if match != tt.shouldMatch {
				t.Errorf("Expected match=%v, got %v (vary=%q, request header=%q)",
					tt.shouldMatch, match, tt.varyHeader, tt.requestHeader)
			}
		})
	}
}

// TestVaryAbsentHeaders verifies correct handling when headers are absent
func TestVaryAbsentHeaders(t *testing.T) {
	resetTest()

	t.Run("both absent - should match", func(t *testing.T) {
		cachedResp := &http.Response{
			Header: http.Header{
				"Vary": []string{"Accept-Language"},
				// No X-Varied-Accept-Language header (header was absent in stored request)
			},
		}

		req, _ := http.NewRequest(methodGET, "http://example.com/resource", nil)
		// No Accept-Language header in request

		if !varyMatches(cachedResp, req) {
			t.Error("Should match when both headers are absent")
		}
	})

	t.Run("stored present, request absent - should not match", func(t *testing.T) {
		cachedResp := &http.Response{
			Header: http.Header{
				"Vary":                                  []string{"Accept-Language"},
				headerXVariedPrefix + "Accept-Language": []string{"en"},
			},
		}

		req, _ := http.NewRequest(methodGET, "http://example.com/resource", nil)
		// No Accept-Language header in request

		if varyMatches(cachedResp, req) {
			t.Error("Should not match when stored has value but request does not")
		}
	})

	t.Run("stored absent, request present - should not match", func(t *testing.T) {
		cachedResp := &http.Response{
			Header: http.Header{
				"Vary": []string{"Accept-Language"},
				// No X-Varied-Accept-Language header
			},
		}

		req, _ := http.NewRequest(methodGET, "http://example.com/resource", nil)
		req.Header.Set("Accept-Language", "en")

		if varyMatches(cachedResp, req) {
			t.Error("Should not match when request has value but stored does not")
		}
	})
}

// TestVaryMultipleHeaders verifies matching with multiple Vary headers
func TestVaryMultipleHeaders(t *testing.T) {
	resetTest()

	t.Run("all match", func(t *testing.T) {
		cachedResp := &http.Response{
			Header: http.Header{
				"Vary":                                  []string{"Accept, Accept-Language"},
				headerXVariedPrefix + "Accept":          []string{"text/html"},
				headerXVariedPrefix + "Accept-Language": []string{"en"},
			},
		}

		req, _ := http.NewRequest(methodGET, "http://example.com/resource", nil)
		req.Header.Set("Accept", "text/html")
		req.Header.Set("Accept-Language", "en")

		if !varyMatches(cachedResp, req) {
			t.Error("Should match when all vary headers match")
		}
	})

	t.Run("one mismatch", func(t *testing.T) {
		cachedResp := &http.Response{
			Header: http.Header{
				"Vary":                                  []string{"Accept, Accept-Language"},
				headerXVariedPrefix + "Accept":          []string{"text/html"},
				headerXVariedPrefix + "Accept-Language": []string{"en"},
			},
		}

		req, _ := http.NewRequest(methodGET, "http://example.com/resource", nil)
		req.Header.Set("Accept", "text/html")
		req.Header.Set("Accept-Language", "fr") // Different!

		if varyMatches(cachedResp, req) {
			t.Error("Should not match when one vary header mismatches")
		}
	})
}

// TestVaryEmptyAndWhitespace verifies edge cases with empty strings and whitespace
func TestVaryEmptyAndWhitespace(t *testing.T) {
	resetTest()

	t.Run("empty string vs spaces", func(t *testing.T) {
		cachedResp := &http.Response{
			Header: http.Header{
				"Vary":                                  []string{"Accept-Language"},
				headerXVariedPrefix + "Accept-Language": []string{"   "},
			},
		}

		req, _ := http.NewRequest(methodGET, "http://example.com/resource", nil)
		req.Header.Set("Accept-Language", "")

		// After normalization, "   " becomes "" and "" is ""
		if !varyMatches(cachedResp, req) {
			t.Error("Whitespace-only should match empty after normalization")
		}
	})

	t.Run("empty vary header name", func(t *testing.T) {
		cachedResp := &http.Response{
			Header: http.Header{
				"Vary":                                  []string{"  , Accept-Language"},
				headerXVariedPrefix + "Accept-Language": []string{"en"},
			},
		}

		req, _ := http.NewRequest(methodGET, "http://example.com/resource", nil)
		req.Header.Set("Accept-Language", "en")

		// Empty header names should be ignored
		if !varyMatches(cachedResp, req) {
			t.Error("Empty vary header names should be ignored")
		}
	})
}

// TestVaryIntegrationWithCaching verifies Vary matching in real caching scenario
func TestVaryIntegrationWithCaching(t *testing.T) {
	resetTest()
	s.transport.EnableVarySeparation = true // Enable vary separation for proper matching

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set(cacheControlHeader, cacheControlMaxAge3600)
		w.Header().Set(varyHeader, acceptLanguageHeader)

		lang := r.Header.Get(acceptLanguageHeader)
		t.Logf("Server request %d: Accept-Language=%q", requestCount, lang)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("content-" + lang + "-" + string(rune('0'+requestCount))))
	}))
	defer ts.Close()

	// Test 1: Request with "en, fr" (with comma and space)
	req1, _ := http.NewRequest(methodGET, ts.URL+"/resource", nil)
	req1.Header.Set(acceptLanguageHeader, "en, fr")
	resp1, _ := s.client.Do(req1)
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	t.Logf("Request 1: body=%s, from-cache=%s", string(body1), resp1.Header.Get(XFromCache))

	// CRITICAL: Must drain body to trigger caching!

	// Test 2: Request with "en,fr" (no space after comma) - should hit cache due to normalization
	req2, _ := http.NewRequest(methodGET, ts.URL+"/resource", nil)
	req2.Header.Set(acceptLanguageHeader, "en,fr")
	resp2, _ := s.client.Do(req2)
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	t.Logf("Request 2: body=%s, from-cache=%s", string(body2), resp2.Header.Get(XFromCache))

	if resp2.Header.Get(XFromCache) == "" {
		t.Error("Second request should hit cache (whitespace normalized)")
	}

	if requestCount != 1 {
		t.Errorf("Expected 1 server request (whitespace normalized), got %d", requestCount)
	}

	// Test 3: Request with different value - should NOT hit cache
	req3, _ := http.NewRequest(methodGET, ts.URL+"/resource", nil)
	req3.Header.Set(acceptLanguageHeader, "de, it")
	resp3, _ := s.client.Do(req3)
	body3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()
	t.Logf("Request 3: body=%s, from-cache=%s", string(body3), resp3.Header.Get(XFromCache))

	if resp3.Header.Get(XFromCache) != "" {
		t.Error("Third request should not hit cache (different value)")
	}

	if requestCount != 2 {
		t.Errorf("Expected 2 server requests, got %d", requestCount)
	}
}
