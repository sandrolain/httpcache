package compresscache

import (
	"compress/gzip"
	"context"
	"strings"
	"testing"
)

func BenchmarkGzip_Set(b *testing.B) {
	ctx := context.Background()
	cache, _ := NewGzip(GzipConfig{
		Cache: newMockCache(),
		Level: gzip.DefaultCompression,
	})

	data := []byte(strings.Repeat("benchmark data ", 100))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = cache.Set(ctx, "key", data)
	}
}

func BenchmarkGzip_Get(b *testing.B) {
	ctx := context.Background()
	cache, _ := NewGzip(GzipConfig{
		Cache: newMockCache(),
		Level: gzip.DefaultCompression,
	})

	data := []byte(strings.Repeat("benchmark data ", 100))
	_ = cache.Set(ctx, "key", data)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, _ = cache.Get(ctx, "key")
	}
}

func BenchmarkBrotli_Set(b *testing.B) {
	ctx := context.Background()
	cache, _ := NewBrotli(BrotliConfig{
		Cache: newMockCache(),
		Level: 6,
	})

	data := []byte(strings.Repeat("benchmark data ", 100))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = cache.Set(ctx, "key", data)
	}
}

func BenchmarkBrotli_Get(b *testing.B) {
	ctx := context.Background()
	cache, _ := NewBrotli(BrotliConfig{
		Cache: newMockCache(),
		Level: 6,
	})

	data := []byte(strings.Repeat("benchmark data ", 100))
	_ = cache.Set(ctx, "key", data)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, _ = cache.Get(ctx, "key")
	}
}

func BenchmarkSnappy_Set(b *testing.B) {
	ctx := context.Background()
	cache, _ := NewSnappy(SnappyConfig{
		Cache: newMockCache(),
	})

	data := []byte(strings.Repeat("benchmark data ", 100))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = cache.Set(ctx, "key", data)
	}
}

func BenchmarkSnappy_Get(b *testing.B) {
	ctx := context.Background()
	cache, _ := NewSnappy(SnappyConfig{
		Cache: newMockCache(),
	})

	data := []byte(strings.Repeat("benchmark data ", 100))
	_ = cache.Set(ctx, "key", data)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, _ = cache.Get(ctx, "key")
	}
}

func BenchmarkGzip_SetGet_Small(b *testing.B) {
	ctx := context.Background()
	cache, _ := NewGzip(GzipConfig{
		Cache: newMockCache(),
		Level: gzip.DefaultCompression,
	})

	data := []byte("small data")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = cache.Set(ctx, "key", data)
		_, _, _ = cache.Get(ctx, "key")
	}
}

func BenchmarkGzip_SetGet_Large(b *testing.B) {
	ctx := context.Background()
	cache, _ := NewGzip(GzipConfig{
		Cache: newMockCache(),
		Level: gzip.DefaultCompression,
	})

	data := []byte(strings.Repeat("large benchmark data ", 1000))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = cache.Set(ctx, "key", data)
		_, _, _ = cache.Get(ctx, "key")
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
			ctx := context.Background()
			cache, _ := NewGzip(GzipConfig{
				Cache: newMockCache(),
				Level: l.level,
			})

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = cache.Set(ctx, "key", data)
				_, _, _ = cache.Get(ctx, "key")
			}
		})
	}
}

func BenchmarkAlgorithmComparison(b *testing.B) {
	data := []byte(strings.Repeat("algorithm comparison benchmark ", 100))

	b.Run("Gzip", func(b *testing.B) {
		ctx := context.Background()
		cache, _ := NewGzip(GzipConfig{
			Cache: newMockCache(),
			Level: gzip.DefaultCompression,
		})
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = cache.Set(ctx, "key", data)
			_, _, _ = cache.Get(ctx, "key")
		}
	})

	b.Run("Brotli", func(b *testing.B) {
		ctx := context.Background()
		cache, _ := NewBrotli(BrotliConfig{
			Cache: newMockCache(),
			Level: 6,
		})
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = cache.Set(ctx, "key", data)
			_, _, _ = cache.Get(ctx, "key")
		}
	})

	b.Run("Snappy", func(b *testing.B) {
		ctx := context.Background()
		cache, _ := NewSnappy(SnappyConfig{
			Cache: newMockCache(),
		})
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = cache.Set(ctx, "key", data)
			_, _, _ = cache.Get(ctx, "key")
		}
	})
}
