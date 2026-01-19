package prometheus

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/diskcache"
)

func TestCollector_BasicMetrics(t *testing.T) {
	// Create a new registry for isolation
	reg := prometheus.NewRegistry()

	// Create internal metrics
	metrics := httpcache.NewTransportMetrics()

	// Simulate some activity
	metrics.CacheHits.Add(10)
	metrics.CacheMisses.Add(5)
	metrics.CacheErrors.Add(2)
	metrics.StaleServed.Add(1)
	metrics.Deduplication.Add(3)
	metrics.CachedBytes.Add(1024)

	// Create Prometheus collector
	collector := NewCollector(CollectorConfig{
		Metrics:  metrics,
		Registry: reg,
	})

	// Verify metrics are exported
	expectedMetrics := `
# HELP httpcache_cache_errors_total Total number of cache operation errors
# TYPE httpcache_cache_errors_total gauge
httpcache_cache_errors_total 2
# HELP httpcache_cache_hit_rate Cache hit rate (0-1)
# TYPE httpcache_cache_hit_rate gauge
httpcache_cache_hit_rate 0.6666666666666666
# HELP httpcache_cache_hits_total Total number of cache hits
# TYPE httpcache_cache_hits_total gauge
httpcache_cache_hits_total 10
# HELP httpcache_cache_misses_total Total number of cache misses
# TYPE httpcache_cache_misses_total gauge
httpcache_cache_misses_total 5
# HELP httpcache_cached_bytes Approximate number of bytes currently cached
# TYPE httpcache_cached_bytes gauge
httpcache_cached_bytes 1024
# HELP httpcache_deduplication_total Total number of requests deduplicated via singleflight
# TYPE httpcache_deduplication_total gauge
httpcache_deduplication_total 3
# HELP httpcache_stale_served_total Total number of stale responses served
# TYPE httpcache_stale_served_total gauge
httpcache_stale_served_total 1
# HELP httpcache_total_requests Total number of cache requests (hits + misses)
# TYPE httpcache_total_requests gauge
httpcache_total_requests 15
`

	if err := testutil.GatherAndCompare(reg, strings.NewReader(expectedMetrics)); err != nil {
		t.Errorf("Metrics mismatch: %v", err)
	}

	// Test Update method
	metrics.CacheHits.Add(5)
	collector.Update()

	if count := testutil.CollectAndCount(reg, "httpcache_cache_hits_total"); count != 1 {
		t.Errorf("Expected 1 httpcache_cache_hits_total metric, got %d", count)
	}
}

func TestCollector_AutoUpdate(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := httpcache.NewTransportMetrics()

	collector := NewCollector(CollectorConfig{
		Metrics:        metrics,
		Registry:       reg,
		UpdateInterval: 100 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := collector.Start(ctx)
	defer stop()

	// Add metrics after start
	metrics.CacheHits.Add(5)

	// Wait for update
	time.Sleep(150 * time.Millisecond)

	// Check if metrics were updated
	metricFamilies, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "httpcache_cache_hits_total" {
			found = true
			if len(mf.GetMetric()) > 0 {
				value := mf.GetMetric()[0].GetGauge().GetValue()
				if value != 5 {
					t.Errorf("Expected cache_hits_total = 5, got %v", value)
				}
			}
		}
	}

	if !found {
		t.Error("httpcache_cache_hits_total metric not found")
	}
}

func TestCollector_Integration(t *testing.T) {
	tmpDir := t.TempDir()
	cache := diskcache.New(tmpDir)

	// Create internal metrics
	metrics := httpcache.NewTransportMetrics()

	// Create transport with metrics
	transport := httpcache.NewTransport(cache, httpcache.WithMetrics(metrics))

	// Create test server
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
	}))
	defer server.Close()

	client := &http.Client{Transport: transport}

	// Make first request (cache miss)
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	// Make second request (cache hit)
	resp2, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}
	_, _ = io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()

	// Verify backend was called only once
	if requestCount != 1 {
		t.Errorf("Expected 1 backend request, got %d", requestCount)
	}

	// Create Prometheus collector and verify metrics
	reg := prometheus.NewRegistry()
	collector := NewCollector(CollectorConfig{
		Metrics:  metrics,
		Registry: reg,
	})
	collector.Update()

	// Verify cache hit and miss counters
	metricFamilies, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	metricsMap := make(map[string]float64)
	for _, mf := range metricFamilies {
		if len(mf.GetMetric()) > 0 {
			metricsMap[mf.GetName()] = mf.GetMetric()[0].GetGauge().GetValue()
		}
	}

	if hits := metricsMap["httpcache_cache_hits_total"]; hits != 1 {
		t.Errorf("Expected cache_hits_total = 1, got %v", hits)
	}
	if misses := metricsMap["httpcache_cache_misses_total"]; misses != 1 {
		t.Errorf("Expected cache_misses_total = 1, got %v", misses)
	}
	if hitRate := metricsMap["httpcache_cache_hit_rate"]; hitRate != 0.5 {
		t.Errorf("Expected cache_hit_rate = 0.5, got %v", hitRate)
	}
}

func TestCollector_HTTPEndpoint(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := httpcache.NewTransportMetrics()

	// Populate some metrics
	metrics.CacheHits.Add(100)
	metrics.CacheMisses.Add(50)

	NewCollector(CollectorConfig{
		Metrics:  metrics,
		Registry: reg,
	})

	// Create HTTP handler for metrics endpoint
	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})

	// Test metrics endpoint
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "httpcache_cache_hits_total") {
		t.Error("Metrics endpoint doesn't contain cache_hits_total")
	}
	if !strings.Contains(bodyStr, "httpcache_cache_misses_total") {
		t.Error("Metrics endpoint doesn't contain cache_misses_total")
	}
}

func TestCollector_CustomNamespace(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := httpcache.NewTransportMetrics()
	metrics.CacheHits.Add(5)

	NewCollector(CollectorConfig{
		Metrics:   metrics,
		Registry:  reg,
		Namespace: "myapp",
		Subsystem: "cache",
	})

	// Verify custom namespace is used
	metricFamilies, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "myapp_cache_cache_hits_total" {
			found = true
		}
	}

	if !found {
		t.Error("Expected metric with custom namespace 'myapp_cache_cache_hits_total' not found")
	}
}

func TestCollector_ContextCancellation(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := httpcache.NewTransportMetrics()

	collector := NewCollector(CollectorConfig{
		Metrics:        metrics,
		Registry:       reg,
		UpdateInterval: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())

	stop := collector.Start(ctx)

	// Wait a bit for the goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for goroutine to stop
	time.Sleep(100 * time.Millisecond)

	// Verify we can still call stop without panic
	stop()
}

func TestNewCollectorWithRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := httpcache.NewTransportMetrics()
	metrics.CacheHits.Add(1)

	collector := NewCollectorWithRegistry(reg, metrics)

	if collector == nil {
		t.Fatal("Expected non-nil collector")
	}

	// Verify metrics are registered
	if count := testutil.CollectAndCount(reg, "httpcache_cache_hits_total"); count != 1 {
		t.Errorf("Expected 1 metric, got %d", count)
	}
}

func TestCollector_PanicOnNilMetrics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when Metrics is nil")
		}
	}()

	NewCollector(CollectorConfig{
		Metrics: nil,
	})
}
