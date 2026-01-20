# Thread-Safety Guide

The httpcache package is designed for safe concurrent use across multiple goroutines. Understanding which operations are thread-safe helps you use the package correctly in concurrent environments.

## Overview

The package uses atomic operations, mutexes, and other synchronization primitives to ensure thread-safety where needed, while allowing high-performance concurrent access for read-heavy workloads.

## ✅ Safe for Concurrent Use

The following operations are fully thread-safe and can be called from multiple goroutines simultaneously:

### Core Transport Operations

- **`Transport.RoundTrip()`** - Core caching logic with atomic metrics and internal synchronization
- **`Transport.Client()`** - Returns an `*http.Client` that wraps the transport safely

### Metrics Operations

- **`TransportMetrics` operations** - All metric counters use `atomic.Int64` for lock-free updates:
  - `CacheHits.Add()`, `CacheMisses.Add()`, etc.
  - `Snapshot()` - Creates a consistent point-in-time view of all metrics

### Cache Interface

- **Cache interface implementations** - All provided backends (Memory, Disk, Redis, etc.) are thread-safe
- **Response cloning** - Internal response cloning mechanism uses `sync.RWMutex` for safe concurrent access

## ⚠️ Not Safe for Concurrent Use

The following operations should only be performed during initialization, before the transport is used:

### Modifying Transport Fields

**❌ UNSAFE: Don't modify fields after transport is in use**

```go
transport := httpcache.NewTransport(cache)
client := transport.Client()

go makeRequests(client)
transport.MaxCacheableResponseSize = 20 * 1024 * 1024  // ❌ Race condition!
```

**✅ SAFE: Configure before use**

```go
transport := httpcache.NewTransport(cache,
    httpcache.WithMaxCacheableResponseSize(20*1024*1024),
)
client := transport.Client()
go makeRequests(client)  // ✅ Safe
```

### Modifying Metrics Fields

**❌ UNSAFE: Direct field access**

```go
metrics := httpcache.NewTransportMetrics()

// Use atomic methods, but prefer letting Transport update
metrics.CacheHits.Store(100)  // ❌ Manual manipulation not recommended
```

**✅ SAFE: Let Transport handle updates**

```go
// Let Transport handle updates via WithMetrics()
transport := httpcache.NewTransport(cache, httpcache.WithMetrics(metrics))

// ✅ SAFE: Read-only snapshot
snapshot := metrics.Snapshot()
fmt.Printf("Hits: %d\n", snapshot.CacheHits)
```

## Concurrent Usage Examples

### Basic Concurrent Client Usage

Using the cached HTTP client from multiple goroutines:

```go
package main

import (
    "fmt"
    "net/http"
    "sync"
    
    "github.com/sandrolain/httpcache"
)

func main() {
    // Setup (done once)
    cache := httpcache.NewMemoryCache()
    transport := httpcache.NewTransport(cache)
    client := transport.Client()
    
    // Use from multiple goroutines safely
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            
            // ✅ Safe: RoundTrip is thread-safe
            resp, err := client.Get("https://api.example.com/data")
            if err != nil {
                fmt.Printf("Worker %d: error: %v\n", id, err)
                return
            }
            defer resp.Body.Close()
            
            // Check if cached
            if resp.Header.Get(httpcache.XFromCache) == "1" {
                fmt.Printf("Worker %d: cache hit\n", id)
            }
        }(i)
    }
    wg.Wait()
}
```

### Monitoring Metrics from Multiple Sources

Safe concurrent access to metrics:

```go
package main

import (
    "fmt"
    "time"
    
    "github.com/sandrolain/httpcache"
)

func main() {
    cache := httpcache.NewMemoryCache()
    
    // Setup metrics collection
    metrics := httpcache.NewTransportMetrics()
    transport := httpcache.NewTransport(cache, httpcache.WithMetrics(metrics))
    
    // Service goroutines can use client concurrently
    go handleAPIRequests(transport.Client())
    go handleWebhooks(transport.Client())
    
    // Monitoring goroutine reads metrics safely
    go func() {
        ticker := time.NewTicker(10 * time.Second)
        defer ticker.Stop()
        
        for range ticker.C {
            // ✅ Safe: Snapshot creates consistent view
            snapshot := metrics.Snapshot()
            fmt.Printf("Hit rate: %.2f%%\n", metrics.HitRate()*100)
            fmt.Printf("Total: %d, Hits: %d, Misses: %d\n",
                metrics.TotalRequests(),
                snapshot.CacheHits,
                snapshot.CacheMisses,
            )
        }
    }()
    
    // Keep main running
    select {}
}

func handleAPIRequests(client *http.Client) {
    // Your API handling logic
}

func handleWebhooks(client *http.Client) {
    // Your webhook handling logic
}
```

### Request Deduplication with Concurrent Requests

Coalescing concurrent requests to the same resource:

```go
package main

import (
    "sync"
    
    "github.com/sandrolain/httpcache"
)

func main() {
    cache := httpcache.NewMemoryCache()
    
    // Enable deduplication for high-traffic scenarios
    transport := httpcache.NewTransport(cache)
    transport.EnableDeduplication = true  // ✅ Set before use
    client := transport.Client()
    
    // Multiple concurrent requests to same URL are coalesced
    var wg sync.WaitGroup
    for i := 0; i < 50; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            // Only one request actually hits the origin
            resp, _ := client.Get("https://api.example.com/expensive-resource")
            resp.Body.Close()
        }()
    }
    wg.Wait()
}
```

