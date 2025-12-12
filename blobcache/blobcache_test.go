package blobcache

import (
	"context"
	"os"
	"testing"
	"time"

	_ "gocloud.dev/blob/fileblob" // Register file:// scheme
	_ "gocloud.dev/blob/memblob"  // Register mem:// scheme

	"github.com/sandrolain/httpcache/test"
)

func TestBlobCache(t *testing.T) {
	// Use in-memory blob for testing
	ctx := context.Background()

	cache, err := New(ctx, Config{
		BucketURL: "mem://",
		KeyPrefix: "test/",
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() {
		if closer, ok := cache.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				t.Logf("Failed to close cache: %v", err)
			}
		}
	}()

	test.Cache(t, cache)
}

func TestBlobCacheStale(t *testing.T) {
	// Use in-memory blob for testing
	ctx := context.Background()

	cache, err := New(ctx, Config{
		BucketURL: "mem://",
		KeyPrefix: "test/",
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() {
		if closer, ok := cache.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				t.Logf("Failed to close cache: %v", err)
			}
		}
	}()

	test.CacheStale(t, cache)
}

func TestBlobCacheWithFile(t *testing.T) {
	// Create temporary directory for file-based blob storage
	tmpDir, err := os.MkdirTemp("", "blobcache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	cache, err := New(ctx, Config{
		BucketURL: "file://" + tmpDir,
		KeyPrefix: "cache/",
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() {
		if closer, ok := cache.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				t.Logf("Failed to close cache: %v", err)
			}
		}
	}()

	test.Cache(t, cache)
}

func TestBlobCacheConfig(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "valid config with mem",
			config: Config{
				BucketURL: "mem://",
				KeyPrefix: "test/",
			},
			expectError: false,
		},
		{
			name: "missing bucket URL and bucket",
			config: Config{
				KeyPrefix: "test/",
			},
			expectError: true,
		},
		{
			name: "custom timeout",
			config: Config{
				BucketURL: "mem://",
				Timeout:   1 * time.Second,
			},
			expectError: false,
		},
		{
			name: "default prefix",
			config: Config{
				BucketURL: "mem://",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(ctx, tt.config)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if c == nil {
				t.Fatal("Expected cache, got nil")
			}

			if closer, ok := c.(interface{ Close() error }); ok {
				defer closer.Close()
			}

			// Verify default values are applied
			blobCache, ok := c.(*cache)
			if !ok {
				t.Fatal("cache is not of type *cache")
			}
			if tt.config.KeyPrefix == "" && blobCache.keyPrefix != DefaultConfig().KeyPrefix {
				t.Errorf("Expected default key prefix %q, got %q", DefaultConfig().KeyPrefix, blobCache.keyPrefix)
			}
			if tt.config.Timeout == 0 && blobCache.timeout != DefaultConfig().Timeout {
				t.Errorf("Expected default timeout %v, got %v", DefaultConfig().Timeout, blobCache.timeout)
			}
		})
	}
}

func TestBlobCacheDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.KeyPrefix != "cache/" {
		t.Errorf("Expected default key prefix 'cache/', got %q", config.KeyPrefix)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("Expected default timeout 30s, got %v", config.Timeout)
	}
}

func TestBlobCacheKeyPrefix(t *testing.T) {
	ctx := context.Background()

	c, err := New(ctx, Config{
		BucketURL: "mem://",
		KeyPrefix: "custom-prefix/",
	})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() {
		if closer, ok := c.(interface{ Close() error }); ok {
			closer.Close()
		}
	}()

	blobCache, ok := c.(*cache)
	if !ok {
		t.Fatal("cache is not of type *cache")
	}
	key := blobCache.cacheKey("test-key")

	if len(key) < len("custom-prefix/") {
		t.Errorf("Cache key too short: %q", key)
	}

	if key[:len("custom-prefix/")] != "custom-prefix/" {
		t.Errorf("Expected key to start with 'custom-prefix/', got %q", key)
	}
}

func TestBlobCacheOperations(t *testing.T) {
	ctx := context.Background()

	cache, err := New(ctx, Config{
		BucketURL: "mem://",
	})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() {
		if closer, ok := cache.(interface{ Close() error }); ok {
			closer.Close()
		}
	}()

	// Test Set and Get
	key := "test-key"
	value := []byte("test-value")

	if err := cache.Set(ctx, key, value); err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	retrieved, ok, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Failed to get value: %v", err)
	}
	if !ok {
		t.Fatal("Expected to find cached value")
	}
	if string(retrieved) != string(value) {
		t.Errorf("Expected %q, got %q", string(value), string(retrieved))
	}

	// Test Delete
	if err := cache.Delete(ctx, key); err != nil {
		t.Fatalf("Failed to delete value: %v", err)
	}

	_, ok, err = cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Failed to get value after delete: %v", err)
	}
	if ok {
		t.Error("Expected key to be deleted")
	}
}
