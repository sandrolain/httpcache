package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/diskcache"
)

func main() {
	// Create a temporary directory for the cache
	cacheDir := filepath.Join(os.TempDir(), "httpcache-example")
	defer os.RemoveAll(cacheDir)

	fmt.Printf("Using cache directory: %s\n\n", cacheDir)

	// Create a disk-based cache
	cache := diskcache.New(cacheDir)

	// Create an HTTP transport with the disk cache
	transport := httpcache.NewTransport(cache)
	transport.MarkCachedResponses = true

	client := &http.Client{Transport: transport}

	fmt.Println("Example: Persistent disk cache")
	fmt.Println("===============================\n")

	url := "https://httpbin.org/cache/3600" // Cacheable for 1 hour

	// First request
	fmt.Println("Making first request...")
	resp1, err := client.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp1.Body.Close()

	body1, _ := io.ReadAll(resp1.Body)
	fmt.Printf("Status: %s\n", resp1.Status)
	fmt.Printf("From cache: %s\n", resp1.Header.Get(httpcache.XFromCache))
	fmt.Printf("Response length: %d bytes\n", len(body1))
	fmt.Printf("Cache-Control: %s\n\n", resp1.Header.Get("Cache-Control"))

	// Check cache directory
	entries, _ := os.ReadDir(cacheDir)
	fmt.Printf("Cache directory has %d file(s)\n\n", len(entries))

	// Second request - from disk cache
	fmt.Println("Making second request (should be cached on disk)...")
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
		fmt.Println("✓ Disk cache is working!")
		fmt.Println("\nNote: The cached response persists across application restarts.")
		fmt.Println("In a real application, you would use a permanent directory.")
	}

	// Example: Cache survives client recreation
	fmt.Println("\nExample: Creating a new client with same cache directory")
	fmt.Println("=========================================================\n")

	// Create a new transport and client
	cache2 := diskcache.New(cacheDir)
	transport2 := httpcache.NewTransport(cache2)
	client2 := &http.Client{Transport: transport2}

	fmt.Println("Making request with new client...")
	resp3, err := client2.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp3.Body.Close()
	io.Copy(io.Discard, resp3.Body)

	fmt.Printf("From cache: %s\n", resp3.Header.Get(httpcache.XFromCache))

	if resp3.Header.Get(httpcache.XFromCache) == "1" {
		fmt.Println("\n✓ Cache persisted! New client used the existing cache.")
	}

	fmt.Println("\nExample completed successfully!")
}
