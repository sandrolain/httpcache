//go:build integration

package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sandrolain/httpcache"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	prometheusImage = "prom/prometheus:v2.54.1"
	scrapeInterval  = "2s"
)

var (
	// Shared Prometheus container used by all tests
	sharedPrometheusContainer testcontainers.Container
	sharedPrometheusURL       string
)

// TestMain sets up the Prometheus container once for all tests
func TestMain(m *testing.M) {
	ctx := context.Background()

	// Setup a temporary metrics server for Prometheus to scrape
	// We'll use a dummy one during setup, real tests will create their own
	dummyRegistry := prometheus.NewRegistry()
	dummyServer, dummyURL := setupMetricsServer(dummyRegistry)
	dummyHost, dummyPort := extractHostPort(dummyURL)

	// Start shared Prometheus container
	container, prometheusURL, cleanup, err := setupPrometheusContainer(ctx, dummyHost, dummyPort)
	if err != nil {
		dummyServer.Close()
		fmt.Fprintf(os.Stderr, "Failed to start Prometheus container: %v\n", err)
		os.Exit(1)
	}

	sharedPrometheusContainer = container
	sharedPrometheusURL = prometheusURL

	// Close dummy server as it's no longer needed
	dummyServer.Close()

	// Run tests
	code := m.Run()

	// Cleanup
	cleanup()

	os.Exit(code)
}

// prometheusConfig generates a Prometheus configuration file content
func prometheusConfig(metricsHost, metricsPort string) string {
	return fmt.Sprintf(`
global:
  scrape_interval: %s
  evaluation_interval: %s

scrape_configs:
  - job_name: 'httpcache'
    metrics_path: '%s'
    static_configs:
      - targets: ['%s:%s']
`, scrapeInterval, scrapeInterval, metricsPath, metricsHost, metricsPort)
}

// setupPrometheusContainer starts a Prometheus container and returns the container and API URL
func setupPrometheusContainer(ctx context.Context, metricsHost, metricsPort string) (testcontainers.Container, string, func(), error) {
	// Create temporary prometheus.yml file
	configContent := prometheusConfig(metricsHost, metricsPort)
	tmpFile, err := os.CreateTemp("", "prometheus-*.yml")
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to create temp config file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(configContent); err != nil {
		os.Remove(tmpFile.Name())
		return nil, "", nil, fmt.Errorf("failed to write config file: %w", err)
	}

	// Start Prometheus container
	req := testcontainers.ContainerRequest{
		Image:        prometheusImage,
		ExposedPorts: []string{"9090/tcp"},
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      tmpFile.Name(),
				ContainerFilePath: "/etc/prometheus/prometheus.yml",
				FileMode:          0o644,
			},
		},
		WaitingFor: wait.ForHTTP("/").
			WithPort("9090/tcp").
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		os.Remove(tmpFile.Name())
		return nil, "", nil, fmt.Errorf("failed to start Prometheus container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		os.Remove(tmpFile.Name())
		return nil, "", nil, fmt.Errorf("failed to get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "9090")
	if err != nil {
		container.Terminate(ctx)
		os.Remove(tmpFile.Name())
		return nil, "", nil, fmt.Errorf("failed to get container port: %w", err)
	}

	prometheusURL := fmt.Sprintf("http://%s:%s", host, port.Port())

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "failed to terminate Prometheus container: %v\n", err)
		}
		os.Remove(tmpFile.Name())
	}

	return container, prometheusURL, cleanup, nil
}

// queryPrometheus queries the Prometheus API
func queryPrometheus(t *testing.T, prometheusURL, query string) ([]PrometheusResult, error) {
	t.Helper()

	url := fmt.Sprintf("%s/api/v1/query?query=%s", prometheusURL, query)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to query Prometheus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var response PrometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Status != "success" {
		return nil, fmt.Errorf("query failed: %s", response.Status)
	}

	return response.Data.Result, nil
}

// PrometheusResponse represents the Prometheus API response
type PrometheusResponse struct {
	Status string         `json:"status"`
	Data   PrometheusData `json:"data"`
}

// PrometheusData represents the data section of Prometheus response
type PrometheusData struct {
	ResultType string             `json:"resultType"`
	Result     []PrometheusResult `json:"result"`
}

