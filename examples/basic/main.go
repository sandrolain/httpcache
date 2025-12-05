package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/diskcache"
)

func main() {
	// Create a temporary directory for the disk cache
	tmpDir, err := os.MkdirTemp("", "httpcache-example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir) // Clean up when done

	// Create a new HTTP client with disk cache
	cache := diskcache.New(tmpDir)
	transport := httpcache.NewTransport(cache)
	transport.MarkCachedResponses = true
	client := transport.Client()

	url := "https://httpbin.org/cache/300" // Cacheable for 300 seconds

	fmt.Println("Example 1: Basic disk caching")
	fmt.Println("==============================")

	// First request - will fetch from server
	fmt.Println("Making first request...")
	resp1, err := client.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp1.Body.Close()

	body1, _ := io.ReadAll(resp1.Body)
	fmt.Printf("Status: %s\n", resp1.Status)
	fmt.Printf("From cache: %s\n", resp1.Header.Get(httpcache.XFromCache))
	fmt.Printf("Response length: %d bytes\n\n", len(body1))

	// Second request - should come from cache
	fmt.Println("Making second request (should be cached)...")
	time.Sleep(100 * time.Millisecond) // Small delay to show it's not simultaneous

	resp2, err := client.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	fmt.Printf("Status: %s\n", resp2.Status)
	fmt.Printf("From cache: %s\n", resp2.Header.Get(httpcache.XFromCache))
	fmt.Printf("Response length: %d bytes\n\n", len(body2))

	// Verify cache hit
	if resp2.Header.Get(httpcache.XFromCache) == "1" {
		fmt.Println("✓ Cache is working! Second request was served from cache.")
	} else {
		fmt.Println("✗ Cache miss - this shouldn't happen for cacheable responses")
	}

	// Example with ETag validation
	fmt.Println("\nExample 2: Cache with ETag validation")
	fmt.Println("======================================")

	etagURL := "https://httpbin.org/etag/test-etag"

	// First request
	fmt.Println("Making first request with ETag...")
	resp3, err := client.Get(etagURL)
	if err != nil {
		log.Fatal(err)
	}
	defer resp3.Body.Close()
	io.Copy(io.Discard, resp3.Body)

	fmt.Printf("ETag: %s\n", resp3.Header.Get("ETag"))
	fmt.Printf("From cache: %s\n\n", resp3.Header.Get(httpcache.XFromCache))

	// Second request - should validate with If-None-Match
	fmt.Println("Making second request (should revalidate)...")
	resp4, err := client.Get(etagURL)
	if err != nil {
		log.Fatal(err)
	}
	defer resp4.Body.Close()
	io.Copy(io.Discard, resp4.Body)

	fmt.Printf("Status: %s\n", resp4.Status)
	fmt.Printf("From cache: %s\n", resp4.Header.Get(httpcache.XFromCache))

	fmt.Println("\nExample completed successfully!")
}
