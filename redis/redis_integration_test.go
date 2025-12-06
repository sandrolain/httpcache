//go:build integration

package redis

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/sandrolain/httpcache/test"
	"github.com/testcontainers/testcontainers-go"
	rediscontainer "github.com/testcontainers/testcontainers-go/modules/redis"
)

const (
	skipIntegrationMsg = "skipping integration test; use -integration.redis flag to enable"
	redisImage         = "redis:7-alpine"
	failedConnectMsg   = "failed to connect to Redis: %v"
	failedFlushMsg     = "failed to flush Redis: %v"
)

var (
	// Global Redis container and endpoint shared across all tests.
	sharedRedisContainer testcontainers.Container
	sharedRedisEndpoint  string
)

// TestMain sets up the Redis container once for all tests.
func TestMain(m *testing.M) {
	// Parse flags to check for integration flag
	flag.Parse()

	var code int

	ctx := context.Background()

	// Start Redis container
	container, err := rediscontainer.Run(ctx, redisImage)
	if err != nil {
		panic("failed to start Redis container: " + err.Error())
	}
	sharedRedisContainer = container

	// Get endpoint
	endpoint, err := container.Endpoint(ctx, "")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		panic("failed to get Redis endpoint: " + err.Error())
	}
	sharedRedisEndpoint = endpoint

	// Run tests
	code = m.Run()

	// Cleanup
	if err := testcontainers.TerminateContainer(container); err != nil {
		panic("failed to terminate Redis container: " + err.Error())
	}

	os.Exit(code)
}

// setupRedisCache creates a new connection to the shared Redis container and returns the cache instance.
func setupRedisCache(t *testing.T) (cache, func()) {
	t.Helper()

	// Connect to the shared Redis instance
	client := redis.NewClient(&redis.Options{
		Addr: sharedRedisEndpoint,
	})

	ctx := context.Background()

	cleanup := func() {
		_ = client.Close()
	}

	// Flush all data before each test
	if err := client.FlushAll(ctx).Err(); err != nil {
		cleanup()
		t.Fatalf(failedFlushMsg, err)
	}

	return NewWithClient(client).(cache), cleanup
}

// verifyMultipleKeys verifies that all keys have the expected values.
func verifyMultipleKeys(t *testing.T, c cache, keys []string, values [][]byte) {
	t.Helper()
	ctx := context.Background()
	for i, key := range keys {
		val, ok, err := c.Get(ctx, key)
		if err != nil {
			t.Errorf("error getting key %s: %v", key, err)
			continue
		}
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
	ctx := context.Background()
	_, ok, err := c.Get(ctx, key)
	if err != nil {
		t.Errorf("error getting key %s: %v", key, err)
		return
	}
	if ok != shouldExist {
		if shouldExist {
			t.Errorf("expected key %s to exist", key)
		} else {
			t.Errorf("expected key %s to not exist", key)
		}
	}
}

// TestRedisCacheIntegration tests the Redis cache implementation using a real Redis instance via testcontainers.
func TestRedisCacheIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	c, cleanup := setupRedisCache(t)
	defer cleanup()

	// Run cache tests
	test.Cache(t, c)
}

// TestRedisCacheIntegrationMultipleOperations tests multiple cache operations in sequence.
func TestRedisCacheIntegrationMultipleOperations(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	c, cleanup := setupRedisCache(t)
	defer cleanup()

	ctx := context.Background()

	// Test multiple keys
	keys := []string{"key1", "key2", "key3"}
	values := [][]byte{[]byte("value1"), []byte("value2"), []byte("value3")}

	// Set multiple keys
	for i, key := range keys {
		if err := c.Set(ctx, key, values[i]); err != nil {
			t.Fatalf("failed to set key %s: %v", key, err)
		}
	}

	// Verify all keys
	verifyMultipleKeys(t, c, keys, values)

	// Delete one key
	if err := c.Delete(ctx, keys[1]); err != nil {
		t.Fatalf("failed to delete key %s: %v", keys[1], err)
	}

	// Verify deletion
	verifyKeyExists(t, c, keys[1], false)

	// Verify other keys still exist
	verifyKeyExists(t, c, keys[0], true)
	verifyKeyExists(t, c, keys[2], true)
}

