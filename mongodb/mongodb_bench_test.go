package mongodb

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/sandrolain/httpcache"
)

func setupBenchmarkCache(b *testing.B) (httpcache.Cache, func()) {
	b.Helper()

	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}

	config := Config{
		URI:        uri,
		Database:   "httpcache_bench",
		Collection: "cache_bench",
		Timeout:    10 * time.Second,
	}

	ctx := context.Background()
	cache, err := New(ctx, config)
	if err != nil {
		b.Skipf("MongoDB unavailable: %v", err)
	}

	cleanup := func() {
		if c, ok := cache.(interface{ Close() error }); ok {
			if err := c.Close(); err != nil {
				b.Logf("Failed to close cache: %v", err)
			}
		}
	}

	return cache, cleanup
}

func BenchmarkMongoDBCacheSet(b *testing.B) {
	cache, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	data := []byte("benchmark data for set operation")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-set-%d", i)
		cache.Set(key, data)
	}
}

func BenchmarkMongoDBCacheGet(b *testing.B) {
	cache, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	// Pre-populate cache
	data := []byte("benchmark data for get operation")
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("bench-get-%d", i)
		cache.Set(key, data)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-get-%d", i%100)
		cache.Get(key)
	}
}

func BenchmarkMongoDBCacheGetMiss(b *testing.B) {
	cache, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-miss-%d", i)
		cache.Get(key)
	}
}

func BenchmarkMongoDBCacheDelete(b *testing.B) {
	cache, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	// Pre-populate cache
	data := []byte("benchmark data for delete operation")
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-delete-%d", i)
		cache.Set(key, data)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-delete-%d", i)
		cache.Delete(key)
	}
}

func BenchmarkMongoDBCacheSetGet(b *testing.B) {
	cache, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	data := []byte("benchmark data for set-get operation")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-setget-%d", i)
		cache.Set(key, data)
		cache.Get(key)
	}
}

func BenchmarkMongoDBCacheSetParallel(b *testing.B) {
	cache, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	data := []byte("benchmark data for parallel set")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench-parallel-set-%d", i)
			cache.Set(key, data)
			i++
		}
	})
}

func BenchmarkMongoDBCacheGetParallel(b *testing.B) {
	cache, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	// Pre-populate cache
	data := []byte("benchmark data for parallel get")
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("bench-parallel-get-%d", i)
		cache.Set(key, data)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench-parallel-get-%d", i%100)
			cache.Get(key)
			i++
		}
	})
}

func BenchmarkMongoDBCacheMixedParallel(b *testing.B) {
	cache, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	data := []byte("benchmark data for mixed operations")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench-mixed-%d", i%100)
			switch i % 3 {
			case 0:
				cache.Set(key, data)
			case 1:
				cache.Get(key)
			default:
				cache.Delete(key)
			}
			i++
		}
	})
}

func BenchmarkMongoDBCacheSmallData(b *testing.B) {
	cache, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	data := []byte("small")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-small-%d", i)
		cache.Set(key, data)
	}
}

func BenchmarkMongoDBCacheLargeData(b *testing.B) {
	cache, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	// 10KB of data
	data := make([]byte, 10*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-large-%d", i)
		cache.Set(key, data)
	}
}
