package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/mongodb"
)

func main() {
	// Get MongoDB URI from environment variable or use default
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}

	// Create MongoDB cache configuration
	config := mongodb.Config{
		URI:        uri,
		Database:   "httpcache",      // MongoDB database name
		Collection: "http_responses", // MongoDB collection name
		KeyPrefix:  "http:",          // Optional prefix for all cache keys
		Timeout:    10 * time.Second, // Timeout for MongoDB operations
		TTL:        24 * time.Hour,   // Optional: TTL for cache entries (MongoDB will auto-expire)
	}

	// Create MongoDB cache instance
	ctx := context.Background()
	cache, err := mongodb.New(ctx, config)
	if err != nil {
		log.Fatalf("Failed to create MongoDB cache: %v", err)
	}

	// Ensure proper cleanup
	if closer, ok := cache.(interface{ Close() error }); ok {
		defer func() {
			if err := closer.Close(); err != nil {
				log.Printf("Failed to close MongoDB cache: %v", err)
			}
		}()
	}

	// Create HTTP transport with MongoDB cache
	transport := httpcache.NewTransport(cache)

	// Create HTTP client
	client := transport.Client()

	// Make some HTTP requests
	urls := []string{
		"https://jsonplaceholder.typicode.com/posts/1",
		"https://jsonplaceholder.typicode.com/posts/2",
		"https://jsonplaceholder.typicode.com/users/1",
	}

	for _, url := range urls {
		fmt.Printf("\nFetching: %s\n", url)

		resp, err := client.Get(url)
		if err != nil {
			log.Printf("Error fetching %s: %v", url, err)
			continue
		}

		// Read response body
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Error reading response body: %v", err)
			continue
		}

		// Check if response was served from cache
		cacheStatus := "MISS"
		if resp.Header.Get(httpcache.XFromCache) != "" {
			cacheStatus = "HIT"
		}

		fmt.Printf("Status: %s\n", resp.Status)
		fmt.Printf("Cache: %s\n", cacheStatus)
		fmt.Printf("Body length: %d bytes\n", len(body))
	}

	// Second round - should hit cache
	fmt.Println("\n=== Second round (should hit cache) ===")

	for _, url := range urls {
		fmt.Printf("\nFetching: %s\n", url)

		resp, err := client.Get(url)
		if err != nil {
			log.Printf("Error fetching %s: %v", url, err)
			continue
		}

		// Read response body
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Error reading response body: %v", err)
			continue
		}

		// Check if response was served from cache
		cacheStatus := "MISS"
		if resp.Header.Get(httpcache.XFromCache) != "" {
			cacheStatus = "HIT"
		}

		fmt.Printf("Status: %s\n", resp.Status)
		fmt.Printf("Cache: %s\n", cacheStatus)
		fmt.Printf("Body length: %d bytes\n", len(body))
	}

	fmt.Println("\n=== MongoDB Cache Example Complete ===")
}
