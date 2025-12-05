# MultiCache

`multicache` is a multi-tiered cache wrapper that enables sophisticated caching strategies with automatic fallback and promotion across multiple cache backends.

## Overview

MultiCache allows you to combine multiple cache backends (tiers) ordered from fastest/smallest to slowest/largest. It automatically:

- **Searches tiers in order** on GET operations (fastest to slowest)
- **Promotes data** to faster tiers when found in slower ones
- **Writes to all tiers** on SET operations
- **Maintains consistency** by deleting from all tiers on DELETE

This creates a natural data migration pattern where frequently accessed (hot) data moves to faster tiers, while less frequently accessed data remains in slower, more persistent tiers.

## Use Cases

### 1. Performance + Persistence

```
Tier 1: Memory (fast, volatile)
Tier 2: Disk (medium, persistent)
Tier 3: Database (slow, highly persistent)
```

### 2. Local + Distributed

```
Tier 1: Local memory (fastest, per-instance)
Tier 2: Redis (fast, shared across instances)
Tier 3: PostgreSQL (persistent, shared)
```

### 3. Size-Based Strategy

```
Tier 1: Small LRU cache (10 MB, hot data)
Tier 2: Larger disk cache (1 GB, warm data)
Tier 3: S3/Object storage (unlimited, cold data)
```

## Installation

```bash
go get github.com/sandrolain/httpcache/wrapper/multicache
```

## Basic Usage

```go
package main

import (
    "net/http"
    
    httpcache "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/diskcache"
    "github.com/sandrolain/httpcache/freecache"
    "github.com/sandrolain/httpcache/redis"
    "github.com/sandrolain/httpcache/wrapper/multicache"
)

func main() {
    // Create individual cache tiers
    tier1Cache := freecache.New(10 * 1024 * 1024)  // 10 MB in-memory
    diskCache := diskcache.New("/tmp/cache/tier2")
    redisCache, _ := redis.New("localhost:6379")
    
    // Combine into multi-tier cache (order matters!)
    mc := multicache.New(
        tier1Cache,  // Tier 1: fastest, checked first
        diskCache,   // Tier 2: medium speed
        redisCache,  // Tier 3: slowest, checked last
    )
    
    // Use with HTTP caching
    transport := httpcache.NewTransport(mc)
    client := &http.Client{Transport: transport}
}
```

## How It Works

### GET Operation

1. Check Tier 1 (fastest) → if found, return immediately
2. Check Tier 2 → if found, promote to Tier 1, then return
3. Check Tier 3 (slowest) → if found, promote to Tier 1 & 2, then return
4. If not found in any tier, return cache miss

### SET Operation

Write value to all tiers simultaneously, allowing each tier to apply its own eviction policies.

### DELETE Operation

Remove value from all tiers to maintain consistency.

## Example: Three-Tier Strategy

```go
package main

import (
    "fmt"
    "log"
    
    httpcache "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/diskcache"
    "github.com/sandrolain/httpcache/freecache"
    "github.com/sandrolain/httpcache/postgresql"
    "github.com/sandrolain/httpcache/wrapper/multicache"
)

func main() {
    // Tier 1: FreeCache - Fast, small (10 MB), in-memory
    tier1Cache := freecache.New(10 * 1024 * 1024)
    
    // Tier 2: Disk - Medium speed, larger (100 MB), survives restarts
    diskCache := diskcache.New("/var/cache/httpcache")
    
    // Tier 3: PostgreSQL - Slower, unlimited, highly persistent, shared
    pgCache, err := postgresql.New("postgresql://user:pass@localhost/cache")
    if err != nil {
        log.Fatal(err)
    }
    defer pgCache.Close()
    
    // Create multi-tier cache
    mc := multicache.New(tier1Cache, diskCache, pgCache)
    
    // Example: Store and retrieve
    mc.Set("user:123", []byte(`{"name":"John","email":"john@example.com"}`))
    
    // First Get: Reads from memory (fastest)
    data, ok := mc.Get("user:123")
    fmt.Printf("Found in cache: %v\n", ok)
    
    // Simulate memory cache eviction (e.g., LRU evicted it)
    tier1Cache.Delete("user:123")
    
    // Second Get: Reads from disk, promotes back to memory
    data, ok = mc.Get("user:123")
    fmt.Printf("Found and promoted: %v\n", ok)
    
    // Third Get: Reads from memory again (fast)
    data, ok = mc.Get("user:123")
    fmt.Printf("Data: %s\n", data)
}
```

