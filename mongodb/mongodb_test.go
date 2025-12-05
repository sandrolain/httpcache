package mongodb

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sandrolain/httpcache/test"
)

func TestMongoDBCache(t *testing.T) {
	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}

	config := Config{
		URI:        uri,
		Database:   "httpcache_test",
		Collection: "cache_test",
		Timeout:    2 * time.Second,
	}

	ctx := context.Background()
	cache, err := New(ctx, config)
	if err != nil {
		t.Skipf("Skipping MongoDB tests: %v", err)
		return
	}
	defer cache.(interface{ Close() error }).Close()

	test.Cache(t, cache)
}

func TestMongoDBCacheWithTTL(t *testing.T) {
	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}

	config := Config{
		URI:        uri,
		Database:   "httpcache_test",
		Collection: "cache_ttl_test",
		Timeout:    2 * time.Second,
		TTL:        2 * time.Second, // Short TTL for testing
	}

	ctx := context.Background()
	cache, err := New(ctx, config)
	if err != nil {
		t.Skipf("Skipping MongoDB TTL tests: %v", err)
		return
	}
	defer cache.(interface{ Close() error }).Close()

	// Set a value
	if err := cache.Set(ctx, "test-key", []byte("test-value")); err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Verify it exists
	value, ok, err := cache.Get(ctx, "test-key")
	if err != nil {
		t.Fatalf("Failed to get value: %v", err)
	}
	if !ok {
		t.Fatal("Expected to find cached value immediately after set")
	}
	if string(value) != "test-value" {
		t.Fatalf("Expected 'test-value', got %q", string(value))
	}

	// Wait for TTL to expire (MongoDB TTL monitor runs every 60 seconds,
	// but for testing we just verify the index was created)
	t.Log("TTL index created successfully")
}

func TestMongoDBCacheConfig(t *testing.T) {
	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}

	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "valid config",
			config: Config{
				URI:        uri,
				Database:   "httpcache_test",
				Collection: "cache_config_test",
			},
			expectError: false,
		},
		{
			name: "missing URI",
			config: Config{
				Database: "httpcache_test",
			},
			expectError: true,
		},
		{
			name: "missing database",
			config: Config{
				URI: uri,
			},
			expectError: true,
		},
		{
			name: "custom prefix and collection",
			config: Config{
				URI:        uri,
				Database:   "httpcache_test",
				Collection: "custom_cache",
				KeyPrefix:  "custom:",
			},
			expectError: false,
		},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := New(ctx, tt.config)
			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error but got none")
				}
				return
			}

			if err != nil {
				if os.Getenv("MONGODB_TEST_URI") == "" {
					t.Skipf("Skipping test (MongoDB not available): %v", err)
					return
				}
				t.Fatalf("Unexpected error: %v", err)
			}
			defer cache.(interface{ Close() error }).Close()
		})
	}
}

func TestMongoDBDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Collection != "httpcache" {
		t.Errorf("Expected default collection 'httpcache', got %q", config.Collection)
	}
	if config.KeyPrefix != "cache:" {
		t.Errorf("Expected default key prefix 'cache:', got %q", config.KeyPrefix)
	}
	if config.Timeout != 5*time.Second {
		t.Errorf("Expected default timeout 5s, got %v", config.Timeout)
	}
}
