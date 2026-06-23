# Migration Guide: v1 to v2

This guide provides step-by-step instructions for upgrading from httpcache v1 to v2.

## Table of Contents

- [Overview](#overview)
- [v1 (master) comparison](#v1-master-comparison)
- [Breaking Changes](#breaking-changes)
- [Step-by-Step Migration](#step-by-step-migration)
- [Custom Cache Backend Migration](#custom-cache-backend-migration)
- [Configuration Changes](#configuration-changes)
- [Performance Considerations](#performance-considerations)
- [Testing Your Migration](#testing-your-migration)
- [Troubleshooting](#troubleshooting)

## Overview

httpcache v2 is a major upgrade focusing on performance optimization while maintaining most of the v1 API. The migration path is designed to be straightforward for most use cases.

**Key Improvements in v2:**

- 79-82% faster buffer operations with zero allocations
- 63-85% faster hash key generation with 90-95% less memory
- 67-94% faster Cache-Control parsing
- 98.7% faster repeated key lookups
- 42-73% faster header normalization

For detailed performance metrics, see [Performance Optimization](./performance-v2.md).

## v1 (master) Comparison

This section compares migration-relevant usage between v1 (`master` branch) and v2.

| Area | v1 (master) | v2 | Migration action |
|------|-------------|----|------------------|
| Quick in-memory transport | `httpcache.NewMemoryCacheTransport()` | Removed | Create a backend explicitly, then use `httpcache.NewTransport(cache, ...)` |
| Cache interface | `Get(key)`, `Set(key, value)`, `Delete(key)` | `Get(ctx, key)`, `Set(ctx, key, value) error`, `Delete(ctx, key) error`, plus stale methods | Update custom backends or use a temporary adapter |
| Stored key format | Plain request-derived keys | Hashed keys before backend storage | Expect cache warm-up after upgrade for persistent stores |
| Hash algorithm setting | Not exposed in v1 API | `WithHashAlgorithm(SHA256|XXHash)` | Keep SHA-256 when you need stable v2 behavior; use XXHash for throughput |

Notes from the comparison:

- `NewMemoryCacheTransport()` exists on v1 `master`, but not in v2.
- v2 `Cache` interface is a breaking change for custom backend implementations.
- Existing v1 persisted entries are not directly reusable in v2 because key storage format differs.

## Breaking Changes

### 1. Hash Algorithm Default

**v1 Behavior:**  
SHA-256 is the only hash algorithm available.

**v2 Behavior:**  
xxHash is the default hash algorithm for better performance.

**Migration Impact:**  
Existing cache entries with SHA-256 keys will miss after upgrading. The cache will rebuild naturally over time.

**Compatibility note from v1 comparison:**  
v1 `master` stores plain request-derived keys, while v2 stores hashed keys. Even when v2 uses SHA-256, persisted v1 entries are not read directly because the key format differs.

**Options:**

A) **Accept cache rebuild** (recommended):

```go
// Use default xxHash for best performance
transport := httpcache.NewTransport(cache)
```

B) **Maintain SHA-256 for compatibility**:

```go
// Keep existing keys valid
transport := httpcache.NewTransport(cache,
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmSHA256),
)
```

C) **Gradual migration**:

```go
// 1. Deploy v2 with SHA-256 to maintain cache
transport := httpcache.NewTransport(cache,
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmSHA256),
)

// 2. After cache expires naturally, switch to xxHash
// (In a later deployment)
transport := httpcache.NewTransport(cache,
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmXXHash),
)
```

### 2. Context-Based API

**v1 Behavior:**  
Cache key computation happens per request without memoization.

**v2 Behavior:**  
Cache keys are memoized in request context for 98.7% faster repeated lookups.

**Migration Impact:**  
Minimal. The context-based memoization is transparent to most users.

**Code Changes:**  
None required. Memoization is automatic when using standard `http.Request` with context.

**Note:** If you manually create requests without proper context, ensure you use `http.NewRequestWithContext()` instead of deprecated `http.NewRequest()`.

### 3. Buffer Pooling

**v1 Behavior:**  
Buffer allocation for each response body read.

**v2 Behavior:**  
Sophisticated buffer pooling with size-based pools.

**Migration Impact:**  
None. Buffer pooling is automatic and transparent.

**Memory Considerations:**  
Buffer pools retain memory for reuse. In extremely memory-constrained environments, this may increase baseline memory usage slightly, but drastically reduces allocation overhead.

### 4. `NewMemoryCacheTransport()` Removal

**v1 Behavior:**  
`httpcache.NewMemoryCacheTransport()` was available as a convenience constructor.

**v2 Behavior:**  
Use an explicit cache backend with `httpcache.NewTransport(cache, ...)`.

**Migration Example:**

```go
// v1
transport := httpcache.NewMemoryCacheTransport()

// v2
cache := freecache.New(100 * 1024 * 1024)
transport := httpcache.NewTransport(cache)
```

### 5. Cache Interface Changes for Custom Backends

**v1 Interface (master):**

```go
type Cache interface {
    Get(key string) (responseBytes []byte, ok bool)
    Set(key string, responseBytes []byte)
    Delete(key string)
}
```

**v2 Interface:**

```go
type Cache interface {
    Get(ctx context.Context, key string) (responseBytes []byte, ok bool, err error)
    Set(ctx context.Context, key string, responseBytes []byte) error
    Delete(ctx context.Context, key string) error
    MarkStale(ctx context.Context, key string) error
    IsStale(ctx context.Context, key string) (bool, error)
    GetStale(ctx context.Context, key string) (responseBytes []byte, ok bool, err error)
}
```

**Migration Impact:**  
Custom cache backend implementations from v1 must be updated.

## Step-by-Step Migration

### Step 1: Update Import Statement

No changes required. The import path remains the same:

```go
import "github.com/sandrolain/httpcache"
```

### Step 2: Review Hash Algorithm Choice

Decide whether to keep SHA-256 or switch to xxHash:

**Before (v1 - implicit):**

```go
transport := httpcache.NewTransport(cache)
// Uses SHA-256 (only option)
```

**After (v2 - default xxHash):**

```go
transport := httpcache.NewTransport(cache)
// Uses xxHash by default
```

**After (v2 - explicit SHA-256 for compatibility):**

```go
transport := httpcache.NewTransport(cache,
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmSHA256),
)
```

### Step 2.1: Replace `NewMemoryCacheTransport()` (if used)

If your v1 code uses `NewMemoryCacheTransport()`, migrate to explicit backend initialization:

```go
cache := freecache.New(100 * 1024 * 1024)
transport := httpcache.NewTransport(cache)
```

### Step 3: Update Monitoring/Metrics

v2 includes enhanced debug headers for cache behavior inspection:

```go
// Enable debug headers to understand cache behavior
transport := httpcache.NewTransport(cache)
transport.MarkCachedResponses = true

// Response headers in v2:
// X-From-Cache: "1" (hit)
// X-Cache-Freshness: "fresh" | "stale" | "stale-while-revalidate"
// X-Revalidated: "1" (if revalidated)
// X-Stale: "1" (if stale)
```

### Step 4: Test and Deploy

1. **Unit Tests:** Run existing test suite
2. **Integration Tests:** Verify cache behavior
3. **Performance Tests:** Run benchmarks to confirm improvements
4. **Canary Deployment:** Roll out to small subset first
5. **Monitor:** Watch cache hit rates and performance metrics
6. **Full Deployment:** Roll out to all instances

## Custom Cache Backend Migration

If you implemented the v1 custom `Cache` interface, use this temporary adapter pattern to migrate incrementally.

```go
type LegacyCache interface {
    Get(key string) ([]byte, bool)
    Set(key string, value []byte)
    Delete(key string)
}

type LegacyAdapter struct {
    inner LegacyCache
}

func (a *LegacyAdapter) Get(_ context.Context, key string) ([]byte, bool, error) {
    v, ok := a.inner.Get(key)
    return v, ok, nil
}

func (a *LegacyAdapter) Set(_ context.Context, key string, value []byte) error {
    a.inner.Set(key, value)
    return nil
}

func (a *LegacyAdapter) Delete(_ context.Context, key string) error {
    a.inner.Delete(key)
    return nil
}

func (a *LegacyAdapter) MarkStale(ctx context.Context, key string) error {
    // Fallback behavior for legacy backends without stale semantics.
    return a.Delete(ctx, key)
}

func (a *LegacyAdapter) IsStale(_ context.Context, _ string) (bool, error) {
    return false, nil
}

func (a *LegacyAdapter) GetStale(_ context.Context, _ string) ([]byte, bool, error) {
    return nil, false, nil
}
```

## Configuration Changes

### Transport Options

All v1 options are preserved in v2:

| Option | v1 | v2 | Notes |
|--------|----|----|-------|
| `WithMaxCacheableResponseSize()` | ✅ | ✅ | No change |
| `WithCacheOperationTimeout()` | ✅ | ✅ | No change |
| `WithHashAlgorithm()` | ✅ | ✅ | Default changed from SHA-256 to xxHash |
| `WithLogger()` | ✅ | ✅ | No change |

### Transport Properties

All v1 properties are preserved in v2:

| Property | v1 | v2 | Notes |
|----------|----|----|-------|
| `MarkCachedResponses` | ✅ | ✅ | No change |
| `EnableDeduplication` | ✅ | ✅ | No change |
| `IsPublicCache` | ✅ | ✅ | No change |
| `CacheKeyHeaders` | ✅ | ✅ | No change |

## Performance Considerations

### Memory Usage

**v2 Memory Characteristics:**

- **Buffer pools:** Retain allocated buffers for reuse
- **Parsing cache:** Caches parsed Cache-Control headers
- **Key memoization:** Stores computed keys in request context

**Expected Memory Impact:**

- Slightly higher baseline memory (buffer pools and caches)
- Dramatically lower peak memory (fewer allocations)
- Overall: Better memory efficiency under load

**Monitoring:**

```go
import _ "net/http/pprof"

// Monitor heap allocations
// Before: frequent spikes from buffer allocations
// After: smooth baseline from buffer reuse
```

### CPU Usage

**v2 CPU Characteristics:**

- xxHash is CPU-friendly (SIMD optimizations)
- Cached parsing reduces CPU for repeated headers
- Memoization eliminates redundant computations

**Expected CPU Impact:**

- Lower CPU usage per request
- Better CPU utilization under high concurrency
- Reduced garbage collection pressure

### Configuration Examples

#### High-Performance Configuration

```go
// Optimal for throughput and low latency
transport := httpcache.NewTransport(cache,
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmXXHash),
    httpcache.WithMaxCacheableResponseSize(10*1024*1024), // 10MB
    httpcache.WithCacheOperationTimeout(30*time.Second),
)
transport.EnableDeduplication = true
```

#### Memory-Constrained Configuration

```go
// Optimal for limited memory environments
transport := httpcache.NewTransport(cache,
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmXXHash), // Smaller keys
    httpcache.WithMaxCacheableResponseSize(1*1024*1024),        // 1MB limit
    httpcache.WithCacheOperationTimeout(15*time.Second),
)
// Deduplication disabled to avoid singleflight memory
transport.EnableDeduplication = false
```

#### Backward-Compatible Configuration

```go
// Maintains v1 behavior
transport := httpcache.NewTransport(cache,
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmSHA256), // v1 default
    httpcache.WithMaxCacheableResponseSize(10*1024*1024),
    httpcache.WithCacheOperationTimeout(30*time.Second),
)
// Use same settings as v1
```

## Testing Your Migration

### Unit Tests

Your existing unit tests should pass without modification:

```bash
go test ./...
```

### Performance Benchmarks

Compare v1 and v2 performance:

```bash
# v1 benchmarks
git checkout v1-tag
go test -bench=. -benchmem -benchtime=5s

# v2 benchmarks
git checkout v2-tag
go test -bench=. -benchmem -benchtime=5s
```

Expected improvements:

- Memory cache operations: Similar performance (optimized in both)
- Header normalization: 42-73% faster in v2
- Hash key generation: 63-85% faster with xxHash in v2
- Cache-Control parsing: 67-94% faster in v2 (with cache hits)

### Integration Tests

Test cache behavior with real HTTP traffic:

```go
func TestV2CacheBehavior(t *testing.T) {
    cache := httpcache.NewMemoryCache()
    transport := httpcache.NewTransport(cache)
    transport.MarkCachedResponses = true
    
    client := &http.Client{Transport: transport}
    
    // First request - cache miss
    resp1, err := client.Get("https://example.com/api")
    if err != nil {
        t.Fatal(err)
    }
    defer resp1.Body.Close()
    
    if resp1.Header.Get("X-From-Cache") == "1" {
        t.Error("Expected cache miss on first request")
    }
    
    // Second request - cache hit
    resp2, err := client.Get("https://example.com/api")
    if err != nil {
        t.Fatal(err)
    }
    defer resp2.Body.Close()
    
    if resp2.Header.Get("X-From-Cache") != "1" {
        t.Error("Expected cache hit on second request")
    }
}
```

### Load Testing

Test under production-like load:

```bash
# Use your favorite load testing tool
# Example with hey:
hey -n 10000 -c 100 https://your-service/api

# Monitor:
# - Cache hit rate
# - Response times (p50, p95, p99)
# - Memory usage
# - CPU usage
```

## Troubleshooting

### Cache Misses After Upgrade

**Symptom:** Cache hit rate drops to 0% immediately after deploying v2.

**Cause:** Hash algorithm changed from SHA-256 to xxHash (default).

**Solution:**

Option 1: Accept temporary cache rebuild (recommended):

```go
// Use default xxHash - cache will rebuild naturally
transport := httpcache.NewTransport(cache)
```

Option 2: Use SHA-256 for backward compatibility:

```go
transport := httpcache.NewTransport(cache,
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmSHA256),
)
```

Option 3: Pre-warm cache before full deployment:

```bash
# Deploy to single instance first
# Let cache warm up
# Then deploy to remaining instances
```

### Increased Memory Usage

**Symptom:** Baseline memory usage increased after v2 deployment.

**Cause:** Buffer pools and parsing caches retain memory for reuse.

**Solution:**

This is expected behavior. v2 trades slightly higher baseline memory for much better performance. If memory is truly constrained:

```go
// Reduce max cacheable size
transport := httpcache.NewTransport(cache,
    httpcache.WithMaxCacheableResponseSize(1*1024*1024), // 1MB instead of 10MB
)
```

**Monitoring:**

```go
// Monitor actual memory usage over time
import _ "net/http/pprof"

// Access: http://localhost:6060/debug/pprof/heap
```

### Unexpected Cache Behavior

**Symptom:** Responses are cached or not cached unexpectedly.

**Cause:** Debug headers disabled, making it hard to diagnose.

**Solution:**

Enable debug headers:

```go
transport.MarkCachedResponses = true

// Check response headers:
// X-From-Cache: "1" means cache hit
// X-Cache-Freshness: shows freshness state
// X-Revalidated: "1" means response was revalidated
```

### Performance Not Improved

**Symptom:** Benchmarks don't show expected performance improvements.

**Cause:** Multiple possible causes.

**Checklist:**

1. **Verify v2 is actually deployed:**

   ```bash
   go list -m github.com/sandrolain/httpcache
   ```

2. **Verify xxHash is enabled:**

   ```go
   // Explicitly use xxHash
   transport := httpcache.NewTransport(cache,
       httpcache.WithHashAlgorithm(httpcache.HashAlgorithmXXHash),
   )
   ```

3. **Verify cache is actually being used:**

   ```go
   transport.MarkCachedResponses = true
   // Check X-From-Cache header
   ```

4. **Run proper benchmarks:**

   ```bash
   go test -bench=. -benchmem -benchtime=5s -count=3
   ```

5. **Check for CPU throttling:**

   ```bash
   # Ensure CPU isn't throttled during benchmarks
   go test -bench=. -benchmem -cpu=1,2,4
   ```

### Build Errors

**Symptom:** Build fails after upgrading to v2.

**Cause:** Dependency version mismatch or cache issues.

**Solution:**

```bash
# Clear module cache
go clean -modcache

# Update dependencies
go get -u github.com/sandrolain/httpcache

# Tidy dependencies
go mod tidy

# Verify clean build
go build ./...
```

## Additional Resources

- [Performance Benchmarks](./performance-v2.md) - Detailed v1 vs v2 comparison
- [Advanced Features](./advanced-features.md) - In-depth feature documentation
- [How It Works](./how-it-works.md) - Implementation details
- [RFC 9111 Compliance](../context/RFC_9111_ALIGNMENT.md) - HTTP caching standard compliance

## Support

If you encounter issues not covered in this guide:

1. Check [GitHub Issues](https://github.com/sandrolain/httpcache/issues)
2. Review [Examples](../examples/) for working code samples
3. Open a new issue with:
   - v1 configuration
   - v2 configuration
   - Expected behavior
   - Actual behavior
   - Minimal reproduction code

## Version Information

- **v1**: Stable release, maintained for critical bug fixes
- **v2**: Current release, recommended for new deployments
- **Migration Path**: v1 → v2 (direct upgrade supported)
- **Backward Compatibility**: High (with hash algorithm consideration)
