//go:build integration
// +build integration

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
