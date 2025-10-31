package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sandrolain/httpcache"
	prommetrics "github.com/sandrolain/httpcache/wrapper/metrics/prometheus"
)

func main() {
	fmt.Println("httpcache Prometheus Metrics Example")
	fmt.Println("====================================")

	// Create Prometheus collector
	collector := prommetrics.NewCollector()

	// Create base cache
	baseCache := httpcache.NewMemoryCache()

	// Wrap cache with instrumentation
	instrumentedCache := prommetrics.NewInstrumentedCache(
		baseCache,
		"memory", // backend name for metrics labels
		collector,
	)

	// Create transport with instrumented cache
	transport := httpcache.NewTransport(instrumentedCache)

	// Wrap transport with instrumentation
	instrumentedTransport := prommetrics.NewInstrumentedTransport(transport, collector)

	// Create HTTP client
	client := instrumentedTransport.Client()

	// Start metrics server in background
	fmt.Println("Starting metrics server on http://localhost:9090/metrics")
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())

		server := &http.Server{
			Addr:         ":9090",
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		}

		if err := server.ListenAndServe(); err != nil {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Example URLs to fetch
	urls := []string{
		"https://httpbin.org/cache/300",  // Cacheable for 300 seconds
		"https://httpbin.org/delay/1",    // 1 second delay
		"https://httpbin.org/get",        // Basic GET request
		"https://httpbin.org/cache/300",  // Same as first (cache hit)
		"https://httpbin.org/status/404", // 404 status
	}

	fmt.Println("\nMaking HTTP requests...")
	fmt.Println("========================")

	for i, url := range urls {
		fmt.Printf("\n%d. Requesting: %s\n", i+1, url)

		start := time.Now()
		resp, err := client.Get(url)
		duration := time.Since(start)

		if err != nil {
			log.Printf("   Error: %v\n", err)
			continue
		}

		// Read body to trigger caching
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		cacheStatus := "miss"
		if resp.Header.Get(httpcache.XFromCache) == "1" {
			cacheStatus = "HIT ✓"
		} else if resp.StatusCode == http.StatusNotModified {
			cacheStatus = "revalidated"
		}

		fmt.Printf("   Status: %d\n", resp.StatusCode)
		fmt.Printf("   Cache: %s\n", cacheStatus)
		fmt.Printf("   Duration: %v\n", duration)
		fmt.Printf("   Body size: %d bytes\n", len(body))
	}

	// Additional requests to generate more metrics
	fmt.Println("\n\nGenerating additional traffic for metrics...")
	for i := 0; i < 10; i++ {
		resp, err := client.Get("https://httpbin.org/cache/300")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Show some example metrics queries
	fmt.Println("\n\nMetrics are now available at: http://localhost:9090/metrics")
	fmt.Println("\nExample PromQL queries:")
	fmt.Println("=======================")
	fmt.Println("1. Cache hit rate:")
	fmt.Println("   rate(httpcache_cache_requests_total{result=\"hit\"}[5m]) /")
	fmt.Println("   rate(httpcache_cache_requests_total{operation=\"get\"}[5m]) * 100")
	fmt.Println()
	fmt.Println("2. P95 cache latency:")
	fmt.Println("   histogram_quantile(0.95,")
	fmt.Println("     rate(httpcache_cache_operation_duration_seconds_bucket[5m]))")
	fmt.Println()
	fmt.Println("3. HTTP requests by cache status:")
	fmt.Println("   sum by (cache_status) (httpcache_http_requests_total)")
	fmt.Println()
	fmt.Println("4. Total bandwidth saved:")
	fmt.Println("   httpcache_http_response_size_bytes_total{cache_status=\"hit\"}")

	fmt.Println("\n\nPress Ctrl+C to exit (server will keep running)")

	// Keep running to allow viewing metrics
	select {}
}
