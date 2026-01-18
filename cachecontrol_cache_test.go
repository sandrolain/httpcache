package httpcache

import (
	"log/slog"
	"net/http"
	"testing"
)

// TestParseCacheControlCaching tests that parsing results are cached
func TestParseCacheControlCaching(t *testing.T) {
	// Clear cache before test
	cacheControlCache.Range(func(key, value interface{}) bool {
		cacheControlCache.Delete(key)
		return true
	})

	tests := []struct {
		name     string
		header   string
		expected cacheControl
	}{
		{
			name:   "max-age directive",
			header: "max-age=3600",
			expected: cacheControl{
				"max-age": "3600",
			},
		},
		{
			name:   "multiple directives",
			header: "public, max-age=3600, must-revalidate",
			expected: cacheControl{
				"public":          "",
				"max-age":         "3600",
				"must-revalidate": "",
			},
		},
		{
			name:   "complex header",
			header: "no-cache, no-store, must-revalidate, max-age=0",
			expected: cacheControl{
				"no-cache":        "",
				"no-store":        "",
				"must-revalidate": "",
				"max-age":         "0",
			},
		},
	}

	logger := slog.Default()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set("Cache-Control", tt.header)

			// First parse - should cache the result
			result1 := parseCacheControl(headers, logger)

			// Verify result is correct
			if len(result1) != len(tt.expected) {
				t.Errorf("expected %d directives, got %d", len(tt.expected), len(result1))
			}

			for k, v := range tt.expected {
				if result1[k] != v {
					t.Errorf("expected directive %s=%s, got %s", k, v, result1[k])
				}
			}

			// Second parse - should return cached result
			result2 := parseCacheControl(headers, logger)

			// Verify both results are equal
			if len(result1) != len(result2) {
				t.Errorf("cached result differs: expected %d directives, got %d", len(result1), len(result2))
			}

			for k, v := range result1 {
				if result2[k] != v {
					t.Errorf("cached result differs: expected directive %s=%s, got %s", k, v, result2[k])
				}
			}

			// Verify the result is in cache
			if _, ok := cacheControlCache.Load(tt.header); !ok {
				t.Errorf("expected result to be cached for header: %s", tt.header)
			}
		})
	}
}

// TestParseCacheControlCacheHitRate tests that cache is effective
func TestParseCacheControlCacheHitRate(t *testing.T) {
	// Clear cache before test
	cacheControlCache.Range(func(key, value interface{}) bool {
		cacheControlCache.Delete(key)
		return true
	})

	logger := slog.Default()
	headers := http.Header{}
	headers.Set("Cache-Control", "max-age=3600, public")

	// Count cache entries before
	countBefore := 0
	cacheControlCache.Range(func(key, value interface{}) bool {
		countBefore++
		return true
	})

	// Parse multiple times with same header
	for i := 0; i < 100; i++ {
		parseCacheControl(headers, logger)
	}

	// Count cache entries after
	countAfter := 0
	cacheControlCache.Range(func(key, value interface{}) bool {
		countAfter++
		return true
	})

	// Should have added exactly 1 entry to cache
	if countAfter-countBefore != 1 {
		t.Errorf("expected 1 cache entry, got %d", countAfter-countBefore)
	}
}

// TestParseCacheControlCacheDifferentHeaders tests caching with different headers
func TestParseCacheControlCacheDifferentHeaders(t *testing.T) {
	// Clear cache before test
	cacheControlCache.Range(func(key, value interface{}) bool {
		cacheControlCache.Delete(key)
		return true
	})

	logger := slog.Default()

	headers1 := http.Header{}
	headers1.Set("Cache-Control", "max-age=3600")

	headers2 := http.Header{}
	headers2.Set("Cache-Control", "max-age=7200")

	headers3 := http.Header{}
	headers3.Set("Cache-Control", "no-cache")

	// Parse different headers
	result1 := parseCacheControl(headers1, logger)
	result2 := parseCacheControl(headers2, logger)
	result3 := parseCacheControl(headers3, logger)

	// Verify each has correct value
	if result1["max-age"] != "3600" {
		t.Errorf("expected max-age=3600, got %s", result1["max-age"])
	}
	if result2["max-age"] != "7200" {
		t.Errorf("expected max-age=7200, got %s", result2["max-age"])
	}
	if _, ok := result3["no-cache"]; !ok {
		t.Errorf("expected no-cache directive")
	}

	// Count cache entries
	count := 0
	cacheControlCache.Range(func(key, value interface{}) bool {
		count++
		return true
	})

	// Should have 3 different cache entries
	if count != 3 {
		t.Errorf("expected 3 cache entries, got %d", count)
	}
}

// TestParseCacheControlEmptyHeader tests caching with empty header
func TestParseCacheControlEmptyHeader(t *testing.T) {
	// Clear cache before test
	cacheControlCache.Range(func(key, value interface{}) bool {
		cacheControlCache.Delete(key)
		return true
	})

	logger := slog.Default()
	headers := http.Header{}
	// No Cache-Control header set

	result := parseCacheControl(headers, logger)

	// Should return empty cacheControl
	if len(result) != 0 {
		t.Errorf("expected empty cacheControl, got %d directives", len(result))
	}

	// Should be cached
	if _, ok := cacheControlCache.Load(""); !ok {
		t.Errorf("expected empty header to be cached")
	}
}
