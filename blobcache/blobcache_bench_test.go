package blobcache

import (
	"context"
	"fmt"
	"testing"
	"time"

	_ "gocloud.dev/blob/memblob"

	"github.com/sandrolain/httpcache"
)

func setupBenchmarkCache(b *testing.B) (httpcache.Cache, func()) {
	b.Helper()

	ctx := context.Background()
	cache, err := New(ctx, Config{
		BucketURL: "mem://",
		KeyPrefix: "bench/",
		Timeout:   10 * time.Second,
	})
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}

	cleanup := func() {
		if closer, ok := cache.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				b.Logf("Failed to close cache: %v", err)
			}
		}
	}

	return cache, cleanup
}

func BenchmarkBlobCacheSet(b *testing.B) {
	cache, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	data := []byte("benchmark data for set operation")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-set-%d", i)
		cache.Set(key, data)
	}
}

func BenchmarkBlobCacheGet(b *testing.B) {
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

func BenchmarkBlobCacheGetMiss(b *testing.B) {
	cache, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-miss-%d", i)
		cache.Get(key)
	}
}

func BenchmarkBlobCacheDelete(b *testing.B) {
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

func BenchmarkBlobCacheSetGet(b *testing.B) {
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

func BenchmarkBlobCacheSetParallel(b *testing.B) {
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

func BenchmarkBlobCacheGetParallel(b *testing.B) {
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

func BenchmarkBlobCacheMixedParallel(b *testing.B) {
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

func BenchmarkBlobCacheSmallData(b *testing.B) {
	cache, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	data := []byte("small")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-small-%d", i)
		cache.Set(key, data)
	}
}

func BenchmarkBlobCacheLargeData(b *testing.B) {
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
