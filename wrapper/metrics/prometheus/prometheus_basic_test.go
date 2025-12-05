//go:build integration

package prometheus

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	"github.com/sandrolain/httpcache"
)

const (
	skipIntegrationMsg = "skipping integration test; use -integration flag to enable"
	metricsPath        = "/metrics"
)

// TestPrometheusIntegrationMetricsExport tests that metrics are properly exported and scrapable
func TestPrometheusIntegrationMetricsExport(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	// Setup metrics server
	server, metricsURL := setupMetricsServer(registry)
	defer server.Close()

	// Record some cache operations
	collector.RecordCacheOperation("get", "memory", "hit", 1*time.Millisecond)
	collector.RecordCacheOperation("get", "memory", "miss", 2*time.Millisecond)
	collector.RecordCacheOperation("set", "memory", "success", 500*time.Microsecond)
	collector.RecordCacheSize("memory", 1024000)
	collector.RecordCacheEntries("memory", 150)

	// Scrape metrics
	metrics := scrapeMetrics(t, metricsURL)

	// Verify metrics are present in scraped output
	expectedMetrics := []string{
		"httpcache_cache_requests_total",
		"httpcache_cache_operation_duration_seconds",
		"httpcache_cache_size_bytes",
		"httpcache_cache_entries_total",
	}

	for _, metric := range expectedMetrics {
		if !containsMetric(metrics, metric) {
			t.Errorf("expected metric %s not found in scraped metrics", metric)
		}
	}
}

// TestPrometheusIntegrationHTTPTransport tests metrics collection with actual HTTP requests
func TestPrometheusIntegrationHTTPTransport(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	// Setup metrics server
	metricsServer, metricsURL := setupMetricsServer(registry)
	defer metricsServer.Close()

	// Create test HTTP server
	callCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Cache-Control", "max-age=300")
		w.Header().Set("Content-Length", "13")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer testServer.Close()

	// Create instrumented cache and transport
	baseCache := newMockCache()
	cache := NewInstrumentedCache(baseCache, "memory", collector)
	transport := httpcache.NewTransport(cache)
	instrumentedTransport := NewInstrumentedTransport(transport, collector)

	client := instrumentedTransport.Client()

	// Make first request (cache miss)
	resp1, err := client.Get(testServer.URL)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	// Make second request (cache hit)
	resp2, err := client.Get(testServer.URL)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()

	// Verify only one actual HTTP call was made (second was cached)
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call, got %d", callCount)
	}

	// Scrape metrics
	metrics := scrapeMetrics(t, metricsURL)

	// Verify HTTP request metrics
	if !containsMetric(metrics, "httpcache_http_requests_total") {
		t.Error("HTTP request metrics not found")
	}

	// Verify cache hit and miss metrics
	hitValue := getMetricValue(t, registry, "httpcache_cache_requests_total", map[string]string{
		"operation": "get",
		"result":    "hit",
	})
	missValue := getMetricValue(t, registry, "httpcache_cache_requests_total", map[string]string{
		"operation": "get",
		"result":    "miss",
	})

	if hitValue < 1 {
		t.Errorf("expected at least 1 cache hit, got %v", hitValue)
	}
	if missValue < 1 {
		t.Errorf("expected at least 1 cache miss, got %v", missValue)
	}
}

// TestPrometheusIntegrationMultipleBackends tests metrics with multiple cache backends
func TestPrometheusIntegrationMultipleBackends(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	// Setup metrics server
	server, metricsURL := setupMetricsServer(registry)
	defer server.Close()

	// Simulate operations on different backends
	backends := []string{"memory", "redis", "postgresql"}
	for _, backend := range backends {
		collector.RecordCacheOperation("get", backend, "hit", 1*time.Millisecond)
		collector.RecordCacheSize(backend, 1024000)
		collector.RecordCacheEntries(backend, 100)
	}

	// Scrape metrics
	metrics := scrapeMetrics(t, metricsURL)

	// Verify metrics for each backend
	for _, backend := range backends {
		expectedLabel := fmt.Sprintf(`cache_backend="%s"`, backend)
		if !containsString(metrics, expectedLabel) {
			t.Errorf("expected metrics for backend %s not found", backend)
		}
	}

	// Verify metric values
	for _, backend := range backends {
		hitValue := getMetricValue(t, registry, "httpcache_cache_requests_total", map[string]string{
			"cache_backend": backend,
			"operation":     "get",
			"result":        "hit",
		})
		if hitValue != 1 {
			t.Errorf("expected 1 hit for backend %s, got %v", backend, hitValue)
		}

		sizeValue := getMetricValue(t, registry, "httpcache_cache_size_bytes", map[string]string{
			"cache_backend": backend,
		})
		if sizeValue != 1024000 {
			t.Errorf("expected size 1024000 for backend %s, got %v", backend, sizeValue)
		}
	}
}