// TestRedisCacheIntegrationPersistence tests that values persist across retrievals.
func TestRedisCacheIntegrationPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	c, cleanup := setupRedisCache(t)
	defer cleanup()

	ctx := context.Background()

	// Set a value
	key := "persistentKey"
	value := []byte("persistentValue")
	if err := c.Set(ctx, key, value); err != nil {
		t.Fatalf("failed to set key: %v", err)
	}

	// Retrieve multiple times
	for i := 0; i < 5; i++ {
		val, ok, err := c.Get(ctx, key)
		if err != nil {
			t.Errorf("iteration %d: error getting key: %v", i, err)
			continue
		}
		if !ok {
			t.Errorf("iteration %d: expected key to exist", i)
		}
		if string(val) != string(value) {
			t.Errorf("iteration %d: expected value %s, got %s", i, value, val)
		}
	}
}

// TestRedisCacheNewIntegration tests creating a cache using the New() constructor.
func TestRedisCacheNewIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	// Test with valid configuration
	config := Config{
		Address:      sharedRedisEndpoint,
		PoolSize:     5,
		MaxRetries:   2,
		DialTimeout:  5 * 1e9, // 5 seconds
		ReadTimeout:  3 * 1e9, // 3 seconds
		WriteTimeout: 3 * 1e9, // 3 seconds
	}

	c, err := New(config)
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer c.(interface{ Close() error }).Close()

	ctx := context.Background()

	// Test basic operations
	key := "newTestKey"
	value := []byte("newTestValue")

	if err := c.Set(ctx, key, value); err != nil {
		t.Fatalf("failed to set key: %v", err)
	}

	val, ok, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}
	if !ok {
		t.Fatal("expected key to exist")
	}
	if string(val) != string(value) {
		t.Errorf("expected value %s, got %s", value, val)
	}

	if err := c.Delete(ctx, key); err != nil {
		t.Fatalf("failed to delete key: %v", err)
	}

	_, ok, err = c.Get(ctx, key)
	if err != nil {
		t.Fatalf("failed to get key after delete: %v", err)
	}
	if ok {
		t.Error("expected key to not exist after delete")
	}
}

// TestRedisCacheNewWithEmptyAddress tests that New() returns an error with empty address.
func TestRedisCacheNewWithEmptyAddress(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected error with empty address")
	}
}

// TestRedisCacheNewWithInvalidAddress tests that New() returns an error with invalid address.
func TestRedisCacheNewWithInvalidAddress(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	_, err := New(Config{
		Address:     "localhost:99999", // invalid port
		DialTimeout: 1 * 1e9,           // 1 second timeout
	})
	if err == nil {
		t.Fatal("expected error with invalid address")
	}
}

// TestDefaultConfig tests that DefaultConfig returns sensible defaults.
func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MaxRetries != 3 {
		t.Errorf("expected MaxRetries to be 3, got %d", config.MaxRetries)
	}
	if config.PoolSize != 10 {
		t.Errorf("expected PoolSize to be 10, got %d", config.PoolSize)
	}
	if config.DialTimeout != 5*1e9 {
		t.Errorf("expected DialTimeout to be 5s, got %v", config.DialTimeout)
	}
	if config.ReadTimeout != 5*1e9 {
		t.Errorf("expected ReadTimeout to be 5s, got %v", config.ReadTimeout)
	}
	if config.WriteTimeout != 5*1e9 {
		t.Errorf("expected WriteTimeout to be 5s, got %v", config.WriteTimeout)
	}
	if config.DB != 0 {
		t.Errorf("expected DB to be 0, got %d", config.DB)
	}
}
