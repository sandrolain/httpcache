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
