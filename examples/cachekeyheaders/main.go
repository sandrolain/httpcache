package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/diskcache"
)

func main() {
	fmt.Println("=== Cache Key Headers Example ===")

	// Create a temporary directory for the disk cache
	tmpDir, err := os.MkdirTemp("", "httpcache-cachekeyheaders")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir) // Clean up when done

	// Create a transport with cache key headers configured
	cache := diskcache.New(tmpDir)
	transport := httpcache.NewTransport(cache)
	transport.CacheKeyHeaders = []string{"Authorization", "Accept-Language"}
	transport.MarkCachedResponses = true

	client := transport.Client()

	// Example API endpoint (httpbin.org echoes back headers)
	url := "https://httpbin.org/headers"

	fmt.Println("Scenario: Different Authorization tokens should have separate cache entries")

	// Request 1: User token1
	fmt.Println("1. First request with Authorization: Bearer token1")
	resp1 := makeRequest(client, url, "Bearer token1", "en")
	printResponse("Request 1", resp1)

	// Request 2: User token2 (different user)
	fmt.Println("\n2. Second request with Authorization: Bearer token2")
	resp2 := makeRequest(client, url, "Bearer token2", "en")
	printResponse("Request 2", resp2)

	// Request 3: User token1 again (same as first)
	fmt.Println("\n3. Third request with Authorization: Bearer token1 (same as first)")
	resp3 := makeRequest(client, url, "Bearer token1", "en")
	printResponse("Request 3", resp3)

	// Request 4: User token1 with different language
	fmt.Println("\n4. Fourth request with Authorization: Bearer token1, Accept-Language: it")
	resp4 := makeRequest(client, url, "Bearer token1", "it")
	printResponse("Request 4", resp4)

	// Request 5: Repeat request 4 (should be cached)
	fmt.Println("\n5. Fifth request (same as fourth, should be cached)")
	resp5 := makeRequest(client, url, "Bearer token1", "it")
	printResponse("Request 5", resp5)

	fmt.Println("\n=== Summary ===")
	fmt.Println("✓ Request 1: Cache MISS (new token1 + en)")
	fmt.Println("✓ Request 2: Cache MISS (new token2 + en)")
	fmt.Println("✓ Request 3: Cache HIT (same as request 1: token1 + en)")
	fmt.Println("✓ Request 4: Cache MISS (new combination: token1 + it)")
	fmt.Println("✓ Request 5: Cache HIT (same as request 4: token1 + it)")
	fmt.Println("\nEach unique combination of Authorization + Accept-Language creates a separate cache entry!")
}

func makeRequest(client *http.Client, url, authToken, language string) *http.Response {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}

	req.Header.Set("Authorization", authToken)
	req.Header.Set("Accept-Language", language)

	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}

	// Important: Read the body to trigger caching mechanism
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	return resp
}

func printResponse(label string, resp *http.Response) {
	fromCache := resp.Header.Get(httpcache.XFromCache)
	freshness := resp.Header.Get(httpcache.XFreshness)

	if fromCache == "1" {
		fmt.Printf("   %s: ✓ CACHE HIT (Freshness: %s)\n", label, freshness)
	} else {
		fmt.Printf("   %s: ✗ CACHE MISS (fetched from server)\n", label)
	}

	// Show when the response will expire
	if cacheControl := resp.Header.Get("Cache-Control"); cacheControl != "" {
		fmt.Printf("   Cache-Control: %s\n", cacheControl)
	}

	// Add a small delay to make the example clearer
	time.Sleep(100 * time.Millisecond)
}
