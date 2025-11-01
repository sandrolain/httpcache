//go:build integration

package natskv

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/sandrolain/httpcache/test"
	"github.com/testcontainers/testcontainers-go"
	natscontainer "github.com/testcontainers/testcontainers-go/modules/nats"
)

const (
	skipIntegrationMsg = "skipping integration test; use -integration.nats flag to enable"
	natsImage          = "nats:2-alpine"
	failedConnectMsg   = "failed to connect to NATS: %v"
	failedSetupMsg     = "failed to setup NATS K/V: %v"
)

var (
	// Global NATS container and endpoint shared across all tests.
	sharedNATSContainer testcontainers.Container
	sharedNATSEndpoint  string
)

// TestMain sets up the NATS container once for all tests.
func TestMain(m *testing.M) {
	// Parse flags to check for integration flag
	flag.Parse()

	var code int

	ctx := context.Background()

	// Start NATS container with JetStream enabled
	container, err := natscontainer.Run(ctx, natsImage, testcontainers.WithCmd("-js"))
	if err != nil {
		panic("failed to start NATS container: " + err.Error())
	}
	sharedNATSContainer = container

	// Get endpoint
	endpoint, err := container.ConnectionString(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		panic("failed to get NATS endpoint: " + err.Error())
	}
	sharedNATSEndpoint = endpoint

	// Run tests
	code = m.Run()

	// Cleanup
	if err := testcontainers.TerminateContainer(container); err != nil {
		panic("failed to terminate NATS container: " + err.Error())
	}

	os.Exit(code)
}

// setupNATSKVCache creates a new connection to the shared NATS container and returns the cache instance.
func setupNATSKVCache(t *testing.T) (cache, func()) {
	t.Helper()

	// Connect to the shared NATS instance
	nc, err := nats.Connect(sharedNATSEndpoint)
	if err != nil {
		t.Fatalf(failedConnectMsg, err)
	}

	cleanup := func() {
		nc.Close()
	}

	// Create JetStream context
	js, err := jetstream.New(nc)
	if err != nil {
		cleanup()
		t.Fatalf(failedSetupMsg, err)
	}

	// Create or get K/V bucket
	ctx := context.Background()
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: "test-cache",
	})
	if err != nil {
		cleanup()
		t.Fatalf(failedSetupMsg, err)
	}

	// Purge all data before each test
	if err := kv.PurgeDeletes(ctx); err != nil {
		cleanup()
		t.Fatalf("failed to purge NATS K/V: %v", err)
	}

	return NewWithKeyValue(kv).(cache), cleanup
}

// verifyMultipleKeys verifies that all keys have the expected values.
func verifyMultipleKeys(t *testing.T, c cache, keys []string, values [][]byte) {
	t.Helper()
	for i, key := range keys {
		val, ok := c.Get(key)
		if !ok {
			t.Errorf("expected key %s to exist", key)
		}
		if string(val) != string(values[i]) {
			t.Errorf("expected value %s, got %s", values[i], val)
		}
	}
}

// verifyKeyExists verifies that a key exists.
func verifyKeyExists(t *testing.T, c cache, key string, shouldExist bool) {
	t.Helper()
	_, ok := c.Get(key)
	if ok != shouldExist {
		if shouldExist {
			t.Errorf("expected key %s to exist", key)
		} else {
			t.Errorf("expected key %s to not exist", key)
		}
	}
}

// TestNATSKVCacheIntegration tests the NATS K/V cache implementation using a real NATS instance via testcontainers.
func TestNATSKVCacheIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	c, cleanup := setupNATSKVCache(t)
	defer cleanup()

	// Run cache tests
	test.Cache(t, c)
}

// TestNATSKVCacheIntegrationMultipleOperations tests multiple cache operations in sequence.
func TestNATSKVCacheIntegrationMultipleOperations(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	c, cleanup := setupNATSKVCache(t)
	defer cleanup()

	// Test multiple keys
	keys := []string{"key1", "key2", "key3"}
	values := [][]byte{[]byte("value1"), []byte("value2"), []byte("value3")}

	// Set multiple keys
	for i, key := range keys {
		c.Set(key, values[i])
	}

	// Verify all keys
	verifyMultipleKeys(t, c, keys, values)

	// Delete one key
	c.Delete(keys[1])

	// Verify deletion
	verifyKeyExists(t, c, keys[1], false)

	// Verify other keys still exist
	verifyKeyExists(t, c, keys[0], true)
	verifyKeyExists(t, c, keys[2], true)
}

// TestNATSKVCacheIntegrationPersistence tests that values persist across retrievals.
func TestNATSKVCacheIntegrationPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	c, cleanup := setupNATSKVCache(t)
	defer cleanup()

	// Set a value
	key := "persistentKey"
	value := []byte("persistentValue")
	c.Set(key, value)

	// Retrieve multiple times
	for i := 0; i < 5; i++ {
		val, ok := c.Get(key)
		if !ok {
			t.Errorf("iteration %d: expected key to exist", i)
		}
		if string(val) != string(value) {
			t.Errorf("iteration %d: expected value %s, got %s", i, value, val)
		}
	}
}

