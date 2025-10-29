package hazelcast

import (
	"context"
	"testing"

	"github.com/hazelcast/hazelcast-go-client"
	"github.com/sandrolain/httpcache"
)

const (
	benchmarkKey   = "bench-key"
	benchmarkValue = "bench-value"
)

// setupBenchmarkCache creates a Hazelcast cache for benchmarking.
func setupBenchmarkCache(b *testing.B) (httpcache.Cache, func()) {
	b.Helper()

	ctx := context.Background()

	// Create Hazelcast client configuration
	config := hazelcast.Config{}
	config.Cluster.Network.SetAddresses("localhost:5701")
	config.Cluster.Unisocket = true

	// Create client
	client, err := hazelcast.StartNewClientWithConfig(ctx, config)
	if err != nil {
		b.Skipf("skipping benchmark; no Hazelcast server running at localhost:5701: %v", err)
	}

	// Get map
	m, err := client.GetMap(ctx, "bench-cache")
	if err != nil {
		client.Shutdown(ctx)
		b.Fatalf("failed to get Hazelcast map: %v", err)
	}

	// Clear any existing data
	if err := m.Clear(ctx); err != nil {
		client.Shutdown(ctx)
		b.Fatalf("failed to clear Hazelcast map: %v", err)
	}

	cleanup := func() {
		_ = m.Clear(ctx)
		_ = client.Shutdown(ctx)
	}

	return NewWithMap(m), cleanup
}

// BenchmarkHazelcastGet benchmarks Get operations.
func BenchmarkHazelcastGet(b *testing.B) {
	c, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	value := []byte(benchmarkValue)
	c.Set(benchmarkKey, value)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Get(benchmarkKey)
	}
}

// BenchmarkHazelcastSet benchmarks Set operations.
func BenchmarkHazelcastSet(b *testing.B) {
	c, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	value := []byte(benchmarkValue)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(benchmarkKey, value)
	}
}

// BenchmarkHazelcastDelete benchmarks Delete operations.
func BenchmarkHazelcastDelete(b *testing.B) {
	c, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	value := []byte(benchmarkValue)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		c.Set(benchmarkKey, value)
		b.StartTimer()
		c.Delete(benchmarkKey)
	}
}

// BenchmarkHazelcastSetGet benchmarks combined Set and Get operations.
func BenchmarkHazelcastSetGet(b *testing.B) {
	c, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	value := []byte(benchmarkValue)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(benchmarkKey, value)
		_, _ = c.Get(benchmarkKey)
	}
}

// BenchmarkHazelcastParallelGet benchmarks parallel Get operations.
func BenchmarkHazelcastParallelGet(b *testing.B) {
	c, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	value := []byte(benchmarkValue)
	c.Set(benchmarkKey, value)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = c.Get(benchmarkKey)
		}
	})
}

// BenchmarkHazelcastParallelSet benchmarks parallel Set operations.
func BenchmarkHazelcastParallelSet(b *testing.B) {
	c, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	value := []byte(benchmarkValue)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Set(benchmarkKey, value)
			i++
		}
	})
}

// BenchmarkHazelcastLargeValue benchmarks operations with large values.
func BenchmarkHazelcastLargeValue(b *testing.B) {
	c, cleanup := setupBenchmarkCache(b)
	defer cleanup()

	// Create a 1MB value
	value := make([]byte, 1024*1024)
	for i := range value {
		value[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set("large-key", value)
		_, _ = c.Get("large-key")
	}
}
