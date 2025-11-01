package compresscache

import (
	"compress/gzip"
	"strings"
	"testing"

	"github.com/sandrolain/httpcache"
)

func BenchmarkGzip_Set(b *testing.B) {
	cache, _ := NewGzip(GzipConfig{
		Cache: httpcache.NewMemoryCache(),
		Level: gzip.DefaultCompression,
	})

	data := []byte(strings.Repeat("benchmark data ", 100))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cache.Set("key", data)
	}
}

func BenchmarkGzip_Get(b *testing.B) {
	cache, _ := NewGzip(GzipConfig{
		Cache: httpcache.NewMemoryCache(),
		Level: gzip.DefaultCompression,
	})

	data := []byte(strings.Repeat("benchmark data ", 100))
	cache.Set("key", data)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cache.Get("key")
	}
}

func BenchmarkBrotli_Set(b *testing.B) {
	cache, _ := NewBrotli(BrotliConfig{
		Cache: httpcache.NewMemoryCache(),
		Level: 6,
	})

	data := []byte(strings.Repeat("benchmark data ", 100))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cache.Set("key", data)
	}
}

func BenchmarkBrotli_Get(b *testing.B) {
	cache, _ := NewBrotli(BrotliConfig{
		Cache: httpcache.NewMemoryCache(),
		Level: 6,
	})

	data := []byte(strings.Repeat("benchmark data ", 100))
	cache.Set("key", data)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cache.Get("key")
	}
}

func BenchmarkSnappy_Set(b *testing.B) {
	cache, _ := NewSnappy(SnappyConfig{
		Cache: httpcache.NewMemoryCache(),
	})

	data := []byte(strings.Repeat("benchmark data ", 100))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cache.Set("key", data)
	}
}

func BenchmarkSnappy_Get(b *testing.B) {
	cache, _ := NewSnappy(SnappyConfig{
		Cache: httpcache.NewMemoryCache(),
	})

	data := []byte(strings.Repeat("benchmark data ", 100))
	cache.Set("key", data)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cache.Get("key")
	}
}

func BenchmarkGzip_SetGet_Small(b *testing.B) {
	cache, _ := NewGzip(GzipConfig{
		Cache: httpcache.NewMemoryCache(),
		Level: gzip.DefaultCompression,
	})

	data := []byte("small data")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cache.Set("key", data)
		cache.Get("key")
	}
}

func BenchmarkGzip_SetGet_Large(b *testing.B) {
	cache, _ := NewGzip(GzipConfig{
		Cache: httpcache.NewMemoryCache(),
		Level: gzip.DefaultCompression,
	})

	data := []byte(strings.Repeat("large benchmark data ", 1000))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cache.Set("key", data)
		cache.Get("key")
	}
}

func BenchmarkCompressionLevels(b *testing.B) {
	levels := []struct {
		name  string
		level int
	}{
		{"BestSpeed", gzip.BestSpeed},
		{"Default", gzip.DefaultCompression},
		{"BestCompression", gzip.BestCompression},
	}

	data := []byte(strings.Repeat("compression level benchmark ", 100))

	for _, l := range levels {
		b.Run(l.name, func(b *testing.B) {
			cache, _ := NewGzip(GzipConfig{
				Cache: httpcache.NewMemoryCache(),
				Level: l.level,
			})

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cache.Set("key", data)
				cache.Get("key")
			}
		})
	}
}

func BenchmarkAlgorithmComparison(b *testing.B) {
	data := []byte(strings.Repeat("algorithm comparison benchmark ", 100))

	b.Run("Gzip", func(b *testing.B) {
		cache, _ := NewGzip(GzipConfig{
			Cache: httpcache.NewMemoryCache(),
			Level: gzip.DefaultCompression,
		})
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.Set("key", data)
			cache.Get("key")
		}
	})

	b.Run("Brotli", func(b *testing.B) {
		cache, _ := NewBrotli(BrotliConfig{
			Cache: httpcache.NewMemoryCache(),
			Level: 6,
		})
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.Set("key", data)
			cache.Get("key")
		}
	})

	b.Run("Snappy", func(b *testing.B) {
		cache, _ := NewSnappy(SnappyConfig{
			Cache: httpcache.NewMemoryCache(),
		})
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.Set("key", data)
			cache.Get("key")
		}
	})
}
