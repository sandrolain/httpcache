# Freecache Backend Example

This example demonstrates how to use the freecache backend for HTTP caching with high performance and zero GC overhead.

## Features

- **Zero GC overhead**: Ideal for caching millions of entries without impacting garbage collection
- **Automatic LRU eviction**: Old entries are automatically evicted when cache is full
- **High concurrency**: Optimized for multi-threaded environments
- **Statistics**: Built-in metrics for hit rate, evictions, and more

## When to Use

Use the freecache backend when you need:

- Cache millions of HTTP responses
- Minimal GC impact on your application
- Automatic memory management with LRU eviction
- High-performance concurrent access

For smaller caches (< 100k entries), the standard in-memory cache is simpler and sufficient.

## Usage

```go
package main

import (
 "fmt"
 "io"
 "net/http"
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

 // Make HTTP requests
 resp, err := client.Get("https://api.example.com/data")
 if err != nil {
  panic(err)
 }
 defer resp.Body.Close()

 // Read response
 body, err := io.ReadAll(resp.Body)
 if err != nil {
  panic(err)
 }

 fmt.Printf("Response: %s\n", body)
 fmt.Printf("From cache: %s\n", resp.Header.Get("X-From-Cache"))

 // Check cache statistics
 fmt.Printf("Hit rate: %.2f%%\n", cache.HitRate()*100)
 fmt.Printf("Entries: %d\n", cache.EntryCount())
 fmt.Printf("Evictions: %d\n", cache.EvacuateCount())
}
```

## Memory Sizing

Choose cache size based on your needs:

```go
// Small cache (10MB) - ~10k cached responses
cache := freecache.New(10 * 1024 * 1024)

// Medium cache (100MB) - ~100k cached responses  
cache := freecache.New(100 * 1024 * 1024)

// Large cache (1GB) - ~1M cached responses
cache := freecache.New(1024 * 1024 * 1024)
debug.SetGCPercent(10) // Reduce GC frequency for large caches
```

## Performance Tips

1. **Set GC percentage**: For caches > 100MB, reduce GC frequency

   ```go
   debug.SetGCPercent(20) // or even lower (10, 5)
   ```

2. **Monitor statistics**: Use built-in metrics to tune cache size

   ```go
   fmt.Printf("Hit rate: %.2f%%\n", cache.HitRate()*100)
   fmt.Printf("Evictions: %d\n", cache.EvacuateCount())
   ```

3. **Size appropriately**: Cache should be large enough to avoid frequent evictions

## Comparison with Standard Memory Cache

| Feature | MemoryCache | Freecache |
|---------|-------------|-----------|
| GC Impact | High (for many entries) | Zero |
| Max Entries | Limited by GC | Millions |
| Eviction | Manual | Automatic LRU |
| Memory Usage | Variable | Fixed |
| Complexity | Simple | Complex |
| Dependencies | None | github.com/coocood/freecache |

## Running the Example

```bash
go run main.go
```