## Cache Backend Thread-Safety

All provided cache backends implement thread-safe operations:

| Backend | Thread-Safety Mechanism |
|---------|------------------------|
| **Memory Cache** (`NewMemoryCache()`) | Uses `sync.RWMutex` |
| **Disk Cache** (`diskcache.New()`) | File-level locking with atomic operations |
| **Redis** (`redis.New()`) | Redis operations are atomic |
| **LevelDB** (`leveldbcache.New()`) | LevelDB handles concurrency internally |
| **PostgreSQL** (`postgresql.New()`) | Database-level concurrency control |
| **MongoDB** (`mongodb.New()`) | MongoDB handles concurrent operations |
| **NATS K/V** (`natskv.New()`) | NATS JetStream is concurrent-safe |
| **Hazelcast** (`hazelcast.New()`) | Distributed locking built-in |

### Custom Cache Backend Requirements

If implementing a custom cache backend, ensure your `Get()`, `Set()`, and `Delete()` methods are thread-safe, as they will be called concurrently by the transport.

Example of a thread-safe custom cache:

```go
type MyCache struct {
    mu    sync.RWMutex
    store map[string][]byte
}

func (c *MyCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    val, ok := c.store[key]
    if !ok {
        return nil, false, nil
    }
    
    // Return a copy to prevent external modification
    result := make([]byte, len(val))
    copy(result, val)
    return result, true, nil
}

func (c *MyCache) Set(ctx context.Context, key string, value []byte) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // Store a copy to prevent external modification
    c.store[key] = append([]byte(nil), value...)
    return nil
}

func (c *MyCache) Delete(ctx context.Context, key string) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    delete(c.store, key)
    return nil
}
```

## Internal Synchronization Details

For those interested in the implementation details:

### Atomic Operations

- **Metrics counters** - All use `atomic.Int64` for lock-free increments and updates
- **Concurrent reads** - Multiple goroutines can read metrics simultaneously without blocking

### Read-Write Mutexes

- **Response cloning** - Uses `sync.RWMutex` to protect concurrent access during header/body cloning
- **Allows concurrent readers** - Multiple goroutines can clone responses simultaneously
- **Minimal lock scope** - Locks are held only during the critical header copying phase

### Request Deduplication

- **Singleflight pattern** - Uses `golang.org/x/sync/singleflight` to coalesce concurrent identical requests
- **Automatic coordination** - When multiple goroutines request the same resource, only one request is made to the origin
- **Shared results** - All waiting goroutines receive the same response

### Buffer Pool

- **sync.Pool** - Uses Go's `sync.Pool` for efficient buffer reuse across goroutines
- **Automatic scaling** - Pool grows and shrinks based on load
- **Size-based pooling** - Only buffers below a certain size are pooled to prevent memory bloat

## Best Practices

### ✅ DO

1. **Configure during initialization** - Set all `Transport` fields before use
2. **Use options pattern** - Prefer `WithXxx()` options over direct field assignment
3. **Read metrics safely** - Use `Snapshot()` for consistent metric views
4. **Share client instances** - Create one client and use it across goroutines
5. **Enable deduplication** - For high-traffic scenarios, enable `EnableDeduplication` before use

### ❌ DON'T

1. **Don't modify transport after use** - Never change `Transport` fields after requests start
2. **Don't manually update metrics** - Let the transport handle metric updates
3. **Don't bypass cache interface** - Always use the provided cache methods
4. **Don't share mutable state** - Keep your own application state separate and synchronized
5. **Don't ignore errors** - Check and handle cache backend errors appropriately

## Performance Considerations

### Read-Heavy Workloads

The package is optimized for read-heavy workloads:

- RWMutex allows concurrent reads without blocking
- Atomic operations for metrics are lock-free
- Response cloning uses minimal locking

### Write-Heavy Workloads

For write-heavy scenarios:

- Consider using a distributed cache backend (Redis, Hazelcast)
- Enable request deduplication to reduce redundant origin requests
- Monitor cache error rates and adjust timeouts accordingly

### Memory Pressure

To optimize memory usage in concurrent environments:

- Configure `MaxCacheableResponseSize` to prevent large responses from being cached
- Use buffer pool configuration to control pooled buffer sizes
- Monitor metrics to identify cache efficiency

## Troubleshooting

### Race Detector

Always test concurrent code with Go's race detector:

```bash
go test -race ./...
```

### Common Issues

**Issue: Inconsistent cache behavior**

- **Cause**: Modifying `Transport` fields after use
- **Solution**: Configure transport before creating client

**Issue: High memory usage**

- **Cause**: Large responses being cached without size limits
- **Solution**: Set `MaxCacheableResponseSize` appropriately

**Issue: Cache misses on identical requests**

- **Cause**: Not enabling deduplication for concurrent requests
- **Solution**: Enable `EnableDeduplication` before use

## Additional Resources

- [Main README](../README.md) - Package overview and quick start
- [Advanced Features](./advanced-features.md) - Configuration options and features
- [Monitoring](./monitoring.md) - Metrics and observability
- [Security Considerations](./security.md) - Multi-user and security best practices