## Configuration Tips

### Tier Ordering

Order tiers from **fastest/smallest to slowest/largest**:

```go
mc := multicache.New(
    fastSmallCache,      // Checked first, promotes to: none
    mediumCache,         // Checked second, promotes to: fast
    slowLargeCache,      // Checked last, promotes to: fast, medium
)
```

### Optimal Tier Count

- **2 tiers**: Simple setup (e.g., memory + disk)
- **3 tiers**: Balanced (e.g., memory + disk + database)
- **4+ tiers**: Advanced scenarios with many performance/persistence levels

### Cache Sizing

Size each tier appropriately for its role:

```go
// Tier 1: Small in-memory, holds only hot data
tier1Cache := freecache.New(100 * 1024 * 1024)  // ~100 MB in-memory

// Tier 2: Larger disk, holds warm data
diskCache := diskcache.New("/cache")   // ~1-10 GB

// Tier 3: Large, holds all cacheable data
pgCache := postgresql.New("...")       // Unlimited
```

## Performance Characteristics

- **Best case (hot data)**: Single lookup in Tier 1
- **Medium case (warm data)**: 2 lookups + 1 promotion write
- **Worst case (cold data)**: N lookups + (N-1) promotion writes (where N = number of tiers)
- **Miss case**: N lookups

The promotion mechanism ensures frequently accessed data naturally migrates to faster tiers, so best-case performance is achieved for hot data.

## Thread Safety

MultiCache is thread-safe as long as the underlying cache implementations are thread-safe. All built-in httpcache backends (memory, disk, Redis, PostgreSQL, etc.) are thread-safe.

## Validation

The `New()` function validates:

- At least one tier is provided
- No tier is `nil`
- No duplicate tiers

Returns `nil` if validation fails.

## Example Use Case: CDN-like Architecture

```go
// Create a CDN-like caching hierarchy
edge := diskcache.New("/tmp/cache/edge")  // Edge cache (fast, small)
regional := redis.New("regional:6379")    // Regional cache (medium)
origin := postgresql.New("origin-db")     // Origin cache (persistent)

cdn := multicache.New(edge, regional, origin)

// First request: Miss in all tiers, fetch from upstream, store everywhere
// Second request from same edge: Hit in edge (fastest)
// Request from different edge: Miss in edge, hit in regional, promote to edge
// Request after edge restart: Miss in edge, hit in regional, promote to edge
```

## Comparison with TwoTier

The `multicache` package is inspired by `lrucache/twotier` but extends it to support:

- **Any number of tiers** (not just two)
- **Automatic promotion** to all faster tiers (not just one)
- **Flexible tier configuration** for complex caching strategies

If you only need two tiers, both implementations work well. For three or more tiers, use `multicache`.

## Best Practices

1. **Order matters**: Always order tiers from fastest to slowest
2. **Independent eviction**: Let each tier manage its own eviction policy
3. **Monitor hit rates**: Track cache hits per tier to optimize sizing
4. **Test failure modes**: Ensure app works even if some tiers fail
5. **Consider costs**: Balance performance gains against complexity

## See Also

- [Main httpcache documentation](../../README.md)
- [Cache backends](../../docs/backends.md)
- [MultiCache example](../../examples/multicache/)
- [lrucache/twotier](https://github.com/die-net/lrucache/tree/main/twotier) - Inspiration for this package
