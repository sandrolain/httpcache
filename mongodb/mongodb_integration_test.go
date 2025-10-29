//go:build integration

package mongodb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sandrolain/httpcache/test"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
)

func setupMongoDBContainer(t *testing.T) (string, func()) {
	t.Helper()

	ctx := context.Background()

	mongodbContainer, err := mongodb.Run(ctx,
		"mongo:8",
		mongodb.WithUsername("root"),
		mongodb.WithPassword("password"),
	)
	if err != nil {
		t.Fatalf("Failed to start MongoDB container: %v", err)
	}

	uri, err := mongodbContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("Failed to get MongoDB connection string: %v", err)
	}

	cleanup := func() {
		if err := mongodbContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate MongoDB container: %v", err)
		}
	}

	return uri, cleanup
}

func TestMongoDBCacheIntegration(t *testing.T) {
	uri, cleanup := setupMongoDBContainer(t)
	defer cleanup()

	config := Config{
		URI:        uri,
		Database:   "httpcache_test",
		Collection: "cache_integration",
		Timeout:    10 * time.Second,
	}

	ctx := context.Background()
	cache, err := New(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.(interface{ Close() error }).Close()

	test.Cache(t, cache)
}

func TestMongoDBCacheIntegrationMultipleOperations(t *testing.T) {
	uri, cleanup := setupMongoDBContainer(t)
	defer cleanup()

	config := Config{
		URI:        uri,
		Database:   "httpcache_test",
		Collection: "cache_multi",
		Timeout:    10 * time.Second,
	}

	ctx := context.Background()
	cache, err := New(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.(interface{ Close() error }).Close()

	// Test multiple operations
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key-%d", i)
		value := []byte(fmt.Sprintf("value-%d", i))

		cache.Set(key, value)

		retrieved, ok := cache.Get(key)
		if !ok {
			t.Errorf("Failed to retrieve key %q", key)
		}
		if string(retrieved) != string(value) {
			t.Errorf("Expected %q, got %q", string(value), string(retrieved))
		}
	}

	// Test deletion
	cache.Delete("key-5")
	_, ok := cache.Get("key-5")
	if ok {
		t.Error("Expected key-5 to be deleted")
	}
}

func TestMongoDBCacheIntegrationWithTTL(t *testing.T) {
	uri, cleanup := setupMongoDBContainer(t)
	defer cleanup()

	config := Config{
		URI:        uri,
		Database:   "httpcache_test",
		Collection: "cache_ttl_integration",
		Timeout:    10 * time.Second,
		TTL:        1 * time.Hour, // Reasonable TTL for production
	}

	ctx := context.Background()
	cache, err := New(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.(interface{ Close() error }).Close()

	// Set and retrieve a value
	cache.Set("ttl-key", []byte("ttl-value"))

	value, ok := cache.Get("ttl-key")
	if !ok {
		t.Fatal("Expected to find cached value")
	}
	if string(value) != "ttl-value" {
		t.Fatalf("Expected 'ttl-value', got %q", string(value))
	}

	t.Log("TTL index created and cache working correctly")
}

func TestMongoDBCacheIntegrationConcurrent(t *testing.T) {
	uri, cleanup := setupMongoDBContainer(t)
	defer cleanup()

	config := Config{
		URI:        uri,
		Database:   "httpcache_test",
		Collection: "cache_concurrent",
		Timeout:    10 * time.Second,
	}

	ctx := context.Background()
	cache, err := New(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.(interface{ Close() error }).Close()

	// Run concurrent operations
	done := make(chan bool, 3)

	// Writer 1
	go func() {
		for i := 0; i < 50; i++ {
			cache.Set(fmt.Sprintf("key-%d", i), []byte(fmt.Sprintf("value-%d", i)))
		}
		done <- true
	}()

	// Writer 2
	go func() {
		for i := 50; i < 100; i++ {
			cache.Set(fmt.Sprintf("key-%d", i), []byte(fmt.Sprintf("value-%d", i)))
		}
		done <- true
	}()

	// Reader
	go func() {
		for i := 0; i < 100; i++ {
			cache.Get(fmt.Sprintf("key-%d", i))
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done

	t.Log("Concurrent operations completed successfully")
}
