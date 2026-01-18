package httpcache

import (
	"log/slog"
	"net/http"
	"testing"
)

// BenchmarkParseCacheControlCached benchmarks parsing with cache
func BenchmarkParseCacheControlCached(b *testing.B) {
	// Clear cache before benchmark
	cacheControlCache.Range(func(key, value interface{}) bool {
		cacheControlCache.Delete(key)
		return true
	})

	logger := slog.Default()
	headers := http.Header{}
	headers.Set("Cache-Control", "public, max-age=3600, must-revalidate")

	// First parse to populate cache
	parseCacheControl(headers, logger)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		parseCacheControl(headers, logger)
	}
}

// BenchmarkParseCacheControlUncached benchmarks parsing without cache
func BenchmarkParseCacheControlUncached(b *testing.B) {
	logger := slog.Default()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Use a different header each time to avoid cache hits
		headers := http.Header{}
		headers.Set("Cache-Control", "max-age="+string(rune('0'+i%10)))
		parseCacheControl(headers, logger)
	}
}

// BenchmarkParseCacheControlSimple benchmarks simple header parsing
func BenchmarkParseCacheControlSimple(b *testing.B) {
	// Clear cache before benchmark
	cacheControlCache.Range(func(key, value interface{}) bool {
		cacheControlCache.Delete(key)
		return true
	})

	logger := slog.Default()
	headers := http.Header{}
	headers.Set("Cache-Control", "max-age=3600")

	// First parse to populate cache
	parseCacheControl(headers, logger)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		parseCacheControl(headers, logger)
	}
}

// BenchmarkParseCacheControlComplex benchmarks complex header parsing
func BenchmarkParseCacheControlComplex(b *testing.B) {
	// Clear cache before benchmark
	cacheControlCache.Range(func(key, value interface{}) bool {
		cacheControlCache.Delete(key)
		return true
	})

	logger := slog.Default()
	headers := http.Header{}
	headers.Set("Cache-Control", "public, max-age=3600, s-maxage=7200, must-revalidate, proxy-revalidate, no-transform")

	// First parse to populate cache
	parseCacheControl(headers, logger)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		parseCacheControl(headers, logger)
	}
}

// BenchmarkParseCacheControlInternal benchmarks the internal parsing function directly
func BenchmarkParseCacheControlInternal(b *testing.B) {
	logger := slog.Default()
	ccHeader := "public, max-age=3600, must-revalidate"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		parseCacheControlInternal(ccHeader, logger)
	}
}

// BenchmarkParseCacheControlConcurrent benchmarks concurrent parsing with cache
func BenchmarkParseCacheControlConcurrent(b *testing.B) {
	// Clear cache before benchmark
	cacheControlCache.Range(func(key, value interface{}) bool {
		cacheControlCache.Delete(key)
		return true
	})

	logger := slog.Default()
	headers := http.Header{}
	headers.Set("Cache-Control", "public, max-age=3600, must-revalidate")

	// First parse to populate cache
	parseCacheControl(headers, logger)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			parseCacheControl(headers, logger)
		}
	})
}
