package compresscache

import (
	"bytes"
	"compress/gzip"
	"context"
	"strings"
	"testing"
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

func (m *mockCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	val, ok := m.data[key]
	return val, ok, nil
}

func (m *mockCache) Set(_ context.Context, key string, value []byte) error {
	m.data[key] = value
	return nil
}

func (m *mockCache) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
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
	ctx := context.Background()
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

	if err := cache.Set(ctx, key, testData); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}
	retrieved, ok, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
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
	ctx := context.Background()
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

	if err := cache.Set(ctx, key, testData); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}
	retrieved, ok, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
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
	ctx := context.Background()
	cache, err := NewSnappy(SnappyConfig{
		Cache: newMockCache(),
	})
	if err != nil {
		t.Fatalf("NewSnappy() failed: %v", err)
	}

	testData := []byte(strings.Repeat("Snappy fast compression! ", 40))
	key := "snappy-key"

	if err := cache.Set(ctx, key, testData); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}
	retrieved, ok, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
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
	ctx := context.Background()
	cache, err := NewGzip(GzipConfig{
		Cache: newMockCache(),
	})
	if err != nil {
		t.Fatalf("NewGzip() failed: %v", err)
	}

	// Small data - compression will still be attempted
	smallData := []byte("small")
	if err := cache.Set(ctx, "small", smallData); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	retrieved, ok, err := cache.Get(ctx, "small")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
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
	ctx := context.Background()
	cache, err := NewGzip(GzipConfig{
		Cache: newMockCache(),
	})
	if err != nil {
		t.Fatalf("NewGzip() failed: %v", err)
	}

	testData := []byte(strings.Repeat("Delete test ", 10))
	if err := cache.Set(ctx, "key", testData); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// Verify it exists
	_, ok, err := cache.Get(ctx, "key")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if !ok {
		t.Fatal("Data should exist before delete")
	}

	// Delete
	if err := cache.Delete(ctx, "key"); err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	// Verify it's gone
	_, ok, err = cache.Get(ctx, "key")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if ok {
		t.Error("Data should not exist after delete")
	}
}

func TestStats(t *testing.T) {
	ctx := context.Background()
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
		if err := cache.Set(ctx, string(rune('a'+i)), data); err != nil {
			t.Fatalf("Set() failed: %v", err)
		}
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
	ctx := context.Background()
	// Test that we can read data compressed with different algorithms
	mock := newMockCache()

	// Store with gzip
	gzipCache, _ := NewGzip(GzipConfig{
		Cache: mock,
	})
	gzipData := []byte(strings.Repeat("Gzip data ", 10))
	_ = gzipCache.Set(ctx, "gzip-key", gzipData)

	// Store with brotli
	brotliCache, _ := NewBrotli(BrotliConfig{
		Cache: mock,
	})
	brotliData := []byte(strings.Repeat("Brotli data ", 10))
	_ = brotliCache.Set(ctx, "brotli-key", brotliData)

	// Store with snappy
	snappyCache, _ := NewSnappy(SnappyConfig{
		Cache: mock,
	})
	snappyData := []byte(strings.Repeat("Snappy data ", 10))
	_ = snappyCache.Set(ctx, "snappy-key", snappyData)

	// Each cache should be able to read its own data
	retrieved, ok, _ := gzipCache.Get(ctx, "gzip-key")
	if !ok || !bytes.Equal(retrieved, gzipData) {
		t.Error("Gzip cache failed to retrieve gzip data")
	}

	retrieved, ok, _ = brotliCache.Get(ctx, "brotli-key")
	if !ok || !bytes.Equal(retrieved, brotliData) {
		t.Error("Brotli cache failed to retrieve brotli data")
	}

	retrieved, ok, _ = snappyCache.Get(ctx, "snappy-key")
	if !ok || !bytes.Equal(retrieved, snappyData) {
		t.Error("Snappy cache failed to retrieve snappy data")
	}

	// Each cache can read data compressed with other algorithms
	// because the marker indicates which algorithm was used
	retrieved, ok, _ = brotliCache.Get(ctx, "gzip-key")
	if !ok || !bytes.Equal(retrieved, gzipData) {
		t.Error("Brotli cache failed to retrieve gzip-compressed data")
	}

	retrieved, ok, _ = snappyCache.Get(ctx, "brotli-key")
	if !ok || !bytes.Equal(retrieved, brotliData) {
		t.Error("Snappy cache failed to retrieve brotli-compressed data")
	}

	retrieved, ok, _ = gzipCache.Get(ctx, "snappy-key")
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
	ctx := context.Background()
	cache, err := NewGzip(GzipConfig{
		Cache: newMockCache(),
	})
	if err != nil {
		t.Fatalf("NewGzip() failed: %v", err)
	}

	_, ok, err := cache.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if ok {
		t.Error("Get() should return false for non-existent key")
	}
}

func TestGetEmptyData(t *testing.T) {
	ctx := context.Background()
	mock := newMockCache()
	cache, err := NewGzip(GzipConfig{
		Cache: mock,
	})
	if err != nil {
		t.Fatalf("NewGzip() failed: %v", err)
	}

	// Set empty data directly in mock cache
	_ = mock.Set(ctx, "empty", []byte{})

	data, ok, err := cache.Get(ctx, "empty")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if !ok {
		t.Error("Get() should return true for empty data")
	}
	if len(data) != 0 {
		t.Errorf("Expected empty data, got %d bytes", len(data))
	}
}

