package main

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/hazelcast/hazelcast-go-client"
	"github.com/sandrolain/httpcache"
	hzcache "github.com/sandrolain/httpcache/hazelcast"
)

func main() {
	ctx := context.Background()

	// Create Hazelcast client configuration
	config := hazelcast.Config{}
	config.Cluster.Network.SetAddresses("localhost:5701")
	config.Cluster.Unisocket = true

	// Create Hazelcast client
	client, err := hazelcast.StartNewClientWithConfig(ctx, config)
	if err != nil {
		log.Fatalf("Failed to create Hazelcast client: %v", err)
	}
	defer client.Shutdown(ctx)

	// Get Hazelcast map for caching
	hzMap, err := client.GetMap(ctx, "http-cache")
	if err != nil {
		log.Fatalf("Failed to get Hazelcast map: %v", err)
	}

	// Create cache with Hazelcast backend
	cache := hzcache.NewWithMap(hzMap)

	// Create transport with cache
	transport := httpcache.NewTransport(cache)

	// Create HTTP client with caching transport
	httpClient := transport.Client()

	// Make first request (will be cached)
	fmt.Println("Making first request...")
	resp1, err := httpClient.Get("https://httpbin.org/cache/60")
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
	resp2, err := httpClient.Get("https://httpbin.org/cache/60")
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	fmt.Printf("Response 2 status: %s\n", resp2.Status)
	fmt.Printf("Response 2 from cache: %s\n", resp2.Header.Get(httpcache.XFromCache))
	fmt.Printf("Response 2 body length: %d bytes\n\n", len(body2))

	if resp2.Header.Get(httpcache.XFromCache) == "1" {
		fmt.Println("✓ Second request was successfully served from Hazelcast cache!")
	} else {
		fmt.Println("✗ Second request was not served from cache")
	}

	// Show Hazelcast map stats
	size, err := hzMap.Size(ctx)
	if err != nil {
		log.Printf("Failed to get map size: %v", err)
	} else {
		fmt.Printf("\nHazelcast Map Status:\n")
		fmt.Printf("  Map Name: http-cache\n")
		fmt.Printf("  Entries: %d\n", size)
	}
}
