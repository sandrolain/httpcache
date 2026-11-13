package httpcache

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestParseCacheControlDuplicates tests handling of duplicate Cache-Control directives
func TestParseCacheControlDuplicates(t *testing.T) {
	tests := []struct {
		name          string
		cacheControl  string
		expectedKey   string
		expectedValue string
		shouldHaveKey bool
	}{
		{
			name:          "duplicate max-age (uses first)",
			cacheControl:  "max-age=300, max-age=600",
			expectedKey:   "max-age",
			expectedValue: "300",
			shouldHaveKey: true,
		},
		{
			name:          "duplicate no-cache (uses first)",
			cacheControl:  "no-cache, max-age=300, no-cache",
			expectedKey:   "no-cache",
			expectedValue: "",
			shouldHaveKey: true,
		},
		{
			name:          "duplicate s-maxage (uses first)",
			cacheControl:  "s-maxage=100, s-maxage=200",
			expectedKey:   "s-maxage",
			expectedValue: "100",
			shouldHaveKey: true,
		},
		{
			name:          "duplicate private (uses first)",
			cacheControl:  "private, max-age=60, private",
			expectedKey:   "private",
			expectedValue: "",
			shouldHaveKey: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set("Cache-Control", tt.cacheControl)

			cc := parseCacheControl(headers, slog.Default())

			if tt.shouldHaveKey {
				value, exists := cc[tt.expectedKey]
				if !exists {
					t.Errorf("Expected key %q to exist in parsed cache control", tt.expectedKey)
					return
				}
				if value != tt.expectedValue {
					t.Errorf("Expected value %q for key %q, got %q", tt.expectedValue, tt.expectedKey, value)
				}
			}
		})
	}
}

// TestParseCacheControlConflicts tests handling of conflicting Cache-Control directives
func TestParseCacheControlConflicts(t *testing.T) {
	tests := []struct {
		name         string
		cacheControl string
		checkKey     string
		shouldExist  bool
		description  string
	}{
		{
			name:         "public + private (private wins)",
			cacheControl: "public, private, max-age=300",
			checkKey:     "public",
			shouldExist:  false,
			description:  "public should be removed when private is present",
		},
		{
			name:         "private + public (private wins)",
			cacheControl: "private, public, max-age=300",
			checkKey:     "public",
			shouldExist:  false,
			description:  "public should be removed when private is present",
		},
		{
			name:         "no-cache + max-age (both kept)",
			cacheControl: "no-cache, max-age=300",
			checkKey:     "max-age",
			shouldExist:  true,
			description:  "max-age kept for freshness calculation, no-cache forces revalidation",
		},
		{
			name:         "no-store + max-age (both kept)",
			cacheControl: "no-store, max-age=600",
			checkKey:     "max-age",
			shouldExist:  true,
			description:  "max-age kept but no-store prevents caching",
		},
		{
			name:         "no-store + must-revalidate (both kept)",
			cacheControl: "no-store, must-revalidate",
			checkKey:     "must-revalidate",
			shouldExist:  true,
			description:  "must-revalidate kept but irrelevant due to no-store",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set("Cache-Control", tt.cacheControl)

			cc := parseCacheControl(headers, slog.Default())

			_, exists := cc[tt.checkKey]
			if exists != tt.shouldExist {
				if tt.shouldExist {
					t.Errorf("%s: expected %q to exist but it doesn't", tt.description, tt.checkKey)
				} else {
					t.Errorf("%s: expected %q to be removed but it exists", tt.description, tt.checkKey)
				}
			}
		})
	}
}

// TestParseCacheControlInvalidValues tests handling of invalid Cache-Control values
func TestParseCacheControlInvalidValues(t *testing.T) {
	tests := []struct {
		name          string
		cacheControl  string
		checkKey      string
		expectedValue string
		description   string
	}{
		{
			name:          "negative max-age (treated as 0)",
			cacheControl:  "max-age=-100",
			checkKey:      "max-age",
			expectedValue: "0",
			description:   "negative max-age should be treated as 0",
		},
		{
			name:          "non-numeric max-age (removed)",
			cacheControl:  "max-age=invalid",
			checkKey:      "max-age",
			expectedValue: "",
			description:   "non-numeric max-age should be removed",
		},
		{
			name:          "negative s-maxage (treated as 0)",
			cacheControl:  "s-maxage=-50",
			checkKey:      "s-maxage",
			expectedValue: "0",
			description:   "negative s-maxage should be treated as 0",
		},
		{
			name:          "non-numeric s-maxage (removed)",
			cacheControl:  "s-maxage=abc",
			checkKey:      "s-maxage",
			expectedValue: "",
			description:   "non-numeric s-maxage should be removed",
		},
		{
			name:          "float max-age (removed)",
			cacheControl:  "max-age=30.5",
			checkKey:      "max-age",
			expectedValue: "",
			description:   "float max-age should be removed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set("Cache-Control", tt.cacheControl)

			cc := parseCacheControl(headers, slog.Default())

			if tt.expectedValue == "" {
				// Directive should be removed
				if _, exists := cc[tt.checkKey]; exists {
					t.Errorf("%s: expected %q to be removed", tt.description, tt.checkKey)
				}
			} else {
				// Directive should have specific value
				value, exists := cc[tt.checkKey]
				if !exists {
					t.Errorf("%s: expected %q to exist", tt.description, tt.checkKey)
					return
				}
				if value != tt.expectedValue {
					t.Errorf("%s: expected value %q, got %q", tt.description, tt.expectedValue, value)
				}
			}
		})
	}
}

