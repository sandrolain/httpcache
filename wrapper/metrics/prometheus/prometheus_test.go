package prometheus

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// mockCache is a simple in-memory cache for testing
type mockCache struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMockCache() *mockCache {
	return &mockCache{
		data: make(map[string][]byte),
	}
}

func (m *mockCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.data[key]
	return val, ok, nil
}

func (m *mockCache) Set(_ context.Context, key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *mockCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func TestPrometheusCollector(t *testing.T) {
	// Create collector with custom registry for testing
	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	// Test cache operation recording
	collector.RecordCacheOperation("get", "memory", "hit", 1*time.Millisecond)
	collector.RecordCacheOperation("get", "memory", "miss", 2*time.Millisecond)
	collector.RecordCacheOperation("set", "memory", "success", 500*time.Microsecond)

	// Verify counter metrics
	expected := `
		# HELP httpcache_cache_requests_total Total number of cache operations
		# TYPE httpcache_cache_requests_total counter
		httpcache_cache_requests_total{cache_backend="memory",operation="get",result="hit"} 1
		httpcache_cache_requests_total{cache_backend="memory",operation="get",result="miss"} 1
		httpcache_cache_requests_total{cache_backend="memory",operation="set",result="success"} 1
	`

	if err := testutil.CollectAndCompare(collector.cacheRequests, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected metrics: %v", err)
	}

	// Verify histogram recorded operations
	count := testutil.CollectAndCount(collector.cacheOpDuration)
	// 2 distinct combinations: (get,memory) and (set,memory)
	if count < 2 {
		t.Errorf("expected at least 2 histogram series, got %d", count)
	}
}

func TestPrometheusCollectorWithConfig(t *testing.T) {
	registry := prometheus.NewRegistry()

	collector := NewCollectorWithConfig(CollectorConfig{
		Registry:  registry,
		Namespace: "custom",
		Subsystem: "test",
		ConstLabels: prometheus.Labels{
			"service": "test-service",
			"region":  "us-west",
		},
	})

	collector.RecordCacheOperation("get", "redis", "hit", 1*time.Millisecond)

	// Verify custom namespace and const labels
	metrics, err := registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, m := range metrics {
		if *m.Name == "custom_test_cache_requests_total" {
			found = true
			// Verify const labels are present
			for _, metric := range m.Metric {
				labels := make(map[string]string)
				for _, label := range metric.Label {
					labels[*label.Name] = *label.Value
				}
				if labels["service"] != "test-service" || labels["region"] != "us-west" {
					t.Errorf("const labels not found or incorrect: %v", labels)
				}
			}
		}
	}

	if !found {
		t.Error("custom metric name not found")
	}
}

func TestInstrumentedCache(t *testing.T) {
	ctx := context.Background()
	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	baseCache := newMockCache()
	cache := NewInstrumentedCache(baseCache, "memory", collector)

	// Test Set operation
	if err := cache.Set(ctx, "key1", []byte("value1")); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Test Get operation (hit)
	value, ok, err := cache.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !ok || string(value) != "value1" {
		t.Errorf("cache Get failed: ok=%v, value=%s", ok, string(value))
	}

	// Test Get operation (miss)
	_, ok, err = cache.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if ok {
		t.Error("expected cache miss for nonexistent key")
	}

	// Test Delete operation
	if err := cache.Delete(ctx, "key1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify metrics were recorded
	expected := `
		# HELP httpcache_cache_requests_total Total number of cache operations
		# TYPE httpcache_cache_requests_total counter
		httpcache_cache_requests_total{cache_backend="memory",operation="delete",result="success"} 1
		httpcache_cache_requests_total{cache_backend="memory",operation="get",result="hit"} 1
		httpcache_cache_requests_total{cache_backend="memory",operation="get",result="miss"} 1
		httpcache_cache_requests_total{cache_backend="memory",operation="set",result="success"} 1
	`

	if err := testutil.CollectAndCompare(collector.cacheRequests, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected metrics: %v", err)
	}
}

func TestInstrumentedCacheWithNilCollector(t *testing.T) {
	ctx := context.Background()
	baseCache := newMockCache()

	// Should use NoOpCollector when nil is passed
	cache := NewInstrumentedCache(baseCache, "memory", nil)

	// Should not panic and should work normally
	if err := cache.Set(ctx, "key1", []byte("value1")); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	value, ok, err := cache.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !ok || string(value) != "value1" {
		t.Errorf("cache operations failed with nil collector")
	}
	if err := cache.Delete(ctx, "key1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

func TestRecordCacheSize(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	collector.RecordCacheSize("memory", 1024000)
	collector.RecordCacheSize("redis", 2048000)

	// Verify gauge metrics
	expected := `
		# HELP httpcache_cache_size_bytes Current size of cache in bytes
		# TYPE httpcache_cache_size_bytes gauge
		httpcache_cache_size_bytes{cache_backend="memory"} 1.024e+06
		httpcache_cache_size_bytes{cache_backend="redis"} 2.048e+06
	`

	if err := testutil.CollectAndCompare(collector.cacheSize, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected metrics: %v", err)
	}
}

func TestRecordCacheEntries(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	collector.RecordCacheEntries("memory", 150)
	collector.RecordCacheEntries("redis", 300)

	// Verify gauge metrics
	expected := `
		# HELP httpcache_cache_entries_total Current number of entries in cache
		# TYPE httpcache_cache_entries_total gauge
		httpcache_cache_entries_total{cache_backend="memory"} 150
		httpcache_cache_entries_total{cache_backend="redis"} 300
	`

	if err := testutil.CollectAndCompare(collector.cacheEntries, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected metrics: %v", err)
	}
}

func TestRecordHTTPRequest(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	collector.RecordHTTPRequest("GET", "hit", 200, 50*time.Millisecond)
	collector.RecordHTTPRequest("GET", "miss", 200, 200*time.Millisecond)
	collector.RecordHTTPRequest("POST", "bypass", 201, 100*time.Millisecond)

	// Verify counter metrics
	expected := `
		# HELP httpcache_http_requests_total Total number of HTTP requests
		# TYPE httpcache_http_requests_total counter
		httpcache_http_requests_total{cache_status="bypass",method="POST",status_code="201"} 1
		httpcache_http_requests_total{cache_status="hit",method="GET",status_code="200"} 1
		httpcache_http_requests_total{cache_status="miss",method="GET",status_code="200"} 1
	`

	if err := testutil.CollectAndCompare(collector.httpRequests, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected metrics: %v", err)
	}
}

func TestRecordHTTPResponseSize(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	collector.RecordHTTPResponseSize("hit", 1024)
	collector.RecordHTTPResponseSize("hit", 2048)
	collector.RecordHTTPResponseSize("miss", 4096)

	// Verify counter metrics (should accumulate)
	expected := `
		# HELP httpcache_http_response_size_bytes_total Total size of HTTP responses in bytes
		# TYPE httpcache_http_response_size_bytes_total counter
		httpcache_http_response_size_bytes_total{cache_status="hit"} 3072
		httpcache_http_response_size_bytes_total{cache_status="miss"} 4096
	`

	if err := testutil.CollectAndCompare(collector.httpResponseSize, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected metrics: %v", err)
	}
}

func TestRecordStaleResponse(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	collector.RecordStaleResponse("network")
	collector.RecordStaleResponse("server_error")
	collector.RecordStaleResponse("timeout")

	// Verify counter metrics
	expected := `
		# HELP httpcache_stale_responses_served_total Total number of stale responses served on error
		# TYPE httpcache_stale_responses_served_total counter
		httpcache_stale_responses_served_total{error_type="network"} 1
		httpcache_stale_responses_served_total{error_type="server_error"} 1
		httpcache_stale_responses_served_total{error_type="timeout"} 1
	`

	if err := testutil.CollectAndCompare(collector.staleResponses, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected metrics: %v", err)
	}
}
