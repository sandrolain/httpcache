package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/blobcache"

	// Import blob drivers as needed
	_ "gocloud.dev/blob/fileblob" // For file:// URLs
	_ "gocloud.dev/blob/memblob"  // For mem:// URLs
	_ "gocloud.dev/blob/s3blob"   // For s3:// URLs (AWS S3, MinIO)
	// _ "gocloud.dev/blob/gcsblob"   // For gs:// URLs (Google Cloud Storage)
	// _ "gocloud.dev/blob/azureblob" // For azblob:// URLs (Azure Blob Storage)
)

func main() {
	ctx := context.Background()

	// Get bucket URL from environment or use in-memory default
	bucketURL := os.Getenv("BUCKET_URL")
	if bucketURL == "" {
		bucketURL = "mem://" // In-memory blob storage for testing
		fmt.Println("Using in-memory blob storage (set BUCKET_URL to use real cloud storage)")
	}

	fmt.Printf("Creating BlobCache with URL: %s\n", bucketURL)

	// Create cache
	cache, err := blobcache.New(ctx, blobcache.Config{
		BucketURL: bucketURL,
		KeyPrefix: "httpcache/",
		Timeout:   30 * time.Second,
	})
	if err != nil {
		log.Fatalf("Failed to create blob cache: %v", err)
	}

	// Ensure cache is properly closed
	if closer, ok := cache.(interface{ Close() error }); ok {
		defer func() {
			fmt.Println("\nClosing blob cache...")
			if err := closer.Close(); err != nil {
				log.Printf("Failed to close cache: %v", err)
			}
		}()
	}

	// Create HTTP client with caching transport
	transport := httpcache.NewTransport(cache)
	client := &http.Client{Transport: transport}

	fmt.Println("\n=== Making HTTP Requests ===")
	fmt.Println()

	// Make first request
	fmt.Println("Request 1: Fetching from server...")
	makeRequest(client, "https://httpbin.org/delay/1")

	// Make second request (should be cached)
	fmt.Println("\nRequest 2: Should come from cache...")
	makeRequest(client, "https://httpbin.org/delay/1")

	// Make request to different URL
	fmt.Println("\nRequest 3: Different URL (cache miss)...")
	makeRequest(client, "https://httpbin.org/get")

	// Make request to same URL again (cached)
	fmt.Println("\nRequest 4: Same URL as request 3 (cache hit)...")
	makeRequest(client, "https://httpbin.org/get")

	fmt.Println("\n=== Example Complete ===")
	fmt.Println("\nCache entries are stored with SHA-256 hashed keys in the blob storage.")
	fmt.Println("This ensures compatibility with cloud storage naming restrictions.")
}

func makeRequest(client *http.Client, url string) {
	start := time.Now()

	resp, err := client.Get(url)
	if err != nil {
		log.Printf("Request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	// Read and discard body
	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		log.Printf("Failed to read body: %v", err)
		return
	}

	duration := time.Since(start)

	// Check if response came from cache
	fromCache := resp.Header.Get(httpcache.XFromCache) == "1"
	age := resp.Header.Get("Age")

	fmt.Printf("  URL: %s\n", url)
	fmt.Printf("  Status: %s\n", resp.Status)
	fmt.Printf("  Duration: %v\n", duration)
	fmt.Printf("  From Cache: %v\n", fromCache)
	if age != "" {
		fmt.Printf("  Age: %s seconds\n", age)
	}

	if fromCache {
		fmt.Println("  ✓ Response served from blob cache!")
	} else {
		fmt.Println("  ⟳ Response fetched from server and cached")
	}
}