// TestPrometheusIntegrationConcurrentMetrics tests thread-safety of metrics collection
func TestPrometheusIntegrationConcurrentMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	// Setup metrics server
	server, _ := setupMetricsServer(registry)
	defer server.Close()

	// Create instrumented cache
	baseCache := newMockCache()
	cache := NewInstrumentedCache(baseCache, "memory", collector)

	// Run concurrent operations
	const numGoroutines = 10
	const numOperations = 100
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				value := []byte(fmt.Sprintf("value-%d-%d", id, j))

				cache.Set(key, value)
				cache.Get(key)
				cache.Delete(key)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify total operations
	totalSets := getMetricValue(t, registry, "httpcache_cache_requests_total", map[string]string{
		"operation": "set",
		"result":    "success",
	})
	totalGets := getMetricValue(t, registry, "httpcache_cache_requests_total", map[string]string{
		"operation": "get",
	})
	totalDeletes := getMetricValue(t, registry, "httpcache_cache_requests_total", map[string]string{
		"operation": "delete",
		"result":    "success",
	})

	expectedTotal := float64(numGoroutines * numOperations)
	if totalSets != expectedTotal {
		t.Errorf("expected %v set operations, got %v", expectedTotal, totalSets)
	}
	if totalGets < expectedTotal {
		t.Errorf("expected at least %v get operations, got %v", expectedTotal, totalGets)
	}
	if totalDeletes != expectedTotal {
		t.Errorf("expected %v delete operations, got %v", expectedTotal, totalDeletes)
	}
}

// TestPrometheusIntegrationMetricsReset tests that metrics can be reset and re-registered
func TestPrometheusIntegrationMetricsReset(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	// Record some metrics
	collector.RecordCacheOperation("get", "memory", "hit", 1*time.Millisecond)

	// Setup metrics server
	server, metricsURL := setupMetricsServer(registry)
	defer server.Close()

	// Verify metrics exist
	metrics1 := scrapeMetrics(t, metricsURL)
	if !containsMetric(metrics1, "httpcache_cache_requests_total") {
		t.Error("initial metrics not found")
	}

	// Create new registry and collector
	newRegistry := prometheus.NewRegistry()
	newCollector := NewCollectorWithRegistry(newRegistry)

	// Record different metrics
	newCollector.RecordCacheOperation("set", "redis", "success", 2*time.Millisecond)

	// Setup new metrics server
	newServer, newMetricsURL := setupMetricsServer(newRegistry)
	defer newServer.Close()

	// Verify new metrics
	metrics2 := scrapeMetrics(t, newMetricsURL)
	if !containsMetric(metrics2, "httpcache_cache_requests_total") {
		t.Error("new metrics not found")
	}

	// Verify the new metric exists (set operation on redis)
	setValue := getMetricValue(t, newRegistry, "httpcache_cache_requests_total", map[string]string{
		"operation": "set",
		"result":    "success",
	})
	if setValue != 1 {
		t.Errorf("expected 1 for new metric, got %v", setValue)
	}

	// Verify the old registry still has its metrics
	oldHitValue := getMetricValue(t, registry, "httpcache_cache_requests_total", map[string]string{
		"operation": "get",
		"result":    "hit",
	})
	if oldHitValue != 1 {
		t.Errorf("expected 1 for old metric in old registry, got %v", oldHitValue)
	}
}

// TestPrometheusIntegrationCustomNamespace tests metrics with custom namespace and labels
func TestPrometheusIntegrationCustomNamespace(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	registry := prometheus.NewRegistry()
	collector := NewCollectorWithConfig(CollectorConfig{
		Registry:  registry,
		Namespace: "myapp",
		Subsystem: "cache",
		ConstLabels: prometheus.Labels{
			"environment": "test",
			"region":      "us-west",
		},
	})

	// Record metrics
	collector.RecordCacheOperation("get", "memory", "hit", 1*time.Millisecond)

	// Setup metrics server
	server, metricsURL := setupMetricsServer(registry)
	defer server.Close()

	// Scrape metrics
	metrics := scrapeMetrics(t, metricsURL)

	// Verify custom namespace
	if !containsMetric(metrics, "myapp_cache_cache_requests_total") {
		t.Error("custom namespace metric not found")
	}

	// Verify const labels
	if !containsString(metrics, `environment="test"`) {
		t.Error("environment label not found")
	}
	if !containsString(metrics, `region="us-west"`) {
		t.Error("region label not found")
	}
}

