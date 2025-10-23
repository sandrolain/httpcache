package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/leveldbcache"
)

func main() {
	// Create a temporary directory for LevelDB
	dbDir := filepath.Join(os.TempDir(), "httpcache-leveldb-example")
	defer os.RemoveAll(dbDir)

	fmt.Printf("Using LevelDB directory: %s\n\n", dbDir)

	// Create LevelDB cache
	cache, err := leveldbcache.New(dbDir)
	if err != nil {
		log.Fatal("Failed to create LevelDB cache:", err)
	}

	fmt.Println("Example: LevelDB persistent cache")
	fmt.Println("==================================\n")

	// Create HTTP transport with LevelDB cache
	transport := httpcache.NewTransport(cache)
	client := &http.Client{Transport: transport}

	url := "https://httpbin.org/cache/300"

	// First request
	fmt.Println("Making first request...")
	resp1, err := client.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp1.Body.Close()

	body1, _ := io.ReadAll(resp1.Body)
	fmt.Printf("Status: %s\n", resp1.Status)
	fmt.Printf("From cache: %s\n", resp1.Header.Get(httpcache.XFromCache))
	fmt.Printf("Response length: %d bytes\n\n", len(body1))

	// Second request - from LevelDB cache
	fmt.Println("Making second request (should be cached in LevelDB)...")
	resp2, err := client.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	fmt.Printf("Status: %s\n", resp2.Status)
	fmt.Printf("From cache: %s\n", resp2.Header.Get(httpcache.XFromCache))
	fmt.Printf("Response length: %d bytes\n\n", len(body2))

	if resp2.Header.Get(httpcache.XFromCache) == "1" {
		fmt.Println("✓ LevelDB cache is working!")
	}

	// Example: Multiple URLs cached
	fmt.Println("\nExample: Caching multiple URLs")
	fmt.Println("===============================\n")

	urls := []string{
		"https://httpbin.org/cache/60",
		"https://httpbin.org/cache/120",
		"https://httpbin.org/cache/180",
	}

	for i, u := range urls {
		fmt.Printf("%d. Fetching %s\n", i+1, u)
		resp, err := client.Get(u)
		if err != nil {
			log.Printf("Error: %v\n", err)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		fromCache := resp.Header.Get(httpcache.XFromCache)
		if fromCache == "1" {
			fmt.Println("   ↳ Served from cache")
		} else {
			fmt.Println("   ↳ Fetched from server")
		}
	}

	// Second pass - all should be cached
	fmt.Println("\nMaking second pass (all should be cached)...")
	cachedCount := 0
	for i, u := range urls {
		resp, err := client.Get(u)
		if err != nil {
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.Header.Get(httpcache.XFromCache) == "1" {
			cachedCount++
			fmt.Printf("%d. %s ✓ cached\n", i+1, u)
		}
	}

	fmt.Printf("\n%d out of %d URLs served from LevelDB cache\n", cachedCount, len(urls))

	// Stats about the cache directory
	var totalSize int64
	filepath.Walk(dbDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	fmt.Printf("\nLevelDB cache size: %.2f KB\n", float64(totalSize)/1024)
	fmt.Println("\nExample completed successfully!")
}
