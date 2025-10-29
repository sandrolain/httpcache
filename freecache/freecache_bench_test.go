package freecache

import (
	"testing"
)

func BenchmarkSet(b *testing.B) {
	cache := New(256 * 1024 * 1024) // 256MB
	key := "benchmark-key"
	value := make([]byte, 1024) // 1KB value

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(key, value)
	}
}

func BenchmarkGet(b *testing.B) {
	cache := New(256 * 1024 * 1024) // 256MB
	key := "benchmark-key"
	value := make([]byte, 1024) // 1KB value
	cache.Set(key, value)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(key)
	}
}

func BenchmarkSetParallel(b *testing.B) {
	cache := New(256 * 1024 * 1024) // 256MB
	value := make([]byte, 1024)     // 1KB value

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := string(rune('a' + i%26))
			cache.Set(key, value)
			i++
		}
	})
}

func BenchmarkGetParallel(b *testing.B) {
	cache := New(256 * 1024 * 1024) // 256MB
	value := make([]byte, 1024)     // 1KB value

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

// Benchmark with realistic HTTP response sizes
func BenchmarkSetHTTPResponse(b *testing.B) {
	cache := New(256 * 1024 * 1024)
	// Typical HTTP response with headers: ~2KB
	value := make([]byte, 2048)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%100))
		cache.Set(key, value)
	}
}

func BenchmarkGetHTTPResponse(b *testing.B) {
	cache := New(256 * 1024 * 1024)
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
func BenchmarkSetLargeResponse(b *testing.B) {
	cache := New(256 * 1024 * 1024)
	// Large response: 100KB
	value := make([]byte, 100*1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%50))
		cache.Set(key, value)
	}
}

func BenchmarkGetLargeResponse(b *testing.B) {
	cache := New(256 * 1024 * 1024)
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
func BenchmarkMixedOperations(b *testing.B) {
	cache := New(256 * 1024 * 1024)
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