// TestPrometheusIntegrationStaleResponses tests stale response metrics
func TestPrometheusIntegrationStaleResponses(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	// Setup metrics server
	server, metricsURL := setupMetricsServer(registry)
	defer server.Close()

	// Record stale responses
	errorTypes := []string{"network", "timeout", "server_error"}
	for _, errType := range errorTypes {
		collector.RecordStaleResponse(errType)
	}

	// Scrape metrics
	metrics := scrapeMetrics(t, metricsURL)

	// Verify stale response metrics
	if !containsMetric(metrics, "httpcache_stale_responses_served_total") {
		t.Error("stale response metrics not found")
	}

	// Verify each error type
	for _, errType := range errorTypes {
		value := getMetricValue(t, registry, "httpcache_stale_responses_served_total", map[string]string{
			"error_type": errType,
		})
		if value != 1 {
			t.Errorf("expected 1 stale response for error type %s, got %v", errType, value)
		}
	}
}

// TestPrometheusIntegrationHistogramBuckets tests that histogram buckets are properly configured
func TestPrometheusIntegrationHistogramBuckets(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	// Record operations with different durations
	durations := []time.Duration{
		100 * time.Microsecond,
		1 * time.Millisecond,
		10 * time.Millisecond,
		100 * time.Millisecond,
		1 * time.Second,
	}

	for _, duration := range durations {
		collector.RecordCacheOperation("get", "memory", "hit", duration)
		collector.RecordHTTPRequest("GET", "hit", 200, duration)
	}

	// Gather metrics
	metrics, err := registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	// Verify histogram metrics exist and have buckets
	histogramFound := false
	for _, m := range metrics {
		if m.GetType() == dto.MetricType_HISTOGRAM {
			histogramFound = true
			for _, metric := range m.GetMetric() {
				buckets := metric.GetHistogram().GetBucket()
				if len(buckets) == 0 {
					t.Error("histogram has no buckets")
				}
				// Verify sample count matches our recordings
				sampleCount := metric.GetHistogram().GetSampleCount()
				if sampleCount != uint64(len(durations)) {
					t.Errorf("expected %d samples, got %d", len(durations), sampleCount)
				}
			}
		}
	}

	if !histogramFound {
		t.Error("no histogram metrics found")
	}
}

// Helper functions

// setupMetricsServer creates an HTTP server that exposes Prometheus metrics
func setupMetricsServer(registry *prometheus.Registry) (*httptest.Server, string) {
	mux := http.NewServeMux()
	mux.Handle(metricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	server := httptest.NewServer(mux)
	metricsURL := server.URL + metricsPath

	return server, metricsURL
}

// scrapeMetrics retrieves metrics from the metrics endpoint
func scrapeMetrics(t *testing.T, url string) string {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to scrape metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read metrics body: %v", err)
	}

	return string(body)
}

// getMetricValue retrieves the value of a specific metric from the registry
func getMetricValue(t *testing.T, registry *prometheus.Registry, metricName string, labels map[string]string) float64 {
	t.Helper()

	metrics, err := registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	for _, m := range metrics {
		if m.GetName() != metricName {
			continue
		}

		for _, metric := range m.GetMetric() {
			if matchLabels(metric.GetLabel(), labels) {
				switch m.GetType() {
				case dto.MetricType_COUNTER:
					return metric.GetCounter().GetValue()
				case dto.MetricType_GAUGE:
					return metric.GetGauge().GetValue()
				case dto.MetricType_HISTOGRAM:
					return float64(metric.GetHistogram().GetSampleCount())
				}
			}
		}
	}

	t.Fatalf("metric %s with labels %v not found", metricName, labels)
	return 0
}

// matchLabels checks if metric labels match the expected labels
func matchLabels(metricLabels []*dto.LabelPair, expectedLabels map[string]string) bool {
	if len(expectedLabels) == 0 {
		return true
	}

	labelMap := make(map[string]string)
	for _, label := range metricLabels {
		labelMap[label.GetName()] = label.GetValue()
	}

	for key, value := range expectedLabels {
		if labelMap[key] != value {
			return false
		}
	}

	return true
}

// containsMetric checks if a metric name exists in the scraped metrics output
func containsMetric(metrics, metricName string) bool {
	return len(metrics) > 0 && containsString(metrics, metricName)
}

// containsString is a simple helper to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

// findSubstring finds a substring in a string
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
