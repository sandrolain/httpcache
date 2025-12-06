// Package main demonstrates the use of the cache prewarmer.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/freecache"
	"github.com/sandrolain/httpcache/wrapper/prewarmer"
)

func main() {
	// Create a freecache with 100MB capacity
	cache := freecache.New(100 * 1024 * 1024)
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	// Create a prewarmer with custom configuration
	pw, err := prewarmer.New(prewarmer.Config{
		Client:       client,
		UserAgent:    "MyApp-Prewarmer/1.0",
		Timeout:      30 * time.Second,
		ForceRefresh: false, // Use cached versions if available
	})
	if err != nil {
		log.Fatal(err)
	}

	// URLs to prewarm
	urls := []string{
		"https://httpbin.org/get",
		"https://httpbin.org/headers",
		"https://httpbin.org/user-agent",
	}

	fmt.Println("=== Sequential Prewarming ===")

	// Prewarm with progress callback
	callback := func(result *prewarmer.Result, current, total int) {
		status := "✓"
		if result.Error != nil {
			status = "✗"
		}
		fmt.Printf("[%d/%d] %s %s (status=%d, duration=%s, cached=%v)\n",
			current, total, status, result.URL, result.StatusCode, result.Duration, result.FromCache)
	}

	stats, err := pw.PrewarmWithCallback(context.Background(), urls, callback)
	if err != nil {
		log.Printf("Prewarm error: %v", err)
	}
	printStats("Sequential", stats)

	fmt.Println("\n=== Concurrent Prewarming (Force Refresh) ===")

	// Create a new prewarmer with force refresh enabled
	pwRefresh, err := prewarmer.New(prewarmer.Config{
		Client:       client,
		UserAgent:    "MyApp-Prewarmer/1.0",
		Timeout:      30 * time.Second,
		ForceRefresh: true, // Always fetch fresh content
	})
	if err != nil {
		log.Fatal(err)
	}

	stats, err = pwRefresh.PrewarmConcurrentWithCallback(context.Background(), urls, 3, callback)
	if err != nil {
		log.Printf("Prewarm error: %v", err)
	}
	printStats("Concurrent (refresh)", stats)

	fmt.Println("\n=== Verifying Cache Population ===")

	// Make requests to verify cache is populated
	for _, url := range urls {
		resp, err := client.Get(url)
		if err != nil {
			log.Printf("Error fetching %s: %v", url, err)
			continue
		}
		resp.Body.Close() //nolint:errcheck

		cacheStatus := resp.Header.Get("X-From-Cache")
		fmt.Printf("%s - X-From-Cache: %s\n", url, cacheStatus)
	}

	fmt.Println("\n=== Sitemap Prewarming Example ===")
	fmt.Println("To prewarm from a sitemap, use:")
	fmt.Println(`  stats, err := pw.PrewarmFromSitemap(ctx, "https://example.com/sitemap.xml", 10)`)

	// Example with context cancellation
	fmt.Println("\n=== Context Cancellation Example ===")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This will likely be cancelled before completing
	manyURLs := make([]string, 100)
	for i := range manyURLs {
		manyURLs[i] = fmt.Sprintf("https://httpbin.org/delay/%d", i%3)
	}

	stats, err = pw.PrewarmConcurrent(ctx, manyURLs, 5)
	if err != nil {
		fmt.Printf("Prewarming cancelled: completed %d/%d before timeout\n",
			stats.Successful+stats.Failed, stats.Total)
	}
}

func printStats(label string, stats *prewarmer.Stats) {
	fmt.Printf("\n%s Stats:\n", label)
	fmt.Printf("  Total:      %d\n", stats.Total)
	fmt.Printf("  Successful: %d\n", stats.Successful)
	fmt.Printf("  Failed:     %d\n", stats.Failed)
	fmt.Printf("  Duration:   %s\n", stats.TotalDuration)

	if stats.Failed > 0 {
		fmt.Println("  Errors:")
		for _, err := range stats.Errors {
			fmt.Printf("    - %v\n", err)
		}
	}
}
