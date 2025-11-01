//go:build integration

package memcache

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/sandrolain/httpcache/test"
	"github.com/testcontainers/testcontainers-go"
	testcontainersMemcache "github.com/testcontainers/testcontainers-go/modules/memcached"
)

const (
	skipIntegrationMsg = "skipping integration test in short mode"
	memcachedImage     = "memcached:1.6-alpine"
)

var (
	// Global Memcached container and endpoint shared across all tests.
	sharedMemcachedContainer testcontainers.Container
	sharedMemcachedEndpoint  string
)

// TestMain sets up the Memcached container once for all tests.
func TestMain(m *testing.M) {
	// Parse flags to check for -short
	flag.Parse()

	var code int

	// Check SKIP_INTEGRATION environment variable
	skipIntegration := os.Getenv("SKIP_INTEGRATION") != ""

	if !skipIntegration {
		ctx := context.Background()

		// Start Memcached container
		container, err := testcontainersMemcache.Run(ctx, memcachedImage)
		if err != nil {
			panic("failed to start Memcached container: " + err.Error())
		}
		sharedMemcachedContainer = container

		// Get endpoint
		endpoint, err := container.Endpoint(ctx, "")
		if err != nil {
			_ = testcontainers.TerminateContainer(container)
			panic("failed to get Memcached endpoint: " + err.Error())
		}
		sharedMemcachedEndpoint = endpoint

		// Run tests
		code = m.Run()

		// Cleanup
		if err := testcontainers.TerminateContainer(container); err != nil {
			panic("failed to terminate Memcached container: " + err.Error())
		}
	} else {
		// Just run tests without container
		code = m.Run()
	}

	os.Exit(code)
}

// setupMemcacheCache creates a new cache instance using the shared Memcached container.
func setupMemcacheCache(t *testing.T) *Cache {
	t.Helper()

	// Create cache instance
	cache := New(sharedMemcachedEndpoint)

	// Flush all data before each test (best effort)
	_ = cache.DeleteAll()

	return cache
}

// TestMemcacheIntegration tests the Memcache implementation using a real Memcached instance via testcontainers.
func TestMemcacheIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	cache := setupMemcacheCache(t)

	// Run cache tests
	test.Cache(t, cache)
}

// TestMemcacheIntegrationMultipleOperations tests multiple cache operations in sequence.
func TestMemcacheIntegrationMultipleOperations(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	cache := setupMemcacheCache(t)

	// Test multiple keys
	keys := []string{"key1", "key2", "key3"}
	values := [][]byte{[]byte("value1"), []byte("value2"), []byte("value3")}

	// Set multiple keys
	for i, key := range keys {
		cache.Set(key, values[i])
	}

	// Verify all keys
	for i, key := range keys {
		val, ok := cache.Get(key)
		if !ok {
			t.Errorf("expected key %s to exist", key)
		}
		if string(val) != string(values[i]) {
			t.Errorf("expected value %s, got %s", values[i], val)
		}
	}

	// Delete one key
	cache.Delete(keys[1])

	// Verify deletion
	_, ok := cache.Get(keys[1])
	if ok {
		t.Error("expected key2 to be deleted")
	}

	// Verify other keys still exist
	_, ok = cache.Get(keys[0])
	if !ok {
		t.Error("expected key1 to still exist")
	}
	_, ok = cache.Get(keys[2])
	if !ok {
		t.Error("expected key3 to still exist")
	}
}

// TestMemcacheIntegrationPersistence tests that values persist across retrievals.
func TestMemcacheIntegrationPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	cache := setupMemcacheCache(t)

	// Set a value
	key := "persistentKey"
	value := []byte("persistentValue")
	cache.Set(key, value)

	// Retrieve multiple times
	for i := 0; i < 5; i++ {
		val, ok := cache.Get(key)
		if !ok {
			t.Errorf("iteration %d: expected key to exist", i)
		}
		if string(val) != string(value) {
			t.Errorf("iteration %d: expected value %s, got %s", i, value, val)
		}
	}
}

// TestMemcacheIntegrationLargeValue tests storing and retrieving large values.
func TestMemcacheIntegrationLargeValue(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	cache := setupMemcacheCache(t)

	// Create a large value (100KB)
	largeValue := make([]byte, 100*1024)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	key := "largeKey"
	cache.Set(key, largeValue)

	// Retrieve and verify
	retrievedValue, ok := cache.Get(key)
	if !ok {
		t.Fatal("expected large value to be stored and retrieved")
	}

	if len(retrievedValue) != len(largeValue) {
		t.Errorf("expected length %d, got %d", len(largeValue), len(retrievedValue))
	}

	// Verify content
	for i := range largeValue {
		if retrievedValue[i] != largeValue[i] {
			t.Errorf("value mismatch at position %d", i)
			break
		}
	}
}
