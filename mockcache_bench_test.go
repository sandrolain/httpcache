package httpcache

import (
	"context"
	"testing"
)

const benchmarkKey = "benchmark-key"

func BenchmarkMockCacheGet(b *testing.B) {
	ctx := context.Background()
	cache := newMockCache()
	value := make([]byte, 1024) // 1KB value
	_ = cache.Set(ctx, benchmarkKey, value)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = cache.Get(ctx, benchmarkKey)
	}
}

func BenchmarkMockCacheSet(b *testing.B) {
	ctx := context.Background()
	cache := newMockCache()
	value := make([]byte, 1024) // 1KB value

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Set(ctx, benchmarkKey, value)
	}
}

func BenchmarkMockCacheDelete(b *testing.B) {
	ctx := context.Background()
	cache := newMockCache()
	value := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%26))
		_ = cache.Set(ctx, key, value)
		_ = cache.Delete(ctx, key)
	}
}

func BenchmarkMockCacheSetGet(b *testing.B) {
	ctx := context.Background()
	cache := newMockCache()
	value := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Set(ctx, benchmarkKey, value)
		_, _, _ = cache.Get(ctx, benchmarkKey)
	}
}

func BenchmarkMockCacheParallelGet(b *testing.B) {
	ctx := context.Background()
	cache := newMockCache()
	value := make([]byte, 1024)

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

func BenchmarkMockCacheParallelSet(b *testing.B) {
	ctx := context.Background()
	cache := newMockCache()
	value := make([]byte, 1024)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := string(rune('a' + i%26))
			_ = cache.Set(ctx, key, value)
			i++
		}
	})
}

// Benchmark with realistic HTTP response sizes
func BenchmarkMockCacheSetHTTPResponse(b *testing.B) {
	ctx := context.Background()
	cache := newMockCache()
	// Typical HTTP response with headers: ~2KB
	value := make([]byte, 2048)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%100))
		_ = cache.Set(ctx, key, value)
	}
}

func BenchmarkMockCacheGetHTTPResponse(b *testing.B) {
	ctx := context.Background()
	cache := newMockCache()
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
func BenchmarkMockCacheSetLargeResponse(b *testing.B) {
	ctx := context.Background()
	cache := newMockCache()
	// Large response: 100KB
	value := make([]byte, 100*1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%50))
		_ = cache.Set(ctx, key, value)
	}
}

func BenchmarkMockCacheGetLargeResponse(b *testing.B) {
	ctx := context.Background()
	cache := newMockCache()
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
func BenchmarkMockCacheMixedOperations(b *testing.B) {
	ctx := context.Background()
	cache := newMockCache()
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

// Benchmark concurrent mixed operations
func BenchmarkMockCacheParallelMixed(b *testing.B) {
	ctx := context.Background()
	cache := newMockCache()
	value := make([]byte, 1024)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := string(rune('a' + i%100))
			switch i % 3 {
			case 0:
				_ = cache.Set(ctx, key, value)
			case 1:
				_, _, _ = cache.Get(ctx, key)
			case 2:
				_ = cache.Delete(ctx, key)
			}
			i++
		}
	})
}
