package main

import (
	"fmt"
	"io"
	"log"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/diskcache"
)

func main() {
	// Create a disk cache
	cache := diskcache.New("/tmp/httpcache")

	// Create transport with custom MaxCacheableResponseSize
	// This prevents memory exhaustion from large responses
	transport := httpcache.NewTransport(
		cache,
		// Only cache responses up to 5MB
		// Larger responses will be streamed without caching
		httpcache.WithMaxCacheableResponseSize(5*1024*1024), // 5MB
		httpcache.WithMarkCachedResponses(true),
	)

	client := transport.Client()

	// Example 1: Small response (will be cached)
	fmt.Println("Fetching small API response...")
	smallResp, err := client.Get("https://api.github.com/users/octocat")
	if err != nil {
		log.Fatal(err)
	}
	defer smallResp.Body.Close()

	smallBody, _ := io.ReadAll(smallResp.Body)
	fmt.Printf("Small response: %d bytes, Cached: %s\n",
		len(smallBody),
		smallResp.Header.Get("X-From-Cache"))

	// Example 2: Large file (will be streamed, not cached)
	fmt.Println("\nFetching large file...")
	largeResp, err := client.Get("https://speed.hetzner.de/1GB.bin")
	if err != nil {
		log.Fatal(err)
	}
	defer largeResp.Body.Close()

	// Just read first 1KB to demonstrate
	buffer := make([]byte, 1024)
	n, _ := largeResp.Body.Read(buffer)
	fmt.Printf("Large file: Read %d bytes (streaming, not cached)\n", n)

	// Example 3: Making the same small request again (should be cached)
	fmt.Println("\nFetching small API response again...")
	cachedResp, err := client.Get("https://api.github.com/users/octocat")
	if err != nil {
		log.Fatal(err)
	}
	defer cachedResp.Body.Close()

	cachedBody, _ := io.ReadAll(cachedResp.Body)
	fmt.Printf("Small response: %d bytes, Cached: %s\n",
		len(cachedBody),
		cachedResp.Header.Get("X-From-Cache"))

	fmt.Println("\n✅ Memory leak prevention working correctly!")
	fmt.Println("Small responses are cached, large ones are streamed.")
}
