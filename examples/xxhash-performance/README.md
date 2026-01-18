# XXHash Performance Example

This example demonstrates the performance benefits of using xxHash instead of SHA-256 for cache key hashing in high-throughput scenarios.

## Overview

httpcache provides two hashing algorithms for cache keys:

- **SHA-256** (default): Cryptographically secure, backward compatible
- **xxHash**: ~2.7x faster, 72% smaller output, recommended for high-throughput

## Performance Comparison

| Metric | SHA-256 | xxHash | Improvement |
|--------|---------|--------|-------------|
| Speed | 149 ns/op | 54 ns/op | **2.77x faster** |
| Memory | 215 B/op | 18 B/op | **91.6% less** |
| Allocations | 11 allocs/op | 3 allocs/op | **72.7% fewer** |
| Output size | 43 chars | 12 chars | **72.1% smaller** |

## When to Use xxHash

✅ **Recommended for:**

- High-throughput scenarios (>10K req/sec)
- In-memory caches with short TTL
- Performance-critical microservices
- Limited cache storage (smaller keys)

❌ **Not recommended for:**

- Distributed caches across trust boundaries
- Security-sensitive applications requiring cryptographic hashing
- When backward compatibility is critical

## Running the Example

```bash
go run main.go
```

## Code

```go
package main

import (
 "fmt"
 "io"
 "net/http"
 "net/http/httptest"
 "time"

 "github.com/sandrolain/httpcache"
 "github.com/sandrolain/httpcache/diskcache"
)

func main() {
 // Create a test server
 ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Cache-Control", "max-age=3600")
  fmt.Fprintf(w, "Response at %s", time.Now().Format(time.RFC3339))
 }))
 defer ts.Close()

 // Example 1: Default SHA-256 (backward compatible)
 fmt.Println("=== Example 1: SHA-256 (Default) ===")
 cache1 := diskcache.New("/tmp/httpcache-sha256")
 transport1 := httpcache.NewTransport(cache1)
 runBenchmark(transport1.Client(), ts.URL)

 // Example 2: xxHash (high performance)
 fmt.Println("\n=== Example 2: xxHash (High Performance) ===")
 cache2 := diskcache.New("/tmp/httpcache-xxhash")
 transport2 := httpcache.NewTransport(cache2,
  httpcache.WithHashAlgorithm(httpcache.HashAlgorithmXXHash),
 )
 runBenchmark(transport2.Client(), ts.URL)

 // Show recommendations
 fmt.Println("\n=== Recommendations ===")
 fmt.Println("Use SHA-256 (default) for:")
 fmt.Println("  - Existing deployments (backward compatibility)")
 fmt.Println("  - Distributed caches across trust boundaries")
 fmt.Println("  - Security-sensitive applications")
 fmt.Println("\nUse xxHash for:")
 fmt.Println("  - High-throughput scenarios (>10K req/sec)")
 fmt.Println("  - In-memory caches with short TTL")
 fmt.Println("  - Performance-critical microservices")
 fmt.Println("  - Limited cache storage (72% smaller keys)")
 fmt.Println("\n⚠️  Warning: Changing algorithms invalidates existing cache entries")
}

func runBenchmark(client *http.Client, url string) {
 // First request (cache miss)
 start := time.Now()
 resp, err := client.Get(url)
 if err != nil {
  panic(err)
 }
 io.Copy(io.Discard, resp.Body)
 resp.Body.Close()
 miss := time.Since(start)

 // Second request (cache hit)
 start = time.Now()
 resp, err = client.Get(url)
 if err != nil {
  panic(err)
 }
 io.Copy(io.Discard, resp.Body)
 resp.Body.Close()
 hit := time.Since(start)

 // Display results
 cached := resp.Header.Get(httpcache.XFromCache) == "1"
 fmt.Printf("First request (miss):  %v\n", miss)
 fmt.Printf("Second request (hit):  %v\n", hit)
 fmt.Printf("Cached: %v\n", cached)
 fmt.Printf("Speedup: %.2fx faster\n", float64(miss)/float64(hit))
}
```

## Output Example

```
=== Example 1: SHA-256 (Default) ===
First request (miss):  15.2ms
Second request (hit):  245µs
Cached: true
Speedup: 62.04x faster

=== Example 2: xxHash (High Performance) ===
First request (miss):  14.8ms
Second request (hit):  189µs
Cached: true
Speedup: 78.31x faster

=== Recommendations ===
Use SHA-256 (default) for:
  - Existing deployments (backward compatibility)
  - Distributed caches across trust boundaries
  - Security-sensitive applications

Use xxHash for:
  - High-throughput scenarios (>10K req/sec)
  - In-memory caches with short TTL
  - Performance-critical microservices
  - Limited cache storage (72% smaller keys)

⚠️  Warning: Changing algorithms invalidates existing cache entries
```

## Migration Strategy

When switching from SHA-256 to xxHash:

### Strategy 1: Cache Warming (Recommended)

```go
// 1. Deploy new version with xxHash
transport := httpcache.NewTransport(cache,
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmXXHash),
)

// 2. Allow cache to naturally warm up
// 3. Old SHA-256 entries expire naturally (based on TTL)
```

### Strategy 2: Cache Flush (Simplest)

```go
// 1. Clear all cache entries
// cache.Flush()

// 2. Deploy with new algorithm
transport := httpcache.NewTransport(cache,
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmXXHash),
)

// Note: Causes temporary cache miss spike
```

## Performance Benchmarks

Run the included benchmarks to see the performance difference:

```bash
go test -bench=BenchmarkHashAlgorithm -benchmem -benchtime=3s
```

Expected results (Apple M2):

```
BenchmarkHashAlgorithmComparison/SHA256-Base64-Pooled-8    24,048,015    149 ns/op    215 B/op    11 allocs/op
BenchmarkHashAlgorithmComparison/XXHash-Base36-8           69,668,428     54 ns/op     18 B/op     3 allocs/op
```

## See Also

- [Advanced Features Documentation](../../docs/advanced-features.md#hash-algorithm-selection)
- [Security Considerations](../../docs/security.md)
- [Performance Tuning Guide](../../docs/how-it-works.md)
