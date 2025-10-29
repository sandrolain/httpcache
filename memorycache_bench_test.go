package httpcache

import (
	"testing"
)

const benchmarkKey = "benchmark-key"

func BenchmarkMemoryCacheGet(b *testing.B) {
	cache := NewMemoryCache()
	value := make([]byte, 1024) // 1KB value
	cache.Set(benchmarkKey, value)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(benchmarkKey)
	}
}

func BenchmarkMemoryCacheSet(b *testing.B) {
	cache := NewMemoryCache()
	value := make([]byte, 1024) // 1KB value

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(benchmarkKey, value)
	}
}

func BenchmarkMemoryCacheDelete(b *testing.B) {
	cache := NewMemoryCache()
	value := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%26))
		cache.Set(key, value)
		cache.Delete(key)
	}
}

func BenchmarkMemoryCacheSetGet(b *testing.B) {
	cache := NewMemoryCache()
	value := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(benchmarkKey, value)
		cache.Get(benchmarkKey)
	}
}

func BenchmarkMemoryCacheParallelGet(b *testing.B) {
	cache := NewMemoryCache()
	value := make([]byte, 1024)

	// Pre-populate cache
	for i := 0; i < 26; i++ {
		key := string(rune('a' + i))
		cache.Set(key, value)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := string(rune('a' + i%26))
			cache.Get(key)
			i++
		}
	})
}

func BenchmarkMemoryCacheParallelSet(b *testing.B) {
	cache := NewMemoryCache()
	value := make([]byte, 1024)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := string(rune('a' + i%26))
			cache.Set(key, value)
			i++
		}
	})
}

// Benchmark with realistic HTTP response sizes
func BenchmarkMemoryCacheSetHTTPResponse(b *testing.B) {
	cache := NewMemoryCache()
	// Typical HTTP response with headers: ~2KB
	value := make([]byte, 2048)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%100))
		cache.Set(key, value)
	}
}

func BenchmarkMemoryCacheGetHTTPResponse(b *testing.B) {
	cache := NewMemoryCache()
	value := make([]byte, 2048)

	// Pre-populate with 100 entries
	for i := 0; i < 100; i++ {
		key := string(rune('a' + i))
		cache.Set(key, value)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%100))
		cache.Get(key)
	}
}

// Benchmark with large responses
func BenchmarkMemoryCacheSetLargeResponse(b *testing.B) {
	cache := NewMemoryCache()
	// Large response: 100KB
	value := make([]byte, 100*1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%50))
		cache.Set(key, value)
	}
}

func BenchmarkMemoryCacheGetLargeResponse(b *testing.B) {
	cache := NewMemoryCache()
	value := make([]byte, 100*1024)

	// Pre-populate with 50 entries
	for i := 0; i < 50; i++ {
		key := string(rune('a' + i))
		cache.Set(key, value)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%50))
		cache.Get(key)
	}
}

// Benchmark mixed operations
func BenchmarkMemoryCacheMixedOperations(b *testing.B) {
	cache := NewMemoryCache()
	value := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%100))
		switch i % 3 {
		case 0:
			cache.Set(key, value)
		case 1:
			cache.Get(key)
		case 2:
			cache.Delete(key)
		}
	}
}

// Benchmark concurrent mixed operations
func BenchmarkMemoryCacheParallelMixed(b *testing.B) {
	cache := NewMemoryCache()
	value := make([]byte, 1024)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := string(rune('a' + i%100))
			switch i % 3 {
			case 0:
				cache.Set(key, value)
			case 1:
				cache.Get(key)
			case 2:
				cache.Delete(key)
			}
			i++
		}
	})
}
