package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/diskcache"
)

func main() {
	// Create a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		fmt.Fprintf(w, "Response at %s", time.Now().Format(time.RFC3339))
	}))
	defer ts.Close()

	// Create temporary directories for caches
	tmpDir1, _ := os.MkdirTemp("", "httpcache-sha256-*")
	defer os.RemoveAll(tmpDir1)
	tmpDir2, _ := os.MkdirTemp("", "httpcache-xxhash-*")
	defer os.RemoveAll(tmpDir2)

	// Example 1: Default SHA-256 (backward compatible)
	fmt.Println("=== Example 1: SHA-256 (Default) ===")
	cache1 := diskcache.New(tmpDir1)
	transport1 := httpcache.NewTransport(cache1)
	runBenchmark(transport1.Client(), ts.URL)

	// Example 2: xxHash (high performance)
	fmt.Println("\n=== Example 2: xxHash (High Performance) ===")
	cache2 := diskcache.New(tmpDir2)
	transport2 := httpcache.NewTransport(cache2,
		httpcache.WithHashAlgorithm(httpcache.HashAlgorithmXXHash),
	)
	runBenchmark(transport2.Client(), ts.URL)

	// Show recommendations
	fmt.Println("\n=== Recommendations ===")
	fmt.Println("Use SHA-256 (default) for:")
	fmt.Println("  - Existing deployments (backward compatibility)")
	fmt.Println("  - Distributed caches across trust boundaries")
	fmt.Println("  - Security-sensitive applications")
	fmt.Println("\nUse xxHash for:")
	fmt.Println("  - High-throughput scenarios (>10K req/sec)")
	fmt.Println("  - In-memory caches with short TTL")
	fmt.Println("  - Performance-critical microservices")
	fmt.Println("  - Limited cache storage (72% smaller keys)")
	fmt.Println("\n⚠️  Warning: Changing algorithms invalidates existing cache entries")
}

func runBenchmark(client *http.Client, url string) {
	// First request (cache miss)
	start := time.Now()
	resp, err := client.Get(url)
	if err != nil {
		panic(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	miss := time.Since(start)

	// Second request (cache hit)
	start = time.Now()
	resp, err = client.Get(url)
	if err != nil {
		panic(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	hit := time.Since(start)

	// Display results
	cached := resp.Header.Get(httpcache.XFromCache) == "1"
	fmt.Printf("First request (miss):  %v\n", miss)
	fmt.Printf("Second request (hit):  %v\n", hit)
	fmt.Printf("Cached: %v\n", cached)
	if hit > 0 {
		fmt.Printf("Speedup: %.2fx faster\n", float64(miss)/float64(hit))
	}
}
