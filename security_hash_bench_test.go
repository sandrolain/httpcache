// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

// oldHashKey is the previous implementation using hex encoding (for comparison)
func oldHashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// BenchmarkHashKey tests the optimized hash function with pooling and base64
func BenchmarkHashKey(b *testing.B) {
	testCases := []struct {
		name string
		key  string
	}{
		{"Short", "GET:https://example.com/api/users"},
		{"Medium", "GET:https://example.com/api/users?page=1&limit=50&sort=created_at&order=desc"},
		{"Long", "GET:https://example.com/api/data?query=very_long_parameter_string_that_represents_a_complex_query&filter=multiple_conditions&sort=various_fields&limit=100&offset=0&include=relations"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = hashKey(tc.key)
			}
		})
	}
}

// BenchmarkOldHashKey tests the old hex-based implementation (for comparison)
func BenchmarkOldHashKey(b *testing.B) {
	testCases := []struct {
		name string
		key  string
	}{
		{"Short", "GET:https://example.com/api/users"},
		{"Medium", "GET:https://example.com/api/users?page=1&limit=50&sort=created_at&order=desc"},
		{"Long", "GET:https://example.com/api/data?query=very_long_parameter_string_that_represents_a_complex_query&filter=multiple_conditions&sort=various_fields&limit=100&offset=0&include=relations"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = oldHashKey(tc.key)
			}
		})
	}
}

// BenchmarkHashKeyParallel tests concurrent usage with pooling
func BenchmarkHashKeyParallel(b *testing.B) {
	key := "GET:https://example.com/api/users?page=1&limit=50"
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = hashKey(key)
		}
	})
}

// BenchmarkOldHashKeyParallel tests concurrent usage without pooling
func BenchmarkOldHashKeyParallel(b *testing.B) {
	key := "GET:https://example.com/api/users?page=1&limit=50"
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = oldHashKey(key)
		}
	})
}

// BenchmarkHashKeyCollisions tests that hashing produces unique values
func BenchmarkHashKeyCollisions(b *testing.B) {
	seen := make(map[string]bool)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("GET:https://example.com/api/item/%d", i)
		hash := hashKey(key)
		if seen[hash] {
			b.Fatalf("Hash collision detected for key: %s", key)
		}
		seen[hash] = true
	}
}

// BenchmarkHashKeyOutputSize compares output sizes
func BenchmarkHashKeyOutputSize(b *testing.B) {
	key := "GET:https://example.com/api/users"

	newHash := hashKey(key)
	oldHash := oldHashKey(key)
	xxHash := hashKeyXXHash(key)

	b.Logf("New hash (base64):  %s (length: %d)", newHash, len(newHash))
	b.Logf("Old hash (hex):     %s (length: %d)", oldHash, len(oldHash))
	b.Logf("xxHash (base36):    %s (length: %d)", xxHash, len(xxHash))
	b.Logf("SHA256 size reduction vs hex: %.1f%%", float64(len(oldHash)-len(newHash))/float64(len(oldHash))*100)
	b.Logf("xxHash size reduction vs hex: %.1f%%", float64(len(oldHash)-len(xxHash))/float64(len(oldHash))*100)
	b.Logf("xxHash size reduction vs SHA256: %.1f%%", float64(len(newHash)-len(xxHash))/float64(len(newHash))*100)
}

// BenchmarkHashKeyXXHash tests the xxHash implementation
func BenchmarkHashKeyXXHash(b *testing.B) {
	testCases := []struct {
		name string
		key  string
	}{
		{"Short", "GET:https://example.com/api/users"},
		{"Medium", "GET:https://example.com/api/users?page=1&limit=50&sort=created_at&order=desc"},
		{"Long", "GET:https://example.com/api/data?query=very_long_parameter_string_that_represents_a_complex_query&filter=multiple_conditions&sort=various_fields&limit=100&offset=0&include=relations"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = hashKeyXXHash(tc.key)
			}
		})
	}
}

// BenchmarkHashKeyXXHashParallel tests concurrent xxHash usage
func BenchmarkHashKeyXXHashParallel(b *testing.B) {
	key := "GET:https://example.com/api/users?page=1&limit=50"
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = hashKeyXXHash(key)
		}
	})
}

// BenchmarkHashAlgorithmComparison compares all three algorithms side by side
func BenchmarkHashAlgorithmComparison(b *testing.B) {
	key := "GET:https://example.com/api/users?page=1&limit=50&sort=created_at"

	b.Run("SHA256-Hex-Old", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = oldHashKey(key)
		}
	})

	b.Run("SHA256-Base64-Pooled", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = hashKey(key)
		}
	})

	b.Run("XXHash-Base36", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = hashKeyXXHash(key)
		}
	})
}