// TestNewConstructorIntegration tests the New() constructor with a real NATS instance.
func TestNewConstructorIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	ctx := context.Background()

	// Test with basic configuration
	config := Config{
		NATSUrl: sharedNATSEndpoint,
		Bucket:  "test-new-cache",
	}

	cache, err := New(ctx, config)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Type assert to get Close method
	closer, ok := cache.(interface{ Close() error })
	if !ok {
		t.Fatal("cache does not implement Close()")
	}
	defer closer.Close()

	// Test basic operations
	key := "test-key"
	value := []byte("test-value")

	cache.Set(key, value)

	val, ok := cache.Get(key)
	if !ok {
		t.Error("expected key to exist")
	}
	if string(val) != string(value) {
		t.Errorf("expected value %s, got %s", value, val)
	}

	cache.Delete(key)

	_, ok = cache.Get(key)
	if ok {
		t.Error("expected key to not exist after deletion")
	}
}

// TestNewConstructorWithConfigIntegration tests the New() constructor with custom configuration.
func TestNewConstructorWithConfigIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	ctx := context.Background()

	// Test with full configuration
	config := Config{
		NATSUrl:     sharedNATSEndpoint,
		Bucket:      "test-config-cache",
		Description: "Integration test cache",
		TTL:         0, // No TTL for testing
		NATSOptions: []nats.Option{
			nats.Name("integration-test-client"),
		},
	}

	cache, err := New(ctx, config)
	if err != nil {
		t.Fatalf("New() with config failed: %v", err)
	}

	closer, ok := cache.(interface{ Close() error })
	if !ok {
		t.Fatal("cache does not implement Close()")
	}
	defer closer.Close()

	// Run standard cache tests
	test.Cache(t, cache)
}

// TestNewConstructorMultipleInstancesIntegration tests multiple cache instances with different buckets.
func TestNewConstructorMultipleInstancesIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	ctx := context.Background()

	// Create first cache instance
	config1 := Config{
		NATSUrl: sharedNATSEndpoint,
		Bucket:  "test-cache-1",
	}

	cache1, err := New(ctx, config1)
	if err != nil {
		t.Fatalf("New() cache1 failed: %v", err)
	}
	closer1, _ := cache1.(interface{ Close() error })
	defer closer1.Close()

	// Create second cache instance with different bucket
	config2 := Config{
		NATSUrl: sharedNATSEndpoint,
		Bucket:  "test-cache-2",
	}

	cache2, err := New(ctx, config2)
	if err != nil {
		t.Fatalf("New() cache2 failed: %v", err)
	}
	closer2, _ := cache2.(interface{ Close() error })
	defer closer2.Close()

	// Test isolation between caches
	key := "test-key"
	value1 := []byte("value-1")
	value2 := []byte("value-2")

	// Set different values in each cache
	cache1.Set(key, value1)
	cache2.Set(key, value2)

	// Verify each cache has its own value
	val1, ok1 := cache1.Get(key)
	if !ok1 {
		t.Error("cache1: expected key to exist")
	}
	if string(val1) != string(value1) {
		t.Errorf("cache1: expected value %s, got %s", value1, val1)
	}

	val2, ok2 := cache2.Get(key)
	if !ok2 {
		t.Error("cache2: expected key to exist")
	}
	if string(val2) != string(value2) {
		t.Errorf("cache2: expected value %s, got %s", value2, val2)
	}
}

// TestNewConstructorCreateOrUpdateIntegration tests that New() properly creates or updates buckets.
func TestNewConstructorCreateOrUpdateIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	ctx := context.Background()
	bucketName := "test-create-update"

	// Create first cache - should create bucket
	config1 := Config{
		NATSUrl:     sharedNATSEndpoint,
		Bucket:      bucketName,
		Description: "First description",
	}

	cache1, err := New(ctx, config1)
	if err != nil {
		t.Fatalf("First New() failed: %v", err)
	}
	closer1, _ := cache1.(interface{ Close() error })

	// Set a value
	cache1.Set("key1", []byte("value1"))
	closer1.Close()

	// Create second cache with same bucket - should update/reuse bucket
	config2 := Config{
		NATSUrl:     sharedNATSEndpoint,
		Bucket:      bucketName,
		Description: "Updated description",
	}

	cache2, err := New(ctx, config2)
	if err != nil {
		t.Fatalf("Second New() failed: %v", err)
	}
	closer2, _ := cache2.(interface{ Close() error })
	defer closer2.Close()

	// Verify previous data still exists (bucket was updated, not recreated)
	val, ok := cache2.Get("key1")
	if !ok {
		t.Error("expected key1 to exist after bucket update")
	}
	if string(val) != "value1" {
		t.Errorf("expected value1, got %s", val)
	}
}