func TestIntegration(t *testing.T) {
	ctx := context.Background()
	// Integration test with mockCache
	cache, err := NewGzip(GzipConfig{
		Cache: newMockCache(),
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

	if err := cache.Set(ctx, "https://api.example.com/users", testData); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	retrieved, ok, err := cache.Get(ctx, "https://api.example.com/users")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
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
	ctx := context.Background()
	mock := newMockCache()
	cache, err := NewGzip(GzipConfig{
		Cache: mock,
	})
	if err != nil {
		t.Fatalf("NewGzip() failed: %v", err)
	}

	// Store corrupted data with gzip marker but invalid compressed data
	_ = mock.Set(ctx, "corrupted", []byte{byte(Gzip + 1), 0xFF, 0xFF, 0xFF})

	_, ok, _ := cache.Get(ctx, "corrupted")
	if ok {
		t.Error("Get() should return false for corrupted data")
	}
}

func TestUncompressedData(t *testing.T) {
	ctx := context.Background()
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
	_ = mock.Set(ctx, "uncompressed", data)

	retrieved, ok, err := cache.Get(ctx, "uncompressed")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if !ok {
		t.Fatal("Get() should return true for uncompressed data")
	}

	if !bytes.Equal(retrieved, testData) {
		t.Error("Retrieved uncompressed data doesn't match original")
	}
}

func TestCompressionLevels(t *testing.T) {
	ctx := context.Background()
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

			if err := cache.Set(ctx, "key", testData); err != nil {
				t.Fatalf("Set() failed: %v", err)
			}
			retrieved, ok, err := cache.Get(ctx, "key")
			if err != nil {
				t.Fatalf("Get() failed: %v", err)
			}
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
	ctx := context.Background()
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

			if err := cache.Set(ctx, "key", testData); err != nil {
				t.Fatalf("Set() failed: %v", err)
			}
			retrieved, ok, err := cache.Get(ctx, "key")
			if err != nil {
				t.Fatalf("Get() failed: %v", err)
			}
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
	ctx := context.Background()
	testData := []byte(strings.Repeat("round trip test ", 100))

	t.Run("Gzip", func(t *testing.T) {
		cache, _ := NewGzip(GzipConfig{Cache: newMockCache()})
		_ = cache.Set(ctx, "key", testData)
		retrieved, ok, _ := cache.Get(ctx, "key")
		if !ok || !bytes.Equal(retrieved, testData) {
			t.Error("Gzip round trip failed")
		}
	})

	t.Run("Brotli", func(t *testing.T) {
		cache, _ := NewBrotli(BrotliConfig{Cache: newMockCache()})
		_ = cache.Set(ctx, "key", testData)
		retrieved, ok, _ := cache.Get(ctx, "key")
		if !ok || !bytes.Equal(retrieved, testData) {
			t.Error("Brotli round trip failed")
		}
	})

	t.Run("Snappy", func(t *testing.T) {
		cache, _ := NewSnappy(SnappyConfig{Cache: newMockCache()})
		_ = cache.Set(ctx, "key", testData)
		retrieved, ok, _ := cache.Get(ctx, "key")
		if !ok || !bytes.Equal(retrieved, testData) {
			t.Error("Snappy round trip failed")
		}
	})
}

func TestEmptyValue(t *testing.T) {
	ctx := context.Background()
	cache, _ := NewGzip(GzipConfig{Cache: newMockCache()})

	// Set and get empty value
	_ = cache.Set(ctx, "empty", []byte{})
	retrieved, ok, _ := cache.Get(ctx, "empty")
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
	ctx := context.Background()
	cache, _ := NewGzip(GzipConfig{Cache: newMockCache()})

	// Set value multiple times
	for i := 0; i < 3; i++ {
		data := []byte(strings.Repeat("iteration ", i+1))
		_ = cache.Set(ctx, "key", data)
	}

	// Should have the last value
	retrieved, ok, _ := cache.Get(ctx, "key")
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
	ctx := context.Background()
	mock := newMockCache()
	cache, _ := NewBrotli(BrotliConfig{Cache: mock})

	// Store corrupted brotli data
	_ = mock.Set(ctx, "corrupted", []byte{byte(Brotli + 1), 0xFF, 0xFF, 0xFF})

	_, ok, _ := cache.Get(ctx, "corrupted")
	if ok {
		t.Error("Get() should return false for corrupted brotli data")
	}
}

func TestSnappyCorruptedData(t *testing.T) {
	ctx := context.Background()
	mock := newMockCache()
	cache, _ := NewSnappy(SnappyConfig{Cache: mock})

	// Store corrupted snappy data
	_ = mock.Set(ctx, "corrupted", []byte{byte(Snappy + 1), 0xFF, 0xFF, 0xFF})

	_, ok, _ := cache.Get(ctx, "corrupted")
	if ok {
		t.Error("Get() should return false for corrupted snappy data")
	}
}
