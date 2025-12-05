package freecache

import (
	"context"
	"testing"
)

func BenchmarkSet(b *testing.B) {
	cache := New(256 * 1024 * 1024) // 256MB
	ctx := context.Background()
	key := "benchmark-key"
	value := make([]byte, 1024) // 1KB value

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Set(ctx, key, value)
	}
}

func BenchmarkGet(b *testing.B) {
	cache := New(256 * 1024 * 1024) // 256MB
	ctx := context.Background()
	key := "benchmark-key"
	value := make([]byte, 1024) // 1KB value
	_ = cache.Set(ctx, key, value)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = cache.Get(ctx, key)
	}
}

func BenchmarkSetParallel(b *testing.B) {
	cache := New(256 * 1024 * 1024) // 256MB
	ctx := context.Background()
	value := make([]byte, 1024) // 1KB value

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := string(rune('a' + i%26))
			_ = cache.Set(ctx, key, value)
			i++
		}
	})
}

func BenchmarkGetParallel(b *testing.B) {
	cache := New(256 * 1024 * 1024) // 256MB
	ctx := context.Background()
	value := make([]byte, 1024) // 1KB value

	// Pre-populate cache
	for i := 0; i < 26; i++ {
		key := string(rune('a' + i))
		_ = cache.Set(ctx, key, value)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := string(rune('a' + i%26))
			_, _, _ = cache.Get(ctx, key)
			i++
		}
	})
}

// Benchmark with realistic HTTP response sizes
func BenchmarkSetHTTPResponse(b *testing.B) {
	cache := New(256 * 1024 * 1024)
	ctx := context.Background()
	// Typical HTTP response with headers: ~2KB
	value := make([]byte, 2048)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%100))
		_ = cache.Set(ctx, key, value)
	}
}

func BenchmarkGetHTTPResponse(b *testing.B) {
	cache := New(256 * 1024 * 1024)
	ctx := context.Background()
	value := make([]byte, 2048)

	// Pre-populate with 100 entries
	for i := 0; i < 100; i++ {
		key := string(rune('a' + i))
		_ = cache.Set(ctx, key, value)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%100))
		_, _, _ = cache.Get(ctx, key)
	}
}

// Benchmark with large responses
func BenchmarkSetLargeResponse(b *testing.B) {
	cache := New(256 * 1024 * 1024)
	ctx := context.Background()
	// Large response: 100KB
	value := make([]byte, 100*1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%50))
		_ = cache.Set(ctx, key, value)
	}
}

func BenchmarkGetLargeResponse(b *testing.B) {
	cache := New(256 * 1024 * 1024)
	ctx := context.Background()
	value := make([]byte, 100*1024)

	// Pre-populate with 50 entries
	for i := 0; i < 50; i++ {
		key := string(rune('a' + i))
		_ = cache.Set(ctx, key, value)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%50))
		_, _, _ = cache.Get(ctx, key)
	}
}

// Benchmark mixed operations
func BenchmarkMixedOperations(b *testing.B) {
	cache := New(256 * 1024 * 1024)
	ctx := context.Background()
	value := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%100))
		switch i % 3 {
		case 0:
			_ = cache.Set(ctx, key, value)
		case 1:
			_, _, _ = cache.Get(ctx, key)
		case 2:
			_ = cache.Delete(ctx, key)
		}
	}
}
