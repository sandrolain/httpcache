package natskv

import (
	"context"
	"testing"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	benchmarkKey   = "bench-key"
	benchmarkValue = "bench-value"
)

// setupBenchmarkCache creates a NATS K/V cache for benchmarking.
func setupBenchmarkCache(b *testing.B) (cache, func()) {
	b.Helper()

	opts := &server.Options{
		JetStream: true,
		Port:      -1,
		Host:      "127.0.0.1",
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		b.Fatalf("failed to create NATS server: %v", err)
	}

	go ns.Start()

	if !ns.ReadyForConnections(4 * 1e9) {
		b.Fatal("NATS server did not start in time")
	}

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		ns.Shutdown()
		b.Fatalf("failed to connect to NATS: %v", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		ns.Shutdown()
		b.Fatalf("failed to create JetStream context: %v", err)
	}

	ctx := context.Background()
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: "bench-cache",
	})
	if err != nil {
		nc.Close()
		ns.Shutdown()
		b.Fatalf("failed to create K/V bucket: %v", err)
	}

	cleanup := func() {
		nc.Close()
		ns.Shutdown()
	}

	return NewWithKeyValue(kv).(cache), cleanup
}

// BenchmarkNATSKVGet benchmarks Get operations.
func BenchmarkNATSKVGet(b *testing.B) {
	c, cleanup := setupBenchmarkCache(b)
	defer cleanup()
	ctx := context.Background()

	value := []byte(benchmarkValue)
	_ = c.Set(ctx, benchmarkKey, value)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = c.Get(ctx, benchmarkKey)
	}
}

// BenchmarkNATSKVSet benchmarks Set operations.
func BenchmarkNATSKVSet(b *testing.B) {
	c, cleanup := setupBenchmarkCache(b)
	defer cleanup()
	ctx := context.Background()

	value := []byte(benchmarkValue)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Set(ctx, benchmarkKey, value)
	}
}

// BenchmarkNATSKVDelete benchmarks Delete operations.
func BenchmarkNATSKVDelete(b *testing.B) {
	c, cleanup := setupBenchmarkCache(b)
	defer cleanup()
	ctx := context.Background()

	value := []byte(benchmarkValue)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		_ = c.Set(ctx, benchmarkKey, value)
		b.StartTimer()
		_ = c.Delete(ctx, benchmarkKey)
	}
}

// BenchmarkNATSKVSetGet benchmarks combined Set and Get operations.
func BenchmarkNATSKVSetGet(b *testing.B) {
	c, cleanup := setupBenchmarkCache(b)
	defer cleanup()
	ctx := context.Background()

	value := []byte(benchmarkValue)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Set(ctx, benchmarkKey, value)
		_, _, _ = c.Get(ctx, benchmarkKey)
	}
}

// BenchmarkNATSKVParallelGet benchmarks parallel Get operations.
func BenchmarkNATSKVParallelGet(b *testing.B) {
	c, cleanup := setupBenchmarkCache(b)
	defer cleanup()
	ctx := context.Background()

	value := []byte(benchmarkValue)
	_ = c.Set(ctx, benchmarkKey, value)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = c.Get(ctx, benchmarkKey)
		}
	})
}

// BenchmarkNATSKVParallelSet benchmarks parallel Set operations.
func BenchmarkNATSKVParallelSet(b *testing.B) {
	c, cleanup := setupBenchmarkCache(b)
	defer cleanup()
	ctx := context.Background()

	value := []byte(benchmarkValue)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = c.Set(ctx, benchmarkKey, value)
		}
	})
}

// BenchmarkNATSKVLargeValue benchmarks operations with large values.
func BenchmarkNATSKVLargeValue(b *testing.B) {
	c, cleanup := setupBenchmarkCache(b)
	defer cleanup()
	ctx := context.Background()

	// Create a 1MB value
	value := make([]byte, 1024*1024)
	for i := range value {
		value[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "large-key"
		_ = c.Set(ctx, key, value)
		_, _, _ = c.Get(ctx, key)
	}
}