// TestParseCacheControlComplexScenarios tests complex real-world scenarios
func TestParseCacheControlComplexScenarios(t *testing.T) {
	tests := []struct {
		name         string
		cacheControl string
		checks       map[string]string // key -> expected value (empty string means should exist with no value)
		notPresent   []string          // keys that should not be present
	}{
		{
			name:         "CDN response with duplicate and conflict",
			cacheControl: "public, private, max-age=300, max-age=600, s-maxage=100",
			checks: map[string]string{
				"private":  "",
				"max-age":  "300", // First duplicate wins
				"s-maxage": "100",
			},
			notPresent: []string{"public"}, // Removed due to conflict with private
		},
		{
			name:         "Invalid and valid mixed",
			cacheControl: "max-age=-10, no-cache, s-maxage=invalid, must-revalidate",
			checks: map[string]string{
				"max-age":         "0", // Negative becomes 0
				"no-cache":        "",
				"must-revalidate": "",
			},
			notPresent: []string{"s-maxage"}, // Invalid removed
		},
		{
			name:         "Multiple conflicts",
			cacheControl: "public, private, no-store, max-age=300, must-revalidate",
			checks: map[string]string{
				"private":         "",
				"no-store":        "",
				"max-age":         "300",
				"must-revalidate": "",
			},
			notPresent: []string{"public"},
		},
		{
			name:         "Whitespace variations",
			cacheControl: " max-age = 300 , no-cache , private ",
			checks: map[string]string{
				"max-age":  "300",
				"no-cache": "",
				"private":  "",
			},
			notPresent: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set("Cache-Control", tt.cacheControl)

			cc := parseCacheControl(headers, slog.Default())

			// Check expected keys
			for key, expectedValue := range tt.checks {
				value, exists := cc[key]
				if !exists {
					t.Errorf("Expected %q to exist in cache control", key)
					continue
				}
				if value != expectedValue {
					t.Errorf("For key %q, expected value %q, got %q", key, expectedValue, value)
				}
			}

			// Check keys that should not be present
			for _, key := range tt.notPresent {
				if _, exists := cc[key]; exists {
					t.Errorf("Expected %q to NOT exist in cache control", key)
				}
			}
		})
	}
}

// TestParseCacheControlEmptyAndWhitespace tests edge cases with empty values
func TestParseCacheControlEmptyAndWhitespace(t *testing.T) {
	tests := []struct {
		name         string
		cacheControl string
		expectedKeys []string
	}{
		{
			name:         "empty cache-control",
			cacheControl: "",
			expectedKeys: []string{},
		},
		{
			name:         "only whitespace",
			cacheControl: "   ",
			expectedKeys: []string{},
		},
		{
			name:         "only commas",
			cacheControl: ",,,",
			expectedKeys: []string{},
		},
		{
			name:         "mixed empty parts",
			cacheControl: ", , max-age=300, , ",
			expectedKeys: []string{"max-age"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set("Cache-Control", tt.cacheControl)

			cc := parseCacheControl(headers, slog.Default())

			if len(cc) != len(tt.expectedKeys) {
				t.Errorf("Expected %d keys, got %d", len(tt.expectedKeys), len(cc))
			}

			for _, key := range tt.expectedKeys {
				if _, exists := cc[key]; !exists {
					t.Errorf("Expected key %q to exist", key)
				}
			}
		})
	}
}

// TestParseCacheControlPreservesValidDirectives tests that valid directives are preserved
func TestParseCacheControlPreservesValidDirectives(t *testing.T) {
	cacheControl := "public, max-age=3600, s-maxage=7200, must-revalidate, no-transform"
	headers := http.Header{}
	headers.Set("Cache-Control", cacheControl)

	cc := parseCacheControl(headers, slog.Default())

	expected := map[string]string{
		"public":          "",
		"max-age":         "3600",
		"s-maxage":        "7200",
		"must-revalidate": "",
		"no-transform":    "",
	}

	for key, expectedValue := range expected {
		value, exists := cc[key]
		if !exists {
			t.Errorf("Expected %q to exist", key)
			continue
		}
		if value != expectedValue {
			t.Errorf("For %q, expected %q, got %q", key, expectedValue, value)
		}
	}

	if len(cc) != len(expected) {
		t.Errorf("Expected %d directives, got %d", len(expected), len(cc))
	}
}

