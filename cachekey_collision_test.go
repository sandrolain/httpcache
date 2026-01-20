package httpcache

import (
	"net/http"
	"net/url"
	"testing"
)

// parseURL is a helper function for tests
func parseTestURL(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return u
}

// TestCacheKeyCollisionPrevention verifies that cache keys with header values
// containing pipe separator don't collide with different header combinations
func TestCacheKeyCollisionPrevention(t *testing.T) {
	tests := []struct {
		name     string
		req1     *http.Request
		req2     *http.Request
		headers  []string
		wantSame bool
	}{
		{
			name: "collision prevented - pipe in single header value",
			req1: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"X-Custom": []string{"value1|value2"},
				},
			},
			req2: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"X-Custom": []string{"value1"},
					"X-Other":  []string{"value2"},
				},
			},
			headers:  []string{"X-Custom", "X-Other"},
			wantSame: false, // Must be different despite sorting
		},
		{
			name: "same key for identical headers",
			req1: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"Accept-Language": []string{"en-US"},
				},
			},
			req2: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"Accept-Language": []string{"en-US"},
				},
			},
			headers:  []string{"Accept-Language"},
			wantSame: true,
		},
		{
			name: "different key for different header values",
			req1: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"Accept-Language": []string{"en-US"},
				},
			},
			req2: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"Accept-Language": []string{"it-IT"},
				},
			},
			headers:  []string{"Accept-Language"},
			wantSame: false,
		},
		{
			name: "collision prevented - multiple pipes in value",
			req1: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"X-Custom": []string{"a|b|c"},
				},
			},
			req2: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"X-Custom": []string{"a"},
					"X-Other":  []string{"b"},
					"X-Third":  []string{"c"},
				},
			},
			headers:  []string{"X-Custom", "X-Other", "X-Third"},
			wantSame: false,
		},
		{
			name: "collision prevented - colon in value",
			req1: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"X-Custom": []string{"key:value"},
				},
			},
			req2: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"X-Custom": []string{"key"},
				},
			},
			headers:  []string{"X-Custom"},
			wantSame: false,
		},
		{
			name: "collision prevented - special characters",
			req1: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"X-Custom": []string{"a=b&c=d"},
				},
			},
			req2: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"X-Custom": []string{"a=b"},
					"X-Other":  []string{"c=d"},
				},
			},
			headers:  []string{"X-Custom", "X-Other"},
			wantSame: false,
		},
		{
			name: "same key with URL-encoded equivalent values",
			req1: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"X-Custom": []string{"hello world"},
				},
			},
			req2: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"X-Custom": []string{"hello world"},
				},
			},
			headers:  []string{"X-Custom"},
			wantSame: true, // Spaces are encoded consistently
		},
		{
			name: "different URLs produce different keys",
			req1: &http.Request{
				URL: parseTestURL("http://example.com/path1"),
				Header: http.Header{
					"X-Custom": []string{"value"},
				},
			},
			req2: &http.Request{
				URL: parseTestURL("http://example.com/path2"),
				Header: http.Header{
					"X-Custom": []string{"value"},
				},
			},
			headers:  []string{"X-Custom"},
			wantSame: false,
		},
		{
			name: "empty header value handled correctly",
			req1: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"X-Custom": []string{"value"},
				},
			},
			req2: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"X-Custom": []string{""},
				},
			},
			headers:  []string{"X-Custom"},
			wantSame: false, // Empty vs non-empty should be different
		},
		{
			name: "missing header produces different key",
			req1: &http.Request{
				URL: parseTestURL("http://example.com/test"),
				Header: http.Header{
					"X-Custom": []string{"value"},
				},
			},
			req2: &http.Request{
				URL:    parseTestURL("http://example.com/test"),
				Header: http.Header{},
			},
			headers:  []string{"X-Custom"},
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key1 := cacheKeyWithHeaders(tt.req1, tt.headers)
			key2 := cacheKeyWithHeaders(tt.req2, tt.headers)

			if tt.wantSame {
				if key1 != key2 {
					t.Errorf("Expected same key:\nkey1=%q\nkey2=%q", key1, key2)
				}
			} else {
				if key1 == key2 {
					t.Errorf("Expected different keys but got same:\nkey=%q\nreq1 headers=%v\nreq2 headers=%v",
						key1, tt.req1.Header, tt.req2.Header)
				}
			}

			// Log keys for debugging
			t.Logf("key1: %s", key1)
			t.Logf("key2: %s", key2)
		})
	}
}

// TestCacheKeyWithHeadersEncoding verifies URL encoding is applied correctly
func TestCacheKeyWithHeadersEncoding(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		headers       map[string]string
		cacheHeaders  []string
		expectedParts []string // Expected encoded parts in the key
	}{
		{
			name: "pipe character encoded",
			url:  "http://example.com",
			headers: map[string]string{
				"X-Custom": "value1|value2",
			},
			cacheHeaders:  []string{"X-Custom"},
			expectedParts: []string{"X-Custom:value1%7Cvalue2"}, // | is %7C
		},
		{
			name: "space encoded",
			url:  "http://example.com",
			headers: map[string]string{
				"X-Custom": "hello world",
			},
			cacheHeaders:  []string{"X-Custom"},
			expectedParts: []string{"X-Custom:hello+world"}, // space is +
		},
		{
			name: "special characters encoded",
			url:  "http://example.com",
			headers: map[string]string{
				"X-Custom": "a=b&c=d",
			},
			cacheHeaders:  []string{"X-Custom"},
			expectedParts: []string{"X-Custom:a%3Db%26c%3Dd"}, // = is %3D, & is %26
		},
		{
			name: "colon encoded",
			url:  "http://example.com",
			headers: map[string]string{
				"X-Custom": "key:value",
			},
			cacheHeaders:  []string{"X-Custom"},
			expectedParts: []string{"X-Custom:key%3Avalue"}, // : is %3A
		},
		{
			name: "multiple headers encoded and sorted",
			url:  "http://example.com",
			headers: map[string]string{
				"Z-Last":   "z|value",
				"A-First":  "a|value",
				"M-Middle": "m|value",
			},
			cacheHeaders: []string{"Z-Last", "A-First", "M-Middle"},
			expectedParts: []string{
				"A-First:a%7Cvalue",
				"M-Middle:m%7Cvalue",
				"Z-Last:z%7Cvalue",
			}, // Sorted alphabetically
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				URL:    parseTestURL(tt.url),
				Header: make(http.Header),
			}

			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			key := cacheKeyWithHeaders(req, tt.cacheHeaders)

			// Verify each expected part is in the key
			for _, expectedPart := range tt.expectedParts {
				if !containsString(key, expectedPart) {
					t.Errorf("Expected key to contain %q, but got key: %q", expectedPart, key)
				}
			}

			t.Logf("Generated key: %s", key)
		})
	}
}

// TestCacheKeyWithoutHeaders verifies basic cache key generation
func TestCacheKeyWithoutHeaders(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		url      string
		expected string
	}{
		{
			name:     "GET request uses URL only",
			method:   "GET",
			url:      "http://example.com/path",
			expected: "http://example.com/path",
		},
		{
			name:     "POST request includes method",
			method:   "POST",
			url:      "http://example.com/path",
			expected: "POST http://example.com/path",
		},
		{
			name:     "PUT request includes method",
			method:   "PUT",
			url:      "http://example.com/path",
			expected: "PUT http://example.com/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Method: tt.method,
				URL:    parseTestURL(tt.url),
			}

			key := cacheKey(req)

			if key != tt.expected {
				t.Errorf("Expected key %q, got %q", tt.expected, key)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsStringAt(s, substr))
}

func containsStringAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
