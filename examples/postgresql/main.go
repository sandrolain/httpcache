package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/postgresql"
)

func main() {
	ctx := context.Background()

	// PostgreSQL connection string
	// Format: postgres://username:password@host:port/database?sslmode=disable
	connString := "postgres://postgres:postgres@localhost:5432/httpcache?sslmode=disable"

	// Create cache with custom configuration
	config := &postgresql.Config{
		TableName: "my_http_cache",
		KeyPrefix: "api:",
		Timeout:   10 * time.Second,
	}

	cache, err := postgresql.New(ctx, connString, config)
	if err != nil {
		log.Fatalf("Failed to create PostgreSQL cache: %v", err)
	}
	defer cache.Close()

	// Create HTTP transport with caching
	transport := httpcache.NewTransport(cache)

	// Create HTTP client
	client := transport.Client()

	// Make a request (will be cached)
	fmt.Println("Making first request...")
	resp, err := client.Get("https://api.github.com/users/github")
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Check if response was cached
	if resp.Header.Get(httpcache.XFromCache) == "" {
		fmt.Println("Response from server (not cached)")
	} else {
		fmt.Println("Response from cache")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}
	fmt.Printf("Response length: %d bytes\n", len(body))

	// Make the same request again (should be from cache)
	fmt.Println("\nMaking second request...")
	resp2, err := client.Get("https://api.github.com/users/github")
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.Header.Get(httpcache.XFromCache) == "" {
		fmt.Println("Response from server (not cached)")
	} else {
		fmt.Println("Response from cache")
	}

	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}
	fmt.Printf("Response length: %d bytes\n", len(body2))

	fmt.Println("\nCache example completed successfully!")
}
