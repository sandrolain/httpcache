package httpcache

import (
	"net/http"
	"testing"
)

// BenchmarkCacheKeyComputation benchmarks the original cache key computation.
func BenchmarkCacheKeyComputation(b *testing.B) {
	req, _ := http.NewRequest("GET", "http://example.com/test/path?query=value", nil)
	req.Header.Set("X-Custom", "value1")
	req.Header.Set("Accept-Language", "en-US")

	headers := []string{"X-Custom", "Accept-Language"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cacheKeyWithHeaders(req, headers)
	}
}

// BenchmarkGetCacheKeyFirstCall benchmarks the first call to getCacheKey
// which computes and stores the key in context.
func BenchmarkGetCacheKeyFirstCall(b *testing.B) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache, WithCacheKeyHeaders([]string{"X-Custom", "Accept-Language"}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", "http://example.com/test/path?query=value", nil)
		req.Header.Set("X-Custom", "value1")
		req.Header.Set("Accept-Language", "en-US")

		_, _ = transport.getCacheKey(req)
	}
}

// BenchmarkGetCacheKeyMemoized benchmarks subsequent calls to getCacheKey
// which retrieve the key from context without recomputation.
func BenchmarkGetCacheKeyMemoized(b *testing.B) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache, WithCacheKeyHeaders([]string{"X-Custom", "Accept-Language"}))

	req, _ := http.NewRequest("GET", "http://example.com/test/path?query=value", nil)
	req.Header.Set("X-Custom", "value1")
	req.Header.Set("Accept-Language", "en-US")

	// First call to compute and store
	_, req = transport.getCacheKey(req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = transport.getCacheKey(req)
	}
}

// BenchmarkCacheKeyWithVary benchmarks cache key computation with Vary headers.
func BenchmarkCacheKeyWithVary(b *testing.B) {
	req, _ := http.NewRequest("GET", "http://example.com/test/path", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Accept-Language", "en-US")

	varyHeaders := []string{"Accept-Encoding", "Accept-Language"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cacheKeyWithVary(req, varyHeaders)
	}
}

// BenchmarkCacheKeyComparison compares direct computation vs memoized retrieval.
func BenchmarkCacheKeyComparison(b *testing.B) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache, WithCacheKeyHeaders([]string{"X-Custom", "Accept-Language"}))

	b.Run("DirectComputation", func(b *testing.B) {
		req, _ := http.NewRequest("GET", "http://example.com/test/path?query=value", nil)
		req.Header.Set("X-Custom", "value1")
		req.Header.Set("Accept-Language", "en-US")

		headers := []string{"X-Custom", "Accept-Language"}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = cacheKeyWithHeaders(req, headers)
		}
	})

	b.Run("MemoizedRetrieval", func(b *testing.B) {
		req, _ := http.NewRequest("GET", "http://example.com/test/path?query=value", nil)
		req.Header.Set("X-Custom", "value1")
		req.Header.Set("Accept-Language", "en-US")

		// First call to compute and store
		_, req = transport.getCacheKey(req)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = transport.getCacheKey(req)
		}
	})
}

// BenchmarkCacheKeySimpleURL benchmarks cache key for simple URL without headers.
func BenchmarkCacheKeySimpleURL(b *testing.B) {
	req, _ := http.NewRequest("GET", "http://example.com/path", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cacheKey(req)
	}
}

// BenchmarkCacheKeyComplexURL benchmarks cache key for complex URL with query params.
func BenchmarkCacheKeyComplexURL(b *testing.B) {
	req, _ := http.NewRequest("GET", "http://example.com/path?param1=value1&param2=value2&param3=value3", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cacheKey(req)
	}
}