// PrometheusResult represents a single result from Prometheus
type PrometheusResult struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"`
}

// getMetricValueFromResult extracts the numeric value from a Prometheus result
func getMetricValueFromResult(result PrometheusResult) (float64, error) {
	if len(result.Value) < 2 {
		return 0, fmt.Errorf("invalid result value")
	}

	valueStr, ok := result.Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("value is not a string")
	}

	var value float64
	_, err := fmt.Sscanf(valueStr, "%f", &value)
	return value, err
}

// extractHostPort extracts host and port from metrics URL for Prometheus config
func extractHostPort(metricsURL string) (host, port string) {
	// metricsURL is like "http://127.0.0.1:12345/metrics"
	urlWithoutScheme := strings.TrimPrefix(metricsURL, "http://")
	urlWithoutScheme = strings.TrimPrefix(urlWithoutScheme, "https://")

	// Remove the path part
	hostPort := strings.Split(urlWithoutScheme, "/")[0]

	// Split into host and port
	parts := strings.Split(hostPort, ":")

	// Use host.docker.internal to allow container to access host
	host = "host.docker.internal"
	port = "80"

	if len(parts) == 2 {
		port = parts[1]
	}

	return host, port
}

// TestPrometheusIntegrationRealPrometheusServer tests metrics collection with a real Prometheus instance
func TestPrometheusIntegrationRealPrometheusServer(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	// Setup metrics registry and collector
	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	// Setup metrics server
	metricsServer, metricsURL := setupMetricsServer(registry)
	defer metricsServer.Close()

	// Extract host and port from metrics URL to update Prometheus config
	metricsHost, metricsPort := extractHostPort(metricsURL)

	// Reconfigure Prometheus to scrape this test's metrics endpoint
	ctx := context.Background()
	configContent := prometheusConfig(metricsHost, metricsPort)
	tmpFile, err := os.CreateTemp("", "prometheus-test-*.yml")
	if err != nil {
		t.Fatalf("failed to create temp config: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	tmpFile.Close()

	// Copy config to container
	if err := sharedPrometheusContainer.CopyFileToContainer(ctx, tmpFile.Name(), "/etc/prometheus/prometheus.yml", 0o644); err != nil {
		t.Fatalf("failed to copy config to container: %v", err)
	}

	// Reload Prometheus configuration
	if _, _, err := sharedPrometheusContainer.Exec(ctx, []string{"kill", "-HUP", "1"}); err != nil {
		t.Logf("warning: failed to reload Prometheus config: %v", err)
	}

	// Record some metrics
	collector.RecordCacheOperation("get", "memory", "hit", 1*time.Millisecond)
	collector.RecordCacheOperation("get", "memory", "miss", 2*time.Millisecond)
	collector.RecordCacheOperation("set", "memory", "success", 500*time.Microsecond)
	collector.RecordCacheSize("memory", 2048000)
	collector.RecordCacheEntries("memory", 250)

	// Wait for Prometheus to scrape metrics (at least 3 scrape intervals to be safe)
	time.Sleep(8 * time.Second)

	// Query Prometheus for cache requests
	results, err := queryPrometheus(t, sharedPrometheusURL, "httpcache_cache_requests_total")
	if err != nil {
		t.Fatalf("failed to query Prometheus: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("no metrics found in Prometheus")
	}

	// Verify we have metrics for hit, miss, and success
	foundHit := false
	foundMiss := false
	foundSuccess := false

	for _, result := range results {
		operation := result.Metric["operation"]
		resultType := result.Metric["result"]

		if operation == "get" && resultType == "hit" {
			foundHit = true
			value, err := getMetricValueFromResult(result)
			if err != nil {
				t.Errorf("failed to get value: %v", err)
			}
			if value < 1 {
				t.Errorf("expected at least 1 hit, got %v", value)
			}
		}
		if operation == "get" && resultType == "miss" {
			foundMiss = true
			value, err := getMetricValueFromResult(result)
			if err != nil {
				t.Errorf("failed to get value: %v", err)
			}
			if value < 1 {
				t.Errorf("expected at least 1 miss, got %v", value)
			}
		}
		if operation == "set" && resultType == "success" {
			foundSuccess = true
		}
	}

	if !foundHit {
		t.Error("hit metric not found in Prometheus")
	}
	if !foundMiss {
		t.Error("miss metric not found in Prometheus")
	}
	if !foundSuccess {
		t.Error("success metric not found in Prometheus")
	}
}

// TestPrometheusIntegrationRealHTTPWorkflow tests a complete HTTP caching workflow with Prometheus
func TestPrometheusIntegrationRealHTTPWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	// Setup metrics registry and collector
	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	// Setup metrics server
	metricsServer, metricsURL := setupMetricsServer(registry)
	defer metricsServer.Close()

	// Reconfigure Prometheus for this test
	ctx := context.Background()
	metricsHost, metricsPort := extractHostPort(metricsURL)
	configContent := prometheusConfig(metricsHost, metricsPort)
	tmpFile, err := os.CreateTemp("", "prometheus-test-*.yml")
	if err != nil {
		t.Fatalf("failed to create temp config: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	tmpFile.Close()

	if err := sharedPrometheusContainer.CopyFileToContainer(ctx, tmpFile.Name(), "/etc/prometheus/prometheus.yml", 0o644); err != nil {
		t.Fatalf("failed to copy config to container: %v", err)
	}

	if _, _, err := sharedPrometheusContainer.Exec(ctx, []string{"kill", "-HUP", "1"}); err != nil {
		t.Logf("warning: failed to reload Prometheus config: %v", err)
	}

	// Create test HTTP server
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "max-age=300")
		w.Header().Set("Content-Length", "20")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("cached test response"))
	}))
	defer testServer.Close()

	// Create instrumented cache and transport
	baseCache := httpcache.NewMemoryCache()
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

	// Make third request (cache hit)
	resp3, err := client.Get(testServer.URL)
	if err != nil {
		t.Fatalf("third request failed: %v", err)
	}
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()

	// Verify only one actual HTTP call was made
	if requestCount != 1 {
		t.Errorf("expected 1 HTTP call, got %d", requestCount)
	}

	// Wait for Prometheus to scrape metrics
	time.Sleep(6 * time.Second)

	// Query Prometheus for HTTP request metrics
	results, err := queryPrometheus(t, sharedPrometheusURL, "httpcache_http_requests_total")
	if err != nil {
		t.Fatalf("failed to query HTTP metrics: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("no HTTP metrics found in Prometheus")
	}

	// Verify cache hits and misses
	var hitCount, missCount float64
	for _, result := range results {
		cacheStatus := result.Metric["cache_status"]
		value, err := getMetricValueFromResult(result)
		if err != nil {
			t.Errorf("failed to get value: %v", err)
			continue
		}

		switch cacheStatus {
		case "hit":
			hitCount = value
		case "miss":
			missCount = value
		}
	}

	if hitCount < 2 {
		t.Errorf("expected at least 2 cache hits, got %v", hitCount)
	}
	if missCount < 1 {
		t.Errorf("expected at least 1 cache miss, got %v", missCount)
	}

	// Query for response size metrics
	sizeResults, err := queryPrometheus(t, sharedPrometheusURL, "httpcache_http_response_size_bytes_total")
	if err != nil {
		t.Fatalf("failed to query size metrics: %v", err)
	}

	if len(sizeResults) == 0 {
		t.Fatal("no response size metrics found in Prometheus")
	}

	// Verify response sizes
	var totalSize float64
	for _, result := range sizeResults {
		value, err := getMetricValueFromResult(result)
		if err != nil {
			continue
		}
		totalSize += value
	}

	// 3 requests * 20 bytes = 60 bytes minimum
	minExpectedSize := float64(60)
	if totalSize < minExpectedSize {
		t.Errorf("expected total response size at least %v, got %v", minExpectedSize, totalSize)
	}
}

