package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"strings"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/wrapper/compresscache"
)

func main() {
	fmt.Println("CompressCache Example")
	fmt.Println("=====================")
	fmt.Println()

	// Test data - a larger JSON response that compresses well
	testData := generateTestData(100)

	fmt.Printf("Original data size: %d bytes\n\n", len(testData))

	// Test Gzip compression
	demonstrateGzip(testData)

	// Test Brotli compression
	demonstrateBrotli(testData)

	// Test Snappy compression
	demonstrateSnappy(testData)

	// Demonstrate with HTTP client
	demonstrateHTTPClient()

	// Show cross-algorithm compatibility
	demonstrateCrossCompatibility(testData)
}

func demonstrateGzip(testData []byte) {
	fmt.Println("=== Gzip Compression (Level: BestSpeed) ===")

	baseCache := httpcache.NewMemoryCache()
	cache, err := compresscache.NewGzip(compresscache.GzipConfig{
		Cache: baseCache,
		Level: gzip.BestSpeed,
	})
	if err != nil {
		panic(err)
	}

	// Store and retrieve data
	cache.Set("test-key", testData)
	retrieved, ok := cache.Get("test-key")
	if !ok {
		panic("failed to retrieve data")
	}

	// Verify data integrity
	if string(retrieved) != string(testData) {
		panic("data mismatch after compression/decompression")
	}

	// Show statistics
	stats := cache.Stats()
	fmt.Printf("Original size: %d bytes\n", stats.UncompressedBytes)
	fmt.Printf("Compressed size: %d bytes\n", stats.CompressedBytes)
	fmt.Printf("Compression ratio: %.2f\n", stats.CompressionRatio)
	fmt.Printf("Space savings: %.2f%%\n\n", stats.SavingsPercent)
}

func demonstrateBrotli(testData []byte) {
	fmt.Println("=== Brotli Compression (Level: 6) ===")

	baseCache := httpcache.NewMemoryCache()
	cache, err := compresscache.NewBrotli(compresscache.BrotliConfig{
		Cache: baseCache,
		Level: 6, // Default level
	})
	if err != nil {
		panic(err)
	}

	// Store and retrieve data
	cache.Set("test-key", testData)
	retrieved, ok := cache.Get("test-key")
	if !ok {
		panic("failed to retrieve data")
	}

	// Verify data integrity
	if string(retrieved) != string(testData) {
		panic("data mismatch after compression/decompression")
	}

	// Show statistics
	stats := cache.Stats()
	fmt.Printf("Original size: %d bytes\n", stats.UncompressedBytes)
	fmt.Printf("Compressed size: %d bytes\n", stats.CompressedBytes)
	fmt.Printf("Compression ratio: %.2f\n", stats.CompressionRatio)
	fmt.Printf("Space savings: %.2f%%\n\n", stats.SavingsPercent)
}

func demonstrateSnappy(testData []byte) {
	fmt.Println("=== Snappy Compression ===")

	baseCache := httpcache.NewMemoryCache()
	cache, err := compresscache.NewSnappy(compresscache.SnappyConfig{
		Cache: baseCache,
	})
	if err != nil {
		panic(err)
	}

	// Store and retrieve data
	cache.Set("test-key", testData)
	retrieved, ok := cache.Get("test-key")
	if !ok {
		panic("failed to retrieve data")
	}

	// Verify data integrity
	if string(retrieved) != string(testData) {
		panic("data mismatch after compression/decompression")
	}

	// Show statistics
	stats := cache.Stats()
	fmt.Printf("Original size: %d bytes\n", stats.UncompressedBytes)
	fmt.Printf("Compressed size: %d bytes\n", stats.CompressedBytes)
	fmt.Printf("Compression ratio: %.2f\n", stats.CompressionRatio)
	fmt.Printf("Space savings: %.2f%%\n\n", stats.SavingsPercent)
}

func demonstrateHTTPClient() {
	fmt.Println("=== HTTP Client with Gzip Compression ===")

	// Create compressed cache
	baseCache := httpcache.NewMemoryCache()
	cache, err := compresscache.NewGzip(compresscache.GzipConfig{
		Cache: baseCache,
		Level: gzip.DefaultCompression,
	})
	if err != nil {
		panic(err)
	}

	// Create HTTP client with compressed cache
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	// Make a request
	resp, err := client.Get("https://httpbin.org/json")
	if err != nil {
		fmt.Printf("Request failed: %v\n\n", err)
		return
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Failed to read response: %v\n\n", err)
		return
	}

	fmt.Printf("Response size: %d bytes\n", len(body))
	fmt.Printf("Cached: %s\n", resp.Header.Get(httpcache.XFromCache))

	// Make the same request again - should be cached
	resp2, err := client.Get("https://httpbin.org/json")
	if err != nil {
		fmt.Printf("Second request failed: %v\n\n", err)
		return
	}
	defer resp2.Body.Close()

	io.Copy(io.Discard, resp2.Body)

	fmt.Printf("Second request cached: %s\n", resp2.Header.Get(httpcache.XFromCache))

	// Show compression statistics
	stats := cache.Stats()
	fmt.Printf("Compression savings: %.2f%%\n\n", stats.SavingsPercent)
}

func demonstrateCrossCompatibility(testData []byte) {
	fmt.Println("=== Cross-Algorithm Compatibility ===")

	// Shared backend
	baseCache := httpcache.NewMemoryCache()

	// Create caches with different algorithms
	gzipCache, _ := compresscache.NewGzip(compresscache.GzipConfig{
		Cache: baseCache,
	})
	brotliCache, _ := compresscache.NewBrotli(compresscache.BrotliConfig{
		Cache: baseCache,
	})
	snappyCache, _ := compresscache.NewSnappy(compresscache.SnappyConfig{
		Cache: baseCache,
	})

	// Store with Gzip
	gzipCache.Set("gzip-key", testData)

	// Store with Brotli
	brotliCache.Set("brotli-key", testData)

	// Store with Snappy
	snappyCache.Set("snappy-key", testData)

	// Each cache can read data compressed by others
	fmt.Println("Reading Gzip data with Brotli cache...")
	data, ok := brotliCache.Get("gzip-key")
	if !ok || string(data) != string(testData) {
		panic("cross-algorithm read failed")
	}
	fmt.Println("✓ Success")

	fmt.Println("Reading Brotli data with Snappy cache...")
	data, ok = snappyCache.Get("brotli-key")
	if !ok || string(data) != string(testData) {
		panic("cross-algorithm read failed")
	}
	fmt.Println("✓ Success")

	fmt.Println("Reading Snappy data with Gzip cache...")
	data, ok = gzipCache.Get("snappy-key")
	if !ok || string(data) != string(testData) {
		panic("cross-algorithm read failed")
	}
	fmt.Println("✓ Success")
	fmt.Println()

	fmt.Println("All caches can read each other's compressed data!")
}

func generateTestData(count int) []byte {
	// Generate a JSON-like structure that compresses well
	var builder strings.Builder
	builder.WriteString(`{"users": [`)

	for i := 0; i < count; i++ {
		if i > 0 {
			builder.WriteString(",")
		}
		fmt.Fprintf(&builder,
			`{"id": %d, "name": "User %d", "email": "user%d@example.com", "active": true, "roles": ["user", "admin"]}`,
			i, i, i)
	}

	builder.WriteString(`]}`)
	return []byte(builder.String())
}
