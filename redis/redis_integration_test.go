//go:build integration
// +build integration

package redis

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/gomodule/redigo/redis"
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
	conn, err := redis.Dial("tcp", sharedRedisEndpoint)
	if err != nil {
		t.Fatalf(failedConnectMsg, err)
	}

	cleanup := func() {
		_ = conn.Close()
	}

	// Flush all data before each test
	_, err = conn.Do("FLUSHALL")
	if err != nil {
		cleanup()
		t.Fatalf(failedFlushMsg, err)
	}

	return NewWithClient(conn).(cache), cleanup
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

// TestRedisCacheIntegrationPersistence tests that values persist across retrievals.
func TestRedisCacheIntegrationPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	c, cleanup := setupRedisCache(t)
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
