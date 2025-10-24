//go:build integration
// +build integration

package hazelcast

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hazelcast/hazelcast-go-client"
	"github.com/sandrolain/httpcache/test"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	skipIntegrationMsg = "skipping integration test; use -integration.hazelcast flag to enable"
	hazelcastImage     = "hazelcast/hazelcast:5.6"
	failedConnectMsg   = "failed to connect to Hazelcast: %v"
	failedSetupMsg     = "failed to setup Hazelcast map: %v"
)

var (
	// Global Hazelcast container and endpoint shared across all tests.
	sharedHazelcastContainer testcontainers.Container
	sharedHazelcastEndpoint  string
)

// TestMain sets up the Hazelcast container once for all tests.
func TestMain(m *testing.M) {
	// Parse flags to check for integration flag
	flag.Parse()

	var code int

	ctx := context.Background()

	// Start Hazelcast container
	req := testcontainers.ContainerRequest{
		Image:        hazelcastImage,
		ExposedPorts: []string{"5701/tcp"},
		Env: map[string]string{
			"HZ_NETWORK_PUBLICADDRESS": "127.0.0.1:5701",
		},
		WaitingFor: wait.ForLog("is STARTED").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		panic("failed to start Hazelcast container: " + err.Error())
	}
	sharedHazelcastContainer = container

	// Get endpoint
	host, err := container.Host(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		panic("failed to get Hazelcast host: " + err.Error())
	}

	port, err := container.MappedPort(ctx, "5701")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		panic("failed to get Hazelcast port: " + err.Error())
	}

	sharedHazelcastEndpoint = fmt.Sprintf("%s:%s", host, port.Port())

	// Wait a bit for Hazelcast to be fully ready
	time.Sleep(5 * time.Second)

	// Run tests
	code = m.Run()

	// Cleanup
	if err := testcontainers.TerminateContainer(container); err != nil {
		panic("failed to terminate Hazelcast container: " + err.Error())
	}

	os.Exit(code)
}

// setupHazelcastIntegrationCache creates a new connection to the shared Hazelcast container and returns the cache instance.
func setupHazelcastIntegrationCache(t *testing.T) (cache, func()) {
	t.Helper()

	ctx := context.Background()

	// Create Hazelcast client configuration
	config := hazelcast.Config{}
	config.Cluster.Network.SetAddresses(sharedHazelcastEndpoint)
	config.Cluster.Unisocket = true

	// Create client
	client, err := hazelcast.StartNewClientWithConfig(ctx, config)
	if err != nil {
		t.Fatalf(failedConnectMsg, err)
	}

	// Get map
	m, err := client.GetMap(ctx, "test-cache")
	if err != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = client.Shutdown(shutdownCtx)
		cancel()
		t.Fatalf(failedSetupMsg, err)
	}

	// Clear any existing data
	if err := m.Clear(ctx); err != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = client.Shutdown(shutdownCtx)
		cancel()
		t.Fatalf("failed to clear Hazelcast map: %v", err)
	}

	cleanup := func() {
		clearCtx, clearCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = m.Clear(clearCtx)
		clearCancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = client.Shutdown(shutdownCtx)
		shutdownCancel()
	}

	return NewWithMap(m).(cache), cleanup
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

// TestHazelcastCacheIntegration tests the Hazelcast cache implementation using a real Hazelcast instance via testcontainers.
func TestHazelcastCacheIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	c, cleanup := setupHazelcastIntegrationCache(t)
	defer cleanup()

	// Run cache tests
	test.Cache(t, c)
}

// TestHazelcastCacheIntegrationMultipleOperations tests multiple cache operations in sequence.
func TestHazelcastCacheIntegrationMultipleOperations(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	c, cleanup := setupHazelcastIntegrationCache(t)
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

// TestHazelcastCacheIntegrationPersistence tests that values persist across retrievals.
func TestHazelcastCacheIntegrationPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	c, cleanup := setupHazelcastIntegrationCache(t)
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

// TestHazelcastCacheIntegrationWithContext tests cache with custom context.
func TestHazelcastCacheIntegrationWithContext(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	ctx := context.Background()

	// Create Hazelcast client configuration
	config := hazelcast.Config{}
	config.Cluster.Network.SetAddresses(sharedHazelcastEndpoint)
	config.Cluster.Unisocket = true

	// Create client
	client, err := hazelcast.StartNewClientWithConfig(ctx, config)
	if err != nil {
		t.Fatalf(failedConnectMsg, err)
	}

	// Get map
	m, err := client.GetMap(ctx, "test-cache-ctx")
	if err != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = client.Shutdown(shutdownCtx)
		cancel()
		t.Fatalf(failedSetupMsg, err)
	}

	// Clear any existing data
	if err := m.Clear(ctx); err != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = client.Shutdown(shutdownCtx)
		cancel()
		t.Fatalf("failed to clear Hazelcast map: %v", err)
	}

	customCtx := context.Background()
	cache := NewWithMapAndContext(customCtx, m)

	// Test basic operations
	key := "testKey"
	value := []byte("testValue")

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
		t.Error("expected key to not exist after delete")
	}

	// Cleanup
	clearCtx, clearCancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = m.Clear(clearCtx)
	clearCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = client.Shutdown(shutdownCtx)
	shutdownCancel()
}
