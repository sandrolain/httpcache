package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/natskv"
)

func main() {
	// Connect to NATS server
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	// Create JetStream context
	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("Failed to create JetStream context: %v", err)
	}

	// Create or get K/V bucket
	ctx := context.Background()
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "http-cache",
		Description: "HTTP cache storage",
		TTL:         time.Hour * 24, // Cache entries expire after 24 hours
	})
	if err != nil {
		log.Fatalf("Failed to create K/V bucket: %v", err)
	}

	// Create cache with NATS K/V backend
	cache := natskv.NewWithKeyValue(kv)

	// Create transport with cache
	transport := httpcache.NewTransport(cache)

	// Create HTTP client with caching transport
	client := transport.Client()

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

	// Show K/V bucket status
	status, err := kv.Status(ctx)
	if err != nil {
		log.Printf("Failed to get K/V status: %v", err)
	} else {
		fmt.Printf("\nNATS K/V Bucket Status:\n")
		fmt.Printf("  Bucket: %s\n", status.Bucket())
		fmt.Printf("  Values: %d\n", status.Values())
		fmt.Printf("  Bytes: %d\n", status.Bytes())
	}
}