// TestPrometheusIntegrationMultipleBackendsRealPrometheus tests multiple backends with real Prometheus
func TestPrometheusIntegrationMultipleBackendsRealPrometheus(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	// Setup metrics registry and collector
	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	// Setup metrics server
	metricsServer, metricsURL := setupMetricsServer(registry)
	defer metricsServer.Close()

	// Reconfigure Prometheus for this test
	ctx := context.Background()
	metricsHost, metricsPort := extractHostPort(metricsURL)
	configContent := prometheusConfig(metricsHost, metricsPort)
	tmpFile, err := os.CreateTemp("", "prometheus-test-*.yml")
	if err != nil {
		t.Fatalf("failed to create temp config: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	tmpFile.Close()

	if err := sharedPrometheusContainer.CopyFileToContainer(ctx, tmpFile.Name(), "/etc/prometheus/prometheus.yml", 0o644); err != nil {
		t.Fatalf("failed to copy config to container: %v", err)
	}

	if _, _, err := sharedPrometheusContainer.Exec(ctx, []string{"kill", "-HUP", "1"}); err != nil {
		t.Logf("warning: failed to reload Prometheus config: %v", err)
	}

	// Simulate operations on different backends
	backends := []string{"memory", "redis", "postgresql"}
	for _, backend := range backends {
		collector.RecordCacheOperation("get", backend, "hit", 1*time.Millisecond)
		collector.RecordCacheOperation("set", backend, "success", 500*time.Microsecond)
		collector.RecordCacheSize(backend, 1024000)
		collector.RecordCacheEntries(backend, 100)
	}

	// Wait for Prometheus to scrape metrics
	time.Sleep(6 * time.Second)

	// Query Prometheus for cache size metrics
	results, err := queryPrometheus(t, sharedPrometheusURL, "httpcache_cache_size_bytes")
	if err != nil {
		t.Fatalf("failed to query cache size: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("no cache size metrics found in Prometheus")
	}

	// Verify each backend
	foundBackends := make(map[string]bool)
	for _, result := range results {
		backend := result.Metric["cache_backend"]
		if backend != "" {
			foundBackends[backend] = true

			value, err := getMetricValueFromResult(result)
			if err != nil {
				t.Errorf("failed to get value for backend %s: %v", backend, err)
				continue
			}

			if value < 1024000 {
				t.Errorf("expected at least size 1024000 for backend %s, got %v", backend, value)
			}
		}
	}

	for _, backend := range backends {
		if !foundBackends[backend] {
			t.Errorf("backend %s not found in Prometheus metrics", backend)
		}
	}
}

// TestPrometheusIntegrationHistogramInRealPrometheus tests histogram metrics in real Prometheus
func TestPrometheusIntegrationHistogramInRealPrometheus(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationMsg)
	}

	// Setup metrics registry and collector
	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	// Setup metrics server
	metricsServer, metricsURL := setupMetricsServer(registry)
	defer metricsServer.Close()

	// Reconfigure Prometheus for this test
	ctx := context.Background()
	metricsHost, metricsPort := extractHostPort(metricsURL)
	configContent := prometheusConfig(metricsHost, metricsPort)
	tmpFile, err := os.CreateTemp("", "prometheus-test-*.yml")
	if err != nil {
		t.Fatalf("failed to create temp config: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	tmpFile.Close()

	if err := sharedPrometheusContainer.CopyFileToContainer(ctx, tmpFile.Name(), "/etc/prometheus/prometheus.yml", 0o644); err != nil {
		t.Fatalf("failed to copy config to container: %v", err)
	}

	if _, _, err := sharedPrometheusContainer.Exec(ctx, []string{"kill", "-HUP", "1"}); err != nil {
		t.Logf("warning: failed to reload Prometheus config: %v", err)
	}

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
	}

	// Wait for Prometheus to scrape metrics
	time.Sleep(6 * time.Second)

	// Query Prometheus for histogram count
	results, err := queryPrometheus(t, sharedPrometheusURL, "httpcache_cache_operation_duration_seconds_count")
	if err != nil {
		t.Fatalf("failed to query histogram count: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("no histogram count metrics found in Prometheus")
	}

	// Verify sample count
	for _, result := range results {
		if result.Metric["operation"] == "get" {
			value, err := getMetricValueFromResult(result)
			if err != nil {
				t.Errorf("failed to get value: %v", err)
				continue
			}

			if value < float64(len(durations)) {
				t.Errorf("expected at least %d samples, got %v", len(durations), value)
			}
		}
	}
}