// TestParseCacheControlCaseInsensitive verifies RFC 9111 §5.2: cache directive
// names are tokens compared case-insensitively (issue #5).
func TestParseCacheControlCaseInsensitive(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected cacheControl
	}{
		{
			name:   "uppercase directives",
			header: "MAX-AGE=3600, NO-CACHE",
			expected: cacheControl{
				"max-age":  "3600",
				"no-cache": "",
			},
		},
		{
			name:   "mixed case directives",
			header: "Max-Age=60, Must-Revalidate, S-Maxage=120, No-Store",
			expected: cacheControl{
				"max-age":         "60",
				"must-revalidate": "",
				"s-maxage":        "120",
				"no-store":        "",
			},
		},
		{
			name:   "duplicate detection across case variants (first wins)",
			header: "MAX-AGE=100, max-age=200",
			expected: cacheControl{
				"max-age": "100",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set("Cache-Control", tt.header)

			cc := parseCacheControl(headers, slog.Default())

			if len(cc) != len(tt.expected) {
				t.Errorf("Expected %d directives, got %d: %v", len(tt.expected), len(cc), cc)
			}
			for k, v := range tt.expected {
				if got, ok := cc[k]; !ok {
					t.Errorf("Expected normalized directive %q, got %v", k, cc)
				} else if got != v {
					t.Errorf("Expected %q=%q, got %q", k, v, got)
				}
			}
		})
	}
}

// TestParseCacheControlMultipleHeaderLines verifies RFC 9110 §5.2/§5.3: repeated
// Cache-Control field lines are combined into one list; directives on any line
// carry semantics (issue #5).
func TestParseCacheControlMultipleHeaderLines(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		expected cacheControl
	}{
		{
			name:  "directive on second line is honored",
			lines: []string{"max-age=3600", "no-store"},
			expected: cacheControl{
				"max-age":  "3600",
				"no-store": "",
			},
		},
		{
			name:  "duplicate across lines uses first occurrence",
			lines: []string{"max-age=100", "max-age=200"},
			expected: cacheControl{
				"max-age": "100",
			},
		},
		{
			name:  "three lines combined",
			lines: []string{"public", "max-age=60", "must-revalidate"},
			expected: cacheControl{
				"public":          "",
				"max-age":         "60",
				"must-revalidate": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			for _, line := range tt.lines {
				headers.Add("Cache-Control", line)
			}

			cc := parseCacheControl(headers, slog.Default())

			if len(cc) != len(tt.expected) {
				t.Errorf("Expected %d directives, got %d: %v", len(tt.expected), len(cc), cc)
			}
			for k, v := range tt.expected {
				if got, ok := cc[k]; !ok {
					t.Errorf("Expected directive %q from any line, got %v", k, cc)
				} else if got != v {
					t.Errorf("Expected %q=%q, got %q", k, v, got)
				}
			}
		})
	}
}

// TestResponseWithUppercaseNoStoreNotCached verifies end-to-end that a response
// with a case-variant no-store directive is not stored (issue #5).
func TestResponseWithUppercaseNoStoreNotCached(t *testing.T) {
	resetTest()

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "NO-STORE")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("nocache-body"))
	}))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Second request must hit the origin (no-store honored, case-insensitively)
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, err := s.client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if requestCount != 2 {
		t.Errorf("Expected 2 origin requests (no-store honored), got %d", requestCount)
	}
	if resp2.Header.Get(XFromCache) == "1" {
		t.Error("Second response should not come from cache (NO-STORE must be honored)")
	}
}

// TestResponseSecondLineNoCacheForcesRevalidation verifies end-to-end that a
// no-cache directive on the second Cache-Control header line forces revalidation.
func TestResponseSecondLineNoCacheForcesRevalidation(t *testing.T) {
	resetTest()

	requestCount := 0
	etag := `"multi-line-etag"`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Add("Cache-Control", "no-cache")
		w.Header().Set("ETag", etag)
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("multi-line-body"))
	}))
	defer ts.Close()

	// First request: fetch and store
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Second request: must revalidate (no-cache on the second line)
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, err := s.client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if requestCount < 2 {
		t.Errorf("Expected revalidation due to second-line no-cache, origin requests: %d", requestCount)
	}
	if resp2.Header.Get(XRevalidated) != "1" {
		t.Error("Second request should have been revalidated (no-cache from second line honored)")
	}
}
