package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	httpcache "github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/diskcache"
	rediscache "github.com/sandrolain/httpcache/redis"
	"github.com/sandrolain/httpcache/wrapper/multicache"

	"github.com/gomodule/redigo/redis"
)

const (
	jsonURL = "https://httpbin.org/json"
)

func main() {
	fmt.Println("MultiCache Example - Three-Tier Caching Strategy")
	fmt.Println("=================================================")
	fmt.Println("")

	// Tier 1: In-memory cache (fast, small, volatile)
	// 10 MB limit, evicts least recently used
	memoryCache := httpcache.NewMemoryCache()
	fmt.Println("✓ Tier 1: Memory cache initialized (fast, volatile)")

	// Tier 2: Disk cache (medium speed, larger, persistent)
	// 100 MB limit, survives restarts
	diskCache := diskcache.New("/tmp/httpcache-multicache-example")
	fmt.Println("✓ Tier 2: Disk cache initialized (medium speed, persistent)")

	// Tier 3: Redis cache (network-based, largest, shared)
	// Optional - only if Redis is available
	var mc *multicache.MultiCache
	redisConn, err := redis.Dial("tcp", "localhost:6379")
	if err != nil {
		fmt.Println("⚠ Tier 3: Redis not available, using only 2 tiers")
		mc = multicache.New(memoryCache, diskCache)
	} else {
		fmt.Println("✓ Tier 3: Redis cache initialized (network-based, shared)")
		redisCache := rediscache.NewWithClient(redisConn)
		mc = multicache.New(memoryCache, diskCache, redisCache)
	}

	// Create HTTP client with multi-tier caching
	transport := httpcache.NewTransport(mc)
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	fmt.Println("\nSetup complete!")
	fmt.Println("")

	// Example 1: Initial request (cache miss, writes to all tiers)
	fmt.Println("Example 1: Initial request")
	fmt.Println("---------------------------")
	makeRequest(client, jsonURL)
	fmt.Println()

	// Example 2: Subsequent request (cache hit from tier 1 - fastest)
	fmt.Println("Example 2: Second request (should hit tier 1 - memory)")
	fmt.Println("-------------------------------------------------------")
	makeRequest(client, jsonURL)
	fmt.Println()

	// Example 3: Simulate tier 1 eviction/restart
	fmt.Println("Example 3: Simulating tier 1 cache clear")
	fmt.Println("-----------------------------------------")
	fmt.Println("Clearing memory cache...")
	memoryCache.Delete(cacheKey(jsonURL))
	fmt.Println("Making request (should hit tier 2 and promote to tier 1)...")
	makeRequest(client, jsonURL)
	fmt.Println()

	// Example 4: Make another request to verify promotion
	fmt.Println("Example 4: Verify promotion to tier 1")
	fmt.Println("--------------------------------------")
	fmt.Println("Making request (should hit tier 1 again after promotion)...")
	makeRequest(client, jsonURL)
	fmt.Println()

	// Example 5: Different URL to demonstrate independent caching
	fmt.Println("Example 5: Different URL")
	fmt.Println("-------------------------")
	makeRequest(client, "https://httpbin.org/headers")
	fmt.Println()

	fmt.Println("Examples completed!")
	fmt.Println("\nKey Takeaways:")
	fmt.Println("• First request: Cache miss → stores in all tiers")
	fmt.Println("• Second request: Cache hit from tier 1 (fastest)")
	fmt.Println("• After tier 1 clear: Hit from tier 2, auto-promoted to tier 1")
	fmt.Println("• Subsequent requests: Fast hits from tier 1 again")
	fmt.Println("• Each URL is cached independently across all tiers")
}

func makeRequest(client *http.Client, url string) {
	start := time.Now()

	resp, err := client.Get(url)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}
	defer resp.Body.Close()

	// Read and discard body to trigger caching
	_, _ = io.Copy(io.Discard, resp.Body)

	elapsed := time.Since(start)

	// Check cache status from header
	cacheStatus := "MISS"
	if resp.Header.Get("X-From-Cache") == "1" {
		cacheStatus = "HIT"
	}

	fmt.Printf("URL: %s\n", url)
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Cache: %s\n", cacheStatus)
	fmt.Printf("Time: %v\n", elapsed)
}

// cacheKey generates the cache key for a URL
// This is a simplified version - the actual Transport uses a more sophisticated key
func cacheKey(url string) string {
	return url
}
