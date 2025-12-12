package hazelcast

import (
	"context"
	"testing"
	"time"

	"github.com/hazelcast/hazelcast-go-client"
	"github.com/hazelcast/hazelcast-go-client/types"
	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/test"
)

// setupHazelcastCache creates a Hazelcast client and map for testing.
func setupHazelcastCache(t *testing.T) (httpcache.Cache, func()) {
	t.Helper()

	ctx := context.Background()

	// Create Hazelcast client configuration with short connection timeout
	config := hazelcast.Config{}
	config.Cluster.Network.SetAddresses("localhost:5701")
	config.Cluster.Unisocket = true
	config.Cluster.ConnectionStrategy.Timeout = types.Duration(5 * time.Second)

	// Create client
	client, err := hazelcast.StartNewClientWithConfig(ctx, config)
	if err != nil {
		t.Skipf("skipping test; no Hazelcast server running at localhost:5701: %v", err)
	}

	// Get map
	m, err := client.GetMap(ctx, "test-cache")
	if err != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = client.Shutdown(shutdownCtx)
		cancel()
		t.Fatalf("failed to get Hazelcast map: %v", err)
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

	return NewWithMap(m), cleanup
}

// TestHazelcastCache tests the Hazelcast cache implementation.
func TestHazelcastCache(t *testing.T) {
	c, cleanup := setupHazelcastCache(t)
	defer cleanup()

	test.Cache(t, c)
}

func TestHazelcastCacheStale(t *testing.T) {
	c, cleanup := setupHazelcastCache(t)
	defer cleanup()

	test.CacheStale(t, c)
}
