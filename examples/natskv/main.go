package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/natskv"
)

func main() {
	// Example 1: Using New() constructor (recommended for most use cases)
	// This manages the NATS connection internally
	fmt.Println("=== Example 1: Using New() constructor ===")
	exampleWithNew()

	fmt.Println("\n" + strings.Repeat("=", 60) + "\n")

	// Example 2: Using NewWithKeyValue() for manual connection management
	// This is useful when you need more control over the NATS connection
	fmt.Println("=== Example 2: Using NewWithKeyValue() for manual management ===")
	exampleWithKeyValue()
}

func exampleWithNew() {
	ctx := context.Background()

	// Create cache with New() - manages NATS connection internally
	cache, err := natskv.New(ctx, natskv.Config{
		NATSUrl:     "nats://localhost:4222", // Or use "" for default
		Bucket:      "http-cache",
		Description: "HTTP cache storage",
		TTL:         time.Hour * 24, // Cache entries expire after 24 hours
	})
	if err != nil {
		log.Fatalf("Failed to create cache: %v", err)
	}
	defer func() {
		if c, ok := cache.(interface{ Close() error }); ok {
			c.Close()
		}
	}()

	// Create transport with cache
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	// Make requests
	makeRequests(client)
}

func exampleWithKeyValue() {
	// This example shows the traditional approach when you need more control
	// over the NATS connection and JetStream configuration
	// (Implementation similar to previous version)
	fmt.Println("For manual NATS connection management, see the test files.")
	fmt.Println("The New() constructor is recommended for most use cases.")
}

func makeRequests(client *http.Client) {
	// Make first request (will be cached)
	fmt.Println("Making first request...")
	resp1, err := client.Get("https://httpbin.org/cache/60")
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp1.Body.Close()

	body1, _ := io.ReadAll(resp1.Body)
	fmt.Printf("Response 1 status: %s\n", resp1.Status)
	fmt.Printf("Response 1 from cache: %s\n", resp1.Header.Get(httpcache.XFromCache))
	fmt.Printf("Response 1 body length: %d bytes\n\n", len(body1))

	// Make second request (should come from cache)
	fmt.Println("Making second request (should be cached)...")
	resp2, err := client.Get("https://httpbin.org/cache/60")
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	fmt.Printf("Response 2 status: %s\n", resp2.Status)
	fmt.Printf("Response 2 from cache: %s\n", resp2.Header.Get(httpcache.XFromCache))
	fmt.Printf("Response 2 body length: %d bytes\n\n", len(body2))

	if resp2.Header.Get(httpcache.XFromCache) == "1" {
		fmt.Println("✓ Second request was successfully served from NATS K/V cache!")
	} else {
		fmt.Println("✗ Second request was not served from cache")
	}
}
