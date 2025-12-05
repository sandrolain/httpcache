// Package main demonstrates the use of CompressCache wrapper
// which adds compression support to any cache backend.
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/diskcache"
	"github.com/sandrolain/httpcache/wrapper/compresscache"
)

func main() {
	// Create a temporary directory for disk cache
	tmpDir, err := os.MkdirTemp("", "httpcache-compresscache-example-*")
	if err != nil {
		log.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a disk cache as the underlying cache
	cache := diskcache.New(tmpDir)

	// Wrap the cache with compression support using Gzip
	// This will compress cached responses to save storage space
	compressedCache, err := compresscache.NewGzip(compresscache.GzipConfig{
		Cache: cache,
	})
	if err != nil {
		log.Fatalf("Failed to create compressed cache: %v", err)
	}

	// Create a caching transport
	transport := httpcache.NewTransport(compressedCache)
	client := transport.Client()

	// Make a request - response will be cached with compression
	url := "https://httpbin.org/get"

	fmt.Println("Making first request (will be cached with compression)...")
	start := time.Now()
	resp, err := client.Get(url)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()
	fmt.Printf("First request took: %v\n", time.Since(start))
	fmt.Printf("Response status: %s\n", resp.Status)
	fmt.Printf("X-From-Cache: %s\n\n", resp.Header.Get(httpcache.XFromCache))

	// Make the same request again - should be served from compressed cache
	fmt.Println("Making second request (should be served from compressed cache)...")
	start = time.Now()
	resp, err = client.Get(url)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()
	fmt.Printf("Second request took: %v\n", time.Since(start))
	fmt.Printf("Response status: %s\n", resp.Status)
	fmt.Printf("X-From-Cache: %s\n", resp.Header.Get(httpcache.XFromCache))
}
