package natskv

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/sandrolain/httpcache/test"
)

// startNATSServer starts an embedded NATS server for testing.
func startNATSServer(t *testing.T) *server.Server {
	t.Helper()

	opts := &server.Options{
		JetStream: true,
		Port:      -1, // Random port
		Host:      "127.0.0.1",
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("failed to create NATS server: %v", err)
	}

	go ns.Start()

	if !ns.ReadyForConnections(time.Second * 4) { // 4 seconds
		t.Fatal("NATS server did not start in time")
	}

	return ns
}

// setupNATSCache creates a NATS connection and K/V store for testing.
func setupNATSCache(t *testing.T) (cache, *nats.Conn, func()) {
	t.Helper()

	ns := startNATSServer(t)

	// Connect to the embedded server
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		ns.Shutdown()
		t.Fatalf("failed to connect to NATS: %v", err)
	}

	// Create JetStream context
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		ns.Shutdown()
		t.Fatalf("failed to create JetStream context: %v", err)
	}

	// Create K/V bucket
	ctx := context.Background()
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: "test-cache",
	})
	if err != nil {
		nc.Close()
		ns.Shutdown()
		t.Fatalf("failed to create K/V bucket: %v", err)
	}

	cleanup := func() {
		nc.Close()
		ns.Shutdown()
	}

	return NewWithKeyValue(kv).(cache), nc, cleanup
}

// TestNATSKVCache tests the NATS K/V cache implementation.
func TestNATSKVCache(t *testing.T) {
	c, _, cleanup := setupNATSCache(t)
	defer cleanup()

	test.Cache(t, c)
}

// TestNew tests the New constructor.
func TestNew(t *testing.T) {
	ns := startNATSServer(t)
	defer ns.Shutdown()

	ctx := context.Background()

	// Test with valid configuration
	config := Config{
		NATSUrl:     ns.ClientURL(),
		Bucket:      "test-new-cache",
		Description: "Test cache created with New()",
		TTL:         time.Hour,
	}

	c, err := New(ctx, config)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Verify it implements Close
	closer, ok := c.(interface{ Close() error })
	if !ok {
		t.Fatal("cache does not implement Close()")
	}
	defer closer.Close()

	// Test basic operations
	test.Cache(t, c)
}

// TestNewWithEmptyBucket tests that New() fails without a bucket name.
func TestNewWithEmptyBucket(t *testing.T) {
	config := Config{
		NATSUrl: "nats://localhost:4222",
	}

	_, err := New(context.Background(), config)
	if err == nil {
		t.Error("New() should fail with empty bucket name")
	}
}

// TestNewWithInvalidURL tests that New() fails with invalid NATS URL.
func TestNewWithInvalidURL(t *testing.T) {
	config := Config{
		NATSUrl: "nats://invalid-host:9999",
		Bucket:  "test-bucket",
	}

	_, err := New(context.Background(), config)
	if err == nil {
		t.Error("New() should fail with invalid NATS URL")
	}
}

// TestNewWithDefaultURL tests that New() uses default URL when not specified.
func TestNewWithDefaultURL(t *testing.T) {
	ns := startNATSServer(t)
	defer ns.Shutdown()

	// Start server on default port (this test might conflict with a running NATS)
	// Skip this test in CI or when NATS is not available on default port
	t.Skip("Skipping test that requires NATS on default port")

	config := Config{
		Bucket: "test-default-url",
	}

	_, err := New(context.Background(), config)
	if err != nil {
		t.Logf("New() with default URL failed (expected if NATS not running): %v", err)
	}
}

// TestCloseWithoutConnection tests that Close() works when connection is nil.
func TestCloseWithoutConnection(t *testing.T) {
	c, _, cleanup := setupNATSCache(t)
	defer cleanup()

	// Close should not fail even if nc is nil (created with NewWithKeyValue)
	if err := c.Close(); err != nil {
		t.Errorf("Close() on NewWithKeyValue cache failed: %v", err)
	}
}

// TestCloseWithConnection tests that Close() works when connection exists.
func TestCloseWithConnection(t *testing.T) {
	ns := startNATSServer(t)
	defer ns.Shutdown()

	ctx := context.Background()

	config := Config{
		NATSUrl: ns.ClientURL(),
		Bucket:  "test-close",
	}

	cache, err := New(ctx, config)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Type assert to interface with Close method
	closer, ok := cache.(interface{ Close() error })
	if !ok {
		t.Fatal("cache does not implement Close()")
	}

	// Close should work
	if err := closer.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

// TestNewWithNATSOptions tests that custom NATS options are used.
func TestNewWithNATSOptions(t *testing.T) {
	ns := startNATSServer(t)
	defer ns.Shutdown()

	ctx := context.Background()

	config := Config{
		NATSUrl: ns.ClientURL(),
		Bucket:  "test-options",
		NATSOptions: []nats.Option{
			nats.Name("test-client"),
			nats.MaxReconnects(5),
		},
	}

	c, err := New(ctx, config)
	if err != nil {
		t.Fatalf("New() with options failed: %v", err)
	}

	closer, ok := c.(interface{ Close() error })
	if !ok {
		t.Fatal("cache does not implement Close()")
	}
	defer closer.Close()

	// Verify cache works by testing basic operations
	testKey := "test-key"
	testValue := []byte("test-value")

	// Set
	if err := c.Set(ctx, testKey, testValue); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Get
	val, ok, err := c.Get(ctx, testKey)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if !ok {
		t.Error("Get() failed to retrieve value")
	}
	if string(val) != string(testValue) {
		t.Errorf("Get() = %s, want %s", string(val), string(testValue))
	}

	// Delete
	if err := c.Delete(ctx, testKey); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	// Verify deletion
	_, ok, err = c.Get(ctx, testKey)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if ok {
		t.Error("Get() should not retrieve deleted value")
	}
}
