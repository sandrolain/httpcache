package compresscache

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"

	"github.com/sandrolain/httpcache"
)

// mockCache is a simple in-memory cache for testing
type mockCache struct {
	data map[string][]byte
}

func newMockCache() *mockCache {
	return &mockCache{
		data: make(map[string][]byte),
	}
}

func (m *mockCache) Get(key string) ([]byte, bool) {
	val, ok := m.data[key]
	return val, ok
}

func (m *mockCache) Set(key string, value []byte) {
	m.data[key] = value
}

func (m *mockCache) Delete(key string) {
	delete(m.data, key)
}

func TestNewGzip(t *testing.T) {
	tests := []struct {
		name    string
		config  GzipConfig
		wantErr bool
	}{
		{
			name: "valid config with default level",
			config: GzipConfig{
				Cache: newMockCache(),
			},
			wantErr: false,
		},
		{
			name: "valid config with custom level",
			config: GzipConfig{
				Cache: newMockCache(),
				Level: gzip.BestCompression,
			},
			wantErr: false,
		},
		{
			name: "nil cache",
			config: GzipConfig{
				Cache: nil,
			},
			wantErr: true,
		},
		{
			name: "invalid compression level too high",
			config: GzipConfig{
				Cache: newMockCache(),
				Level: 100,
			},
			wantErr: true,
		},
		{
			name: "invalid compression level too low",
			config: GzipConfig{
				Cache: newMockCache(),
				Level: -10,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := NewGzip(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGzip() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && cache == nil {
				t.Error("NewGzip() returned nil cache without error")
			}
			if !tt.wantErr && cache.algorithm != Gzip {
				t.Errorf("NewGzip() algorithm = %v, want %v", cache.algorithm, Gzip)
			}
		})
	}
}

func TestNewBrotli(t *testing.T) {
	tests := []struct {
		name    string
		config  BrotliConfig
		wantErr bool
	}{
		{
			name: "valid config with default level",
			config: BrotliConfig{
				Cache: newMockCache(),
			},
			wantErr: false,
		},
		{
			name: "valid config with custom level",
			config: BrotliConfig{
				Cache: newMockCache(),
				Level: 11,
			},
			wantErr: false,
		},
		{
			name: "nil cache",
			config: BrotliConfig{
				Cache: nil,
			},
			wantErr: true,
		},
		{
			name: "invalid compression level",
			config: BrotliConfig{
				Cache: newMockCache(),
				Level: 20,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := NewBrotli(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBrotli() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && cache == nil {
				t.Error("NewBrotli() returned nil cache without error")
			}
			if !tt.wantErr && cache.algorithm != Brotli {
				t.Errorf("NewBrotli() algorithm = %v, want %v", cache.algorithm, Brotli)
			}
		})
	}
}

func TestNewSnappy(t *testing.T) {
	tests := []struct {
		name    string
		config  SnappyConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: SnappyConfig{
				Cache: newMockCache(),
			},
			wantErr: false,
		},
		{
			name: "nil cache",
			config: SnappyConfig{
				Cache: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := NewSnappy(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSnappy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && cache == nil {
				t.Error("NewSnappy() returned nil cache without error")
			}
			if !tt.wantErr && cache.algorithm != Snappy {
				t.Errorf("NewSnappy() algorithm = %v, want %v", cache.algorithm, Snappy)
			}
		})
	}
}

func TestSetGet_Gzip(t *testing.T) {
	mock := newMockCache()
	cache, err := NewGzip(GzipConfig{
		Cache: mock,
		Level: gzip.DefaultCompression,
	})
	if err != nil {
		t.Fatalf("NewGzip() failed: %v", err)
	}

	testData := []byte(strings.Repeat("Gzip compression test. ", 100))
	key := "gzip-key"

	cache.Set(key, testData)
	retrieved, ok := cache.Get(key)
	if !ok {
		t.Fatal("Get() returned false")
	}

	if !bytes.Equal(retrieved, testData) {
		t.Error("Retrieved data doesn't match original")
	}

	stats := cache.Stats()
	if stats.CompressedCount != 1 {
		t.Errorf("Expected 1 compressed entry, got %d", stats.CompressedCount)
	}
	if stats.UncompressedBytes == 0 {
		t.Error("UncompressedBytes should not be zero")
	}
	if stats.CompressedBytes == 0 {
		t.Error("CompressedBytes should not be zero")
	}
}

func TestSetGet_Brotli(t *testing.T) {
	mock := newMockCache()
	cache, err := NewBrotli(BrotliConfig{
		Cache: mock,
		Level: 6,
	})
	if err != nil {
		t.Fatalf("NewBrotli() failed: %v", err)
	}

	testData := []byte(strings.Repeat("Brotli compression test. ", 50))
	key := "brotli-key"

	cache.Set(key, testData)
	retrieved, ok := cache.Get(key)
	if !ok {
		t.Fatal("Get() returned false")
	}

	if !bytes.Equal(retrieved, testData) {
		t.Error("Retrieved data doesn't match original")
	}

	stats := cache.Stats()
	if stats.CompressedCount != 1 {
		t.Errorf("Expected 1 compressed entry, got %d", stats.CompressedCount)
	}
}

func TestSetGet_Snappy(t *testing.T) {
	cache, err := NewSnappy(SnappyConfig{
		Cache: newMockCache(),
	})
	if err != nil {
		t.Fatalf("NewSnappy() failed: %v", err)
	}

	testData := []byte(strings.Repeat("Snappy fast compression! ", 40))
	key := "snappy-key"

	cache.Set(key, testData)
	retrieved, ok := cache.Get(key)
	if !ok {
		t.Fatal("Get() returned false")
	}

	if !bytes.Equal(retrieved, testData) {
		t.Error("Retrieved data doesn't match original")
	}

	stats := cache.Stats()
	if stats.CompressedCount != 1 {
		t.Errorf("Expected 1 compressed entry, got %d", stats.CompressedCount)
	}
}

func TestSetGet_SmallData(t *testing.T) {
	cache, err := NewGzip(GzipConfig{
		Cache: newMockCache(),
	})
	if err != nil {
		t.Fatalf("NewGzip() failed: %v", err)
	}

	// Small data - compression will still be attempted
	smallData := []byte("small")
	cache.Set("small", smallData)

	retrieved, ok := cache.Get("small")
	if !ok {
		t.Fatal("Get() returned false")
	}

	if !bytes.Equal(retrieved, smallData) {
		t.Error("Small data retrieval failed")
	}

	// Verify it was compressed (even small data)
	stats := cache.Stats()
	if stats.CompressedCount != 1 {
		t.Errorf("Expected 1 compressed entry, got %d", stats.CompressedCount)
	}
}

func TestDelete(t *testing.T) {
	cache, err := NewGzip(GzipConfig{
		Cache: newMockCache(),
	})
	if err != nil {
		t.Fatalf("NewGzip() failed: %v", err)
	}

	testData := []byte(strings.Repeat("Delete test ", 10))
	cache.Set("key", testData)

	// Verify it exists
	_, ok := cache.Get("key")
	if !ok {
		t.Fatal("Data should exist before delete")
	}

	// Delete
	cache.Delete("key")

	// Verify it's gone
	_, ok = cache.Get("key")
	if ok {
		t.Error("Data should not exist after delete")
	}
}

func TestStats(t *testing.T) {
	cache, err := NewGzip(GzipConfig{
		Cache: newMockCache(),
		Level: gzip.BestCompression,
	})
	if err != nil {
		t.Fatalf("NewGzip() failed: %v", err)
	}

	// Add multiple entries
	for i := 0; i < 5; i++ {
		data := []byte(strings.Repeat("Data entry ", 20))
		cache.Set(string(rune('a'+i)), data)
	}

	stats := cache.Stats()

	if stats.CompressedCount != 5 {
		t.Errorf("Expected 5 compressed entries, got %d", stats.CompressedCount)
	}

	if stats.UncompressedBytes == 0 {
		t.Error("UncompressedBytes should not be zero")
	}

	if stats.CompressedBytes == 0 {
		t.Error("CompressedBytes should not be zero")
	}

	if stats.CompressedBytes >= stats.UncompressedBytes {
		t.Errorf("CompressedBytes (%d) should be less than UncompressedBytes (%d)",
			stats.CompressedBytes, stats.UncompressedBytes)
	}

	if stats.CompressionRatio >= 1.0 {
		t.Errorf("CompressionRatio should be < 1.0, got %.2f", stats.CompressionRatio)
	}

	if stats.SavingsPercent <= 0 || stats.SavingsPercent >= 100 {
		t.Errorf("SavingsPercent should be between 0 and 100, got %.2f", stats.SavingsPercent)
	}
}

func TestMixedAlgorithms(t *testing.T) {
	// Test that we can read data compressed with different algorithms
	mock := newMockCache()

	// Store with gzip
	gzipCache, _ := NewGzip(GzipConfig{
		Cache: mock,
	})
	gzipData := []byte(strings.Repeat("Gzip data ", 10))
	gzipCache.Set("gzip-key", gzipData)

	// Store with brotli
	brotliCache, _ := NewBrotli(BrotliConfig{
		Cache: mock,
	})
	brotliData := []byte(strings.Repeat("Brotli data ", 10))
	brotliCache.Set("brotli-key", brotliData)

	// Store with snappy
	snappyCache, _ := NewSnappy(SnappyConfig{
		Cache: mock,
	})
	snappyData := []byte(strings.Repeat("Snappy data ", 10))
	snappyCache.Set("snappy-key", snappyData)

	// Each cache should be able to read its own data
	retrieved, ok := gzipCache.Get("gzip-key")
	if !ok || !bytes.Equal(retrieved, gzipData) {
		t.Error("Gzip cache failed to retrieve gzip data")
	}

	retrieved, ok = brotliCache.Get("brotli-key")
	if !ok || !bytes.Equal(retrieved, brotliData) {
		t.Error("Brotli cache failed to retrieve brotli data")
	}

	retrieved, ok = snappyCache.Get("snappy-key")
	if !ok || !bytes.Equal(retrieved, snappyData) {
		t.Error("Snappy cache failed to retrieve snappy data")
	}

	// Each cache can read data compressed with other algorithms
	// because the marker indicates which algorithm was used
	retrieved, ok = brotliCache.Get("gzip-key")
	if !ok || !bytes.Equal(retrieved, gzipData) {
		t.Error("Brotli cache failed to retrieve gzip-compressed data")
	}

	retrieved, ok = snappyCache.Get("brotli-key")
	if !ok || !bytes.Equal(retrieved, brotliData) {
		t.Error("Snappy cache failed to retrieve brotli-compressed data")
	}

	retrieved, ok = gzipCache.Get("snappy-key")
	if !ok || !bytes.Equal(retrieved, snappyData) {
		t.Error("Gzip cache failed to retrieve snappy-compressed data")
	}
}

func TestAlgorithm_String(t *testing.T) {
	tests := []struct {
		algo Algorithm
		want string
	}{
		{Gzip, "gzip"},
		{Brotli, "brotli"},
		{Snappy, "snappy"},
		{Algorithm(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.algo.String(); got != tt.want {
				t.Errorf("Algorithm.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetNonExistent(t *testing.T) {
	cache, err := NewGzip(GzipConfig{
		Cache: newMockCache(),
	})
	if err != nil {
		t.Fatalf("NewGzip() failed: %v", err)
	}

	_, ok := cache.Get("nonexistent")
	if ok {
		t.Error("Get() should return false for non-existent key")
	}
}

func TestGetEmptyData(t *testing.T) {
	mock := newMockCache()
	cache, err := NewGzip(GzipConfig{
		Cache: mock,
	})
	if err != nil {
		t.Fatalf("NewGzip() failed: %v", err)
	}

	// Set empty data directly in mock cache
	mock.Set("empty", []byte{})

	data, ok := cache.Get("empty")
	if !ok {
		t.Error("Get() should return true for empty data")
	}
	if len(data) != 0 {
		t.Errorf("Expected empty data, got %d bytes", len(data))
	}
}

func TestIntegration(t *testing.T) {
	// Integration test with real MemoryCache
	cache, err := NewGzip(GzipConfig{
		Cache: httpcache.NewMemoryCache(),
		Level: gzip.DefaultCompression,
	})
	if err != nil {
		t.Fatalf("NewGzip() failed: %v", err)
	}

	// Simulate HTTP response caching
	testData := []byte(`{
		"users": [
			{"id": 1, "name": "Alice", "email": "alice@example.com"},
			{"id": 2, "name": "Bob", "email": "bob@example.com"},
			{"id": 3, "name": "Charlie", "email": "charlie@example.com"}
		]
	}`)

	cache.Set("https://api.example.com/users", testData)

	retrieved, ok := cache.Get("https://api.example.com/users")
	if !ok {
		t.Fatal("Failed to retrieve cached data")
	}

	if !bytes.Equal(retrieved, testData) {
		t.Error("Retrieved data doesn't match original")
	}

	stats := cache.Stats()
	t.Logf("Compression stats: %.2f%% savings, ratio: %.2f",
		stats.SavingsPercent, stats.CompressionRatio)

	if stats.SavingsPercent <= 0 {
		t.Error("Expected some compression savings")
	}
}

func TestCorruptedData(t *testing.T) {
	mock := newMockCache()
	cache, err := NewGzip(GzipConfig{
		Cache: mock,
	})
	if err != nil {
		t.Fatalf("NewGzip() failed: %v", err)
	}

	// Store corrupted data with gzip marker but invalid compressed data
	mock.Set("corrupted", []byte{byte(Gzip + 1), 0xFF, 0xFF, 0xFF})

	_, ok := cache.Get("corrupted")
	if ok {
		t.Error("Get() should return false for corrupted data")
	}
}

func TestUncompressedData(t *testing.T) {
	mock := newMockCache()
	cache, err := NewGzip(GzipConfig{
		Cache: mock,
	})
	if err != nil {
		t.Fatalf("NewGzip() failed: %v", err)
	}

	// Store uncompressed data with marker 0
	testData := []byte("uncompressed test data")
	data := make([]byte, len(testData)+1)
	data[0] = 0
	copy(data[1:], testData)
	mock.Set("uncompressed", data)

	retrieved, ok := cache.Get("uncompressed")
	if !ok {
		t.Fatal("Get() should return true for uncompressed data")
	}

	if !bytes.Equal(retrieved, testData) {
		t.Error("Retrieved uncompressed data doesn't match original")
	}
}

func TestCompressionLevels(t *testing.T) {
	levels := []int{
		gzip.BestSpeed,
		gzip.DefaultCompression,
		gzip.BestCompression,
	}

	testData := []byte(strings.Repeat("compression level test ", 50))

	for _, level := range levels {
		t.Run(string(rune('0'+level)), func(t *testing.T) {
			cache, err := NewGzip(GzipConfig{
				Cache: newMockCache(),
				Level: level,
			})
			if err != nil {
				t.Fatalf("NewGzip() failed for level %d: %v", level, err)
			}

			cache.Set("key", testData)
			retrieved, ok := cache.Get("key")
			if !ok {
				t.Fatal("Get() returned false")
			}

			if !bytes.Equal(retrieved, testData) {
				t.Error("Retrieved data doesn't match original")
			}
		})
	}
}

func TestBrotliLevels(t *testing.T) {
	levels := []int{0, 6, 11}
	testData := []byte(strings.Repeat("brotli level test ", 50))

	for _, level := range levels {
		t.Run(string(rune('0'+level)), func(t *testing.T) {
			cache, err := NewBrotli(BrotliConfig{
				Cache: newMockCache(),
				Level: level,
			})
			if err != nil {
				t.Fatalf("NewBrotli() failed for level %d: %v", level, err)
			}

			cache.Set("key", testData)
			retrieved, ok := cache.Get("key")
			if !ok {
				t.Fatal("Get() returned false")
			}

			if !bytes.Equal(retrieved, testData) {
				t.Error("Retrieved data doesn't match original")
			}
		})
	}
}

func TestAllAlgorithmsRoundTrip(t *testing.T) {
	testData := []byte(strings.Repeat("round trip test ", 100))

	t.Run("Gzip", func(t *testing.T) {
		cache, _ := NewGzip(GzipConfig{Cache: newMockCache()})
		cache.Set("key", testData)
		retrieved, ok := cache.Get("key")
		if !ok || !bytes.Equal(retrieved, testData) {
			t.Error("Gzip round trip failed")
		}
	})

	t.Run("Brotli", func(t *testing.T) {
		cache, _ := NewBrotli(BrotliConfig{Cache: newMockCache()})
		cache.Set("key", testData)
		retrieved, ok := cache.Get("key")
		if !ok || !bytes.Equal(retrieved, testData) {
			t.Error("Brotli round trip failed")
		}
	})

	t.Run("Snappy", func(t *testing.T) {
		cache, _ := NewSnappy(SnappyConfig{Cache: newMockCache()})
		cache.Set("key", testData)
		retrieved, ok := cache.Get("key")
		if !ok || !bytes.Equal(retrieved, testData) {
			t.Error("Snappy round trip failed")
		}
	})
}

func TestEmptyValue(t *testing.T) {
	cache, _ := NewGzip(GzipConfig{Cache: newMockCache()})

	// Set and get empty value
	cache.Set("empty", []byte{})
	retrieved, ok := cache.Get("empty")
	if !ok {
		t.Error("Get() should return true for empty value")
	}
	if len(retrieved) != 0 {
		t.Errorf("Expected empty value, got %d bytes", len(retrieved))
	}
}

func TestStatsEmptyCache(t *testing.T) {
	cache, _ := NewGzip(GzipConfig{Cache: newMockCache()})

	stats := cache.Stats()
	if stats.CompressedCount != 0 {
		t.Errorf("Expected 0 compressed count, got %d", stats.CompressedCount)
	}
	if stats.UncompressedCount != 0 {
		t.Errorf("Expected 0 uncompressed count, got %d", stats.UncompressedCount)
	}
	if stats.CompressionRatio != 0 {
		t.Errorf("Expected 0 compression ratio, got %.2f", stats.CompressionRatio)
	}
}

func TestMultipleSetSameKey(t *testing.T) {
	cache, _ := NewGzip(GzipConfig{Cache: newMockCache()})

	// Set value multiple times
	for i := 0; i < 3; i++ {
		data := []byte(strings.Repeat("iteration ", i+1))
		cache.Set("key", data)
	}

	// Should have the last value
	retrieved, ok := cache.Get("key")
	if !ok {
		t.Fatal("Get() returned false")
	}

	expected := []byte(strings.Repeat("iteration ", 3))
	if !bytes.Equal(retrieved, expected) {
		t.Error("Retrieved data doesn't match last set value")
	}

	// Stats should reflect all operations
	stats := cache.Stats()
	if stats.CompressedCount != 3 {
		t.Errorf("Expected 3 compressed operations, got %d", stats.CompressedCount)
	}
}

func TestBrotliCorruptedData(t *testing.T) {
	mock := newMockCache()
	cache, _ := NewBrotli(BrotliConfig{Cache: mock})

	// Store corrupted brotli data
	mock.Set("corrupted", []byte{byte(Brotli + 1), 0xFF, 0xFF, 0xFF})

	_, ok := cache.Get("corrupted")
	if ok {
		t.Error("Get() should return false for corrupted brotli data")
	}
}

func TestSnappyCorruptedData(t *testing.T) {
	mock := newMockCache()
	cache, _ := NewSnappy(SnappyConfig{Cache: mock})

	// Store corrupted snappy data
	mock.Set("corrupted", []byte{byte(Snappy + 1), 0xFF, 0xFF, 0xFF})

	_, ok := cache.Get("corrupted")
	if ok {
		t.Error("Get() should return false for corrupted snappy data")
	}
}
