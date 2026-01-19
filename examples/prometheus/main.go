package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/diskcache"
	prommetrics "github.com/sandrolain/httpcache/wrapper/metrics/prometheus"
)

func main() {
	fmt.Println("httpcache Prometheus Metrics Example")
	fmt.Println("====================================")

	// Create a temporary directory for the disk cache
	tmpDir, err := os.MkdirTemp("", "httpcache-prometheus")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir) // Clean up when done

	// Create base cache
	cache := diskcache.New(tmpDir)

	// Create internal metrics
	metrics := httpcache.NewTransportMetrics()

	// Create transport with metrics enabled
	transport := httpcache.NewTransport(cache, httpcache.WithMetrics(metrics))

	// Create Prometheus collector that exports the internal metrics
	collector := prommetrics.NewCollector(prommetrics.CollectorConfig{
		Metrics: metrics,
	})

	// Start periodic metric updates
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := collector.Start(ctx)
	defer stop()

	// Create HTTP client
	client := &http.Client{Transport: transport}

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
	fmt.Println("   httpcache_cache_hit_rate")
	fmt.Println()
	fmt.Println("2. Total requests:")
	fmt.Println("   httpcache_total_requests")
	fmt.Println()
	fmt.Println("3. Cache hits over time:")
	fmt.Println("   rate(httpcache_cache_hits_total[5m])")
	fmt.Println()
	fmt.Println("4. Stale responses served:")
	fmt.Println("   httpcache_stale_served_total")
	fmt.Println()
	fmt.Println("5. Deduplication effectiveness:")
	fmt.Println("   httpcache_deduplication_total")

	fmt.Println("\n\nPress Ctrl+C to exit (server will keep running)")

	// Keep running to allow viewing metrics
	select {}
}
