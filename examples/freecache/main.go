package main

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/freecache"
)

func main() {
	// Create a 100MB cache
	// For large caches, reduce GC percentage for better performance
	cache := freecache.New(100 * 1024 * 1024)
	debug.SetGCPercent(20)

	// Create HTTP transport with the cache
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	// Make multiple requests to demonstrate caching
	urls := []string{
		"https://httpbin.org/cache/300", // Cacheable for 5 minutes
		"https://httpbin.org/cache/300",
		"https://httpbin.org/cache/300",
	}

	for i, url := range urls {
		fmt.Printf("\n--- Request %d to %s ---\n", i+1, url)

		resp, err := client.Get(url)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		// Read response
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("Error reading body: %v\n", err)
			continue
		}

		// Show cache headers
		fromCache := resp.Header.Get("X-From-Cache")
		fmt.Printf("Response length: %d bytes\n", len(body))
		fmt.Printf("From cache: %s\n", fromCache)
		fmt.Printf("Cache-Control: %s\n", resp.Header.Get("Cache-Control"))

		// Show cache statistics
		fmt.Printf("\nCache Statistics:\n")
		fmt.Printf("  Hit rate: %.2f%%\n", cache.HitRate()*100)
		fmt.Printf("  Entries: %d\n", cache.EntryCount())
		fmt.Printf("  Evictions: %d\n", cache.EvacuateCount())
		fmt.Printf("  Expired: %d\n", cache.ExpiredCount())
	}

	// Demonstrate cache clearing
	fmt.Printf("\n--- Clearing cache ---\n")
	cache.Clear()
	fmt.Printf("Entries after clear: %d\n", cache.EntryCount())

	// Final request after clear
	fmt.Printf("\n--- Request after clear ---\n")
	resp, err := client.Get("https://httpbin.org/cache/300")
	if err == nil {
		resp.Body.Close()
		fmt.Printf("From cache: %s\n", resp.Header.Get("X-From-Cache"))
		fmt.Printf("Entries: %d\n", cache.EntryCount())
	}
}
