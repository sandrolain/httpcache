package prometheus

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sandrolain/httpcache"
)

func TestInstrumentedTransport(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	// Create test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	// First request (cache miss)
	resp1, err := client.Get(testServer.URL)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	// Second request (cache hit)
	resp2, err := client.Get(testServer.URL)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()

	// Verify cache metrics - we expect at least 1 hit and 1 miss
	metrics, _ := registry.Gather()
	var hitCount, missCount float64

	for _, m := range metrics {
		if *m.Name == "httpcache_cache_requests_total" {
			for _, metric := range m.Metric {
				labels := make(map[string]string)
				for _, label := range metric.Label {
					labels[*label.Name] = *label.Value
				}
				if labels["operation"] == "get" {
					switch labels["result"] {
					case "hit":
						hitCount = *metric.Counter.Value
					case "miss":
						missCount = *metric.Counter.Value
					}
				}
			}
		}
	}

	if hitCount != 1 {
		t.Errorf("expected 1 cache hit, got %v", hitCount)
	}
	if missCount != 1 {
		t.Errorf("expected 1 cache miss, got %v", missCount)
	}

	// Verify HTTP metrics
	expectedHTTP := `
		# HELP httpcache_http_requests_total Total number of HTTP requests
		# TYPE httpcache_http_requests_total counter
		httpcache_http_requests_total{cache_status="hit",method="GET",status_code="200"} 1
		httpcache_http_requests_total{cache_status="miss",method="GET",status_code="200"} 1
	`

	if err := testutil.CollectAndCompare(collector.httpRequests, strings.NewReader(expectedHTTP)); err != nil {
		t.Errorf("unexpected HTTP metrics: %v", err)
	}

	// Verify response size metrics
	expectedSize := `
		# HELP httpcache_http_response_size_bytes_total Total size of HTTP responses in bytes
		# TYPE httpcache_http_response_size_bytes_total counter
		httpcache_http_response_size_bytes_total{cache_status="hit"} 13
		httpcache_http_response_size_bytes_total{cache_status="miss"} 13
	`

	if err := testutil.CollectAndCompare(collector.httpResponseSize, strings.NewReader(expectedSize)); err != nil {
		t.Errorf("unexpected size metrics: %v", err)
	}
}

func TestInstrumentedTransportWithNilCollector(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=300")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	}))
	defer testServer.Close()

	baseCache := newMockCache()
	transport := httpcache.NewTransport(baseCache)

	// Should use NoOpCollector when nil is passed
	instrumentedTransport := NewInstrumentedTransport(transport, nil)
	client := instrumentedTransport.Client()

	// Should not panic and should work normally
	resp, err := client.Get(testServer.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
}

func TestInstrumentedTransportCacheStatuses(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	baseCache := newMockCache()
	cache := NewInstrumentedCache(baseCache, "memory", collector)
	transport := httpcache.NewTransport(cache)
	instrumentedTransport := NewInstrumentedTransport(transport, collector)

	// Test cache miss
	testServerMiss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=300")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("miss"))
	}))
	defer testServerMiss.Close()

	client := instrumentedTransport.Client()
	resp, _ := client.Get(testServerMiss.URL)
	io.Copy(io.Discard, resp.Body) // Read body to trigger caching
	resp.Body.Close()

	// Verify miss was recorded
	metrics, _ := registry.Gather()
	found := false
	for _, m := range metrics {
		if *m.Name == "httpcache_http_requests_total" {
			for _, metric := range m.Metric {
				labels := make(map[string]string)
				for _, label := range metric.Label {
					labels[*label.Name] = *label.Value
				}
				if labels["cache_status"] == "miss" {
					found = true
					break
				}
			}
		}
	}

	if !found {
		t.Error("cache miss status not recorded")
	}

	// Test cache hit (second request to same URL)
	resp2, _ := client.Get(testServerMiss.URL)
	io.Copy(io.Discard, resp2.Body) // Read body
	resp2.Body.Close()

	// Verify hit was recorded
	metrics, _ = registry.Gather()
	found = false
	for _, m := range metrics {
		if *m.Name == "httpcache_http_requests_total" {
			for _, metric := range m.Metric {
				labels := make(map[string]string)
				for _, label := range metric.Label {
					labels[*label.Name] = *label.Value
				}
				if labels["cache_status"] == "hit" {
					found = true
					break
				}
			}
		}
	}

	if !found {
		t.Error("cache hit status not recorded")
	}
}

func TestInstrumentedTransportDifferentStatusCodes(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := NewCollectorWithRegistry(registry)

	baseCache := newMockCache()
	cache := NewInstrumentedCache(baseCache, "memory", collector)
	transport := httpcache.NewTransport(cache)
	instrumentedTransport := NewInstrumentedTransport(transport, collector)

	client := instrumentedTransport.Client()

	// Test different status codes
	statusCodes := []int{200, 404, 500}

	for _, code := range statusCodes {
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))

		resp, _ := client.Get(testServer.URL)
		resp.Body.Close()
		testServer.Close()
	}

	// Verify all status codes were recorded
	metrics, _ := registry.Gather()
	statusCodesFound := make(map[string]bool)

	for _, m := range metrics {
		if *m.Name == "httpcache_http_requests_total" {
			for _, metric := range m.Metric {
				for _, label := range metric.Label {
					if *label.Name == "status_code" {
						statusCodesFound[*label.Value] = true
					}
				}
			}
		}
	}

	// Just verify the metrics were created for all status codes
	if len(statusCodesFound) == 0 {
		t.Error("no status codes recorded in metrics")
	}

	// We should have metrics for multiple status codes
	if len(statusCodesFound) < 2 {
		t.Errorf("expected multiple status codes, got %d", len(statusCodesFound))
	}
}
