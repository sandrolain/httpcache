# Performance Optimization in v2

This document provides a comprehensive comparison between httpcache v1 and v2, highlighting the significant performance improvements and new optimizations introduced in version 2.

## Table of Contents

- [Performance Optimization in v2](#performance-optimization-in-v2)
  - [Table of Contents](#table-of-contents)
  - [Overview](#overview)
  - [v1 Baseline Performance](#v1-baseline-performance)
    - [Cache Operations](#cache-operations)
    - [Header Processing](#header-processing)
    - [Vary Matching](#vary-matching)
    - [Request Deduplication](#request-deduplication)
    - [Memory Protection](#memory-protection)
    - [Cache Operation Timeout](#cache-operation-timeout)
    - [Hash Algorithm Selection](#hash-algorithm-selection)
    - [Buffer Pool Configuration](#buffer-pool-configuration)
  - [v2 Performance Improvements](#v2-performance-improvements)
    - [1. Enhanced Buffer Pooling](#1-enhanced-buffer-pooling)
    - [2. Improved Hash Key Performance](#2-improved-hash-key-performance)
    - [3. Cache-Control Parsing Cache](#3-cache-control-parsing-cache)
    - [4. Cache Key Memoization](#4-cache-key-memoization)
    - [5. Optimized Header Normalization](#5-optimized-header-normalization)
  - [v1 vs v2 Performance Comparison](#v1-vs-v2-performance-comparison)
    - [Test Environment](#test-environment)
    - [Memory Cache Operations](#memory-cache-operations)
      - [Get Operations](#get-operations)
      - [Set Operations](#set-operations)
    - [Header Normalization](#header-normalization)
    - [Vary Header Matching](#vary-header-matching)
  - [v2 Performance Tuning Options](#v2-performance-tuning-options)
    - [Available Configuration Options](#available-configuration-options)
      - [1. Request Deduplication](#1-request-deduplication)
      - [2. Memory Protection](#2-memory-protection)
      - [3. Cache Operation Timeout](#3-cache-operation-timeout)
      - [4. Hash Algorithm](#4-hash-algorithm)
      - [5. Buffer Pool Size](#5-buffer-pool-size)
    - [Example: High-Performance Configuration](#example-high-performance-configuration)
    - [Example: Memory-Constrained Configuration](#example-memory-constrained-configuration)
  - [Best Practices](#best-practices)
    - [For High-Throughput Scenarios](#for-high-throughput-scenarios)
    - [For Memory-Constrained Environments](#for-memory-constrained-environments)
    - [For Latency-Sensitive Applications](#for-latency-sensitive-applications)
  - [Profiling and Monitoring](#profiling-and-monitoring)
  - [Summary](#summary)
  - [Migration Overview](#migration-overview)
    - [Breaking Changes](#breaking-changes)
    - [Gradual Migration](#gradual-migration)
    - [Expected Benefits](#expected-benefits)

## Overview

httpcache v2 represents a major performance upgrade over v1, introducing several significant optimizations while maintaining backward compatibility where possible.

**v1 Baseline Features:**

- **Single-pass header normalization** for efficient Vary header processing
- **Fast-path optimization** for exact header value matches
- **Efficient memory cache** with minimal allocation overhead
- **Optimized string operations** to reduce garbage collection pressure
- **Request deduplication** via singleflight (optional, disabled by default)
- **Memory protection** with configurable max cacheable response size (default: 10MB)
- **Cache operation timeout** to prevent indefinite cache writes (default: 30s)
- **Configurable hash algorithms** (SHA-256 default, xxHash available)
- **Buffer pool configuration** for optimal memory/performance trade-off

**v2 New Optimizations:**

- **Enhanced buffer pooling** for zero-allocation HTTP response handling (79-82% faster)
- **Improved xxHash implementation** for 63-85% faster key hashing with 90-95% less memory
- **Cache-Control parsing cache** for 67-94% faster repeated parsing
- **Cache key memoization** for 98.7% faster repeated lookups
- **Optimized header normalization** with 42-73% speed improvements and 50-68% memory reduction

**Migration Path:** For users upgrading from v1, see the [Migration Guide](./migration-v1-to-v2.md).

## v1 Baseline Performance

Version 1 established a solid performance baseline with the following characteristics:

### Cache Operations

| Operation    | Time        | Memory | Allocations |
|--------------|-------------|--------|-------------|
| Get          | 11.82 ns/op | 0 B/op | 0 allocs/op |
| Set          | 29.09 ns/op | 0 B/op | 0 allocs/op |
| Delete       | 60.21 ns/op | 4 B/op | 1 allocs/op |
| SetGet       | 38.49 ns/op | 0 B/op | 0 allocs/op |
| Parallel Get | 73.93 ns/op | 0 B/op | 0 allocs/op |
| Parallel Set | 111.9 ns/op | 4 B/op | 1 allocs/op |

### Header Processing

| Operation             | Time          | Memory      | Allocations   |
|-----------------------|---------------|-------------|---------------|
| Simple normalization  | 35.98 ns/op   | 8 B/op      | 1 allocs/op   |
| Complex normalization | 117-273 ns/op | 32-152 B/op | 3-5 allocs/op |
| Exact match           | 2.387 ns/op   | 0 B/op      | 0 allocs/op   |
| Normalized match      | 79-320 ns/op  | 16-136 B/op | 2-7 allocs/op |

### Vary Matching

| Scenario         | Time        | Memory   | Allocations |
|------------------|-------------|----------|-------------|
| No Vary header   | 33.21 ns/op | 4 B/op   | 1 allocs/op |
| Single header    | 185.1 ns/op | 36 B/op  | 3 allocs/op |
| Multiple headers | 589.4 ns/op | 168 B/op | 8 allocs/op |

### Request Deduplication

v1 includes optional request deduplication using Go's `singleflight` to coalesce concurrent requests to the same resource.

**Configuration:**

```go
transport := httpcache.NewTransport(cache)
transport.EnableDeduplication = true  // Default: false
```

**Benefits:**

- Reduces backend load by coalescing concurrent identical requests
- Single network request for multiple concurrent callers
- All callers receive the same response
- Particularly useful for high-traffic scenarios or slow backends

**Use Cases:**

- High-traffic APIs with many concurrent requests
- Slow backends where response time > request frequency
- Microservices with thundering herd patterns
- Cache warming scenarios

**Performance Impact:**

When enabled, deduplication adds minimal overhead (~few microseconds) for coordination but significantly reduces backend load:

- **Without deduplication**: 10 concurrent requests = 10 backend calls
- **With deduplication**: 10 concurrent requests = 1 backend call (9 requests wait and share result)

See `httpcache_singleflight_bench_test.go` for detailed benchmarks.

### Memory Protection

v1 includes configurable memory protection to prevent exhaustion from large responses.

**MaxCacheableResponseSize:**

```go
transport := httpcache.NewTransport(cache,
    httpcache.WithMaxCacheableResponseSize(5*1024*1024),  // 5MB limit
)
```

**Default**: 10MB (10 *1024* 1024 bytes)  
**Disable**: Set to 0 (not recommended for production)

**Behavior:**

- Responses exceeding the limit are streamed without caching
- Large responses are still served normally to clients
- Prevents memory exhaustion from unexpectedly large payloads
- No impact on cache hits or normal-sized responses

### Cache Operation Timeout

v1 includes configurable timeout for cache write operations.

**CacheOperationTimeout:**

```go
transport := httpcache.NewTransport(cache,
    httpcache.WithCacheOperationTimeout(15*time.Second),  // 15s timeout
)
```

**Default**: 30 seconds  
**Disable**: Set to 0 (uses context.Background() without timeout)

**Purpose:**

- Prevents cache operations from running indefinitely
- Uses independent context to allow cache writes even if client disconnects
- Provides reasonable timeout for cache completion
- Prevents resource leaks from stuck cache operations

### Hash Algorithm Selection

v1 supports multiple hash algorithms for cache key generation.

**Available Algorithms:**

- **SHA-256** (default): Cryptographically secure, backward compatible
- **xxHash**: ~2.7x faster, 72% smaller output, recommended for high-throughput

**Configuration:**

```go
transport := httpcache.NewTransport(cache,
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmXXHash),
)
```

**Warning**: Changing hash algorithm invalidates existing cache entries.

**Performance Comparison (v1):**

| Algorithm | Speed        | Memory       | Output Size       |
|-----------|--------------|--------------|-------------------|
| SHA-256   | baseline     | 176-320 B/op | 43 bytes (base64) |
| xxHash    | ~2.7x faster | 16 B/op      | 12 bytes (base36) |

### Buffer Pool Configuration

v1 includes configurable buffer pooling for memory efficiency.

**MaxPooledBufferSize:**

```go
transport := httpcache.NewTransport(cache,
    httpcache.WithMaxPooledBufferSize(128*1024),  // 128KB
)
```

**Default**: 64KB (64 * 1024 bytes)

**Purpose:**

- Reuse buffers to reduce allocations
- Prevent memory bloat from very large buffers
- Configurable based on typical response sizes
- Larger values increase memory pool but improve performance for large responses

## v2 Performance Improvements

### 1. Enhanced Buffer Pooling

Version 2 implements sophisticated buffer pooling for HTTP response body handling, dramatically reducing allocations for large responses.

**Performance Impact:**

| Buffer Size | v1 (WithoutPool)        | v2 (WithPool)       | Improvement                      |
|-------------|-------------------------|---------------------|----------------------------------|
| 1KB         | 131.4 ns/op, 1024 B/op  | 27.59 ns/op, 0 B/op | **79.0% faster, no allocations** |
| 10KB        | 988.6 ns/op, 10240 B/op | 184.1 ns/op, 0 B/op | **81.4% faster, no allocations** |
| 32KB        | 2685 ns/op, 32768 B/op  | 483.0 ns/op, 0 B/op | **82.0% faster, no allocations** |
| 64KB        | 5408 ns/op, 65536 B/op  | 950.6 ns/op, 0 B/op | **82.4% faster, no allocations** |

**Benefits:**

- 79-82% faster for buffers >1KB
- Zero allocations for all buffer operations
- Reduced GC pressure
- Better throughput for large responses

### 2. Improved Hash Key Performance

Version 2 provides an optimized xxHash implementation as a high-performance alternative to SHA-256 for cache key generation.

**Performance Comparison:**

| Key Length         | SHA-256 (v1)          | xxHash (v2)          | Improvement                         |
|--------------------|-----------------------|----------------------|-------------------------------------|
| Short (~40 chars)  | 121.5 ns/op, 176 B/op | 36.48 ns/op, 16 B/op | **70.0% faster, 90.9% less memory** |
| Medium (~65 chars) | 145.7 ns/op, 208 B/op | 53.20 ns/op, 16 B/op | **63.5% faster, 92.3% less memory** |
| Long (~150 chars)  | 207.4 ns/op, 320 B/op | 73.06 ns/op, 16 B/op | **64.8% faster, 95.0% less memory** |
| Parallel           | 80.17 ns/op, 192 B/op | 11.47 ns/op, 16 B/op | **85.7% faster, 91.7% less memory** |

**Hash Output Sizes:**

- SHA-256 (base64): 43 bytes
- SHA-256 (hex): 64 bytes
- xxHash (base36): 12 bytes (72% smaller than SHA-256 base64)

### 3. Cache-Control Parsing Cache

Version 2 implements intelligent caching for parsed Cache-Control headers, eliminating redundant parsing of common directives.

**Performance Impact:**

| Scenario       | v1 (Uncached)       | v2 (Cached)         | Improvement                      |
|----------------|---------------------|---------------------|----------------------------------|
| Simple header  | ~104 ns/op, 32 B/op | 31.61 ns/op, 0 B/op | **69.6% faster, no allocations** |
| Complex header | ~104 ns/op, 32 B/op | 33.99 ns/op, 0 B/op | **67.3% faster, no allocations** |
| Concurrent     | N/A                 | 6.423 ns/op, 0 B/op | **93.8% faster**                 |

### 4. Cache Key Memoization

Version 2 utilizes context-based cache key memoization to eliminate redundant key computations for repeated requests.

**Performance Impact:**

| Operation        | v1 (Direct)           | v2 (Memoized)       | Improvement                      |
|------------------|-----------------------|---------------------|----------------------------------|
| Cache key lookup | 309.8 ns/op, 216 B/op | 3.914 ns/op, 0 B/op | **98.7% faster, no allocations** |

### 5. Optimized Header Normalization

Version 2 features a highly optimized header normalization algorithm with significantly improved speed and memory efficiency.

**Performance Comparison:**

| Scenario                | v1                    | v2                   | Improvement                         |
|-------------------------|-----------------------|----------------------|-------------------------------------|
| Comma with spaces       | 117.5 ns/op, 32 B/op  | 31.27 ns/op, 16 B/op | **73.4% faster, 50% less memory**   |
| Complex Accept-Language | 272.6 ns/op, 152 B/op | 91.07 ns/op, 48 B/op | **66.6% faster, 68.4% less memory** |
| Accept-Encoding         | 229.4 ns/op, 88 B/op  | 67.87 ns/op, 32 B/op | **70.4% faster, 63.6% less memory** |
| With whitespace         | 115.6 ns/op, 40 B/op  | 39.39 ns/op, 16 B/op | **65.9% faster, 60% less memory**   |

## v1 vs v2 Performance Comparison

### Test Environment

- **Platform**: macOS (darwin/arm64)
- **CPU**: Apple M2
- **Go Version**: 1.25.5
- **Benchmark Duration**: 2 seconds per test
- **Date**: January 2026

### Memory Cache Operations

#### Get Operations

| Benchmark          | v1                  | v2                  | Change          |
|--------------------|---------------------|---------------------|-----------------|
| Basic Get          | 11.82 ns/op, 0 B/op | 11.64 ns/op, 0 B/op | **1.5% faster** |
| HTTP Response Get  | 16.47 ns/op, 0 B/op | 20.70 ns/op, 0 B/op | 25.7% slower    |
| Large Response Get | 15.47 ns/op, 0 B/op | 15.19 ns/op, 0 B/op | **1.8% faster** |
| Parallel Get       | 73.93 ns/op, 0 B/op | 94.38 ns/op, 0 B/op | 27.6% slower    |

#### Set Operations

| Benchmark          | v1                  | v2                  | Change          |
|--------------------|---------------------|---------------------|-----------------|
| Basic Set          | 29.09 ns/op, 0 B/op | 26.66 ns/op, 0 B/op | **8.4% faster** |
| HTTP Response Set  | 32.43 ns/op, 4 B/op | 29.34 ns/op, 4 B/op | **9.5% faster** |
| Large Response Set | 30.11 ns/op, 4 B/op | 30.74 ns/op, 4 B/op | 2.1% slower     |
| Parallel Set       | 111.9 ns/op, 4 B/op | 107.0 ns/op, 4 B/op | **4.4% faster** |

### Header Normalization

| Scenario                | v1                    | v2                   | Improvement                         |
|-------------------------|-----------------------|----------------------|-------------------------------------|
| Simple                  | 35.98 ns/op, 8 B/op   | 20.65 ns/op, 8 B/op  | **42.6% faster**                    |
| Comma (no spaces)       | 55.63 ns/op, 8 B/op   | 29.04 ns/op, 8 B/op  | **47.8% faster**                    |
| Comma (with spaces)     | 117.5 ns/op, 32 B/op  | 31.27 ns/op, 16 B/op | **73.4% faster, 50% less memory**   |
| Complex Accept-Language | 272.6 ns/op, 152 B/op | 91.07 ns/op, 48 B/op | **66.6% faster, 68.4% less memory** |
| Accept-Encoding         | 229.4 ns/op, 88 B/op  | 67.87 ns/op, 32 B/op | **70.4% faster, 63.6% less memory** |

### Vary Header Matching

| Scenario         | v1                    | v2                    | Improvement                        |
|------------------|-----------------------|-----------------------|------------------------------------|
| Matching headers | 711.8 ns/op, 200 B/op | 439.8 ns/op, 132 B/op | **38.2% faster, 34% less memory**  |
| Non-matching     | 319.7 ns/op, 104 B/op | 250.9 ns/op, 96 B/op  | **21.5% faster, 7.7% less memory** |
| Exact match      | 307.6 ns/op, 68 B/op  | 304.6 ns/op, 68 B/op  | **1.0% faster**                    |
| No Vary          | 33.21 ns/op, 4 B/op   | 32.78 ns/op, 4 B/op   | **1.3% faster**                    |

## v2 Performance Tuning Options

### Available Configuration Options

v2 provides several performance tuning options to optimize for your specific use case:

#### 1. Request Deduplication

**Option**: `EnableDeduplication`  
**Default**: `false`  
**Use When**: High concurrent traffic to same resources

```go
transport := httpcache.NewTransport(cache)
transport.EnableDeduplication = true
```

**Impact**:

- Reduces backend load significantly with concurrent requests
- Minimal overhead (~microseconds) for coordination
- Best for: APIs with high concurrency, slow backends

#### 2. Memory Protection

**Option**: `MaxCacheableResponseSize`  
**Default**: `10MB` (10 *1024* 1024)  
**Use When**: Need to prevent memory exhaustion

```go
transport := httpcache.NewTransport(cache,
    httpcache.WithMaxCacheableResponseSize(5*1024*1024),
)
```

**Impact**:

- Prevents caching of large responses
- Zero impact on normal-sized responses
- Best for: Shared hosting, memory-constrained environments

#### 3. Cache Operation Timeout

**Option**: `CacheOperationTimeout`  
**Default**: `30 seconds`  
**Use When**: Need to prevent stuck cache operations

```go
transport := httpcache.NewTransport(cache,
    httpcache.WithCacheOperationTimeout(15*time.Second),
)
```

**Impact**:

- Prevents indefinite cache writes
- Allows cache completion even after client disconnect
- Best for: Production environments with timeouts

#### 4. Hash Algorithm

**Option**: `HashAlgorithm`  
**Default**: `HashAlgorithmSHA256`  
**Alternative**: `HashAlgorithmXXHash` (2.7x faster)

```go
transport := httpcache.NewTransport(cache,
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmXXHash),
)
```

**Impact**:

- xxHash: 2.7x faster, 72% smaller keys
- SHA-256: Cryptographically secure, backward compatible
- Best for: High-throughput scenarios (xxHash), security-critical (SHA-256)
- ⚠️ **Warning**: Changing invalidates existing cache

#### 5. Buffer Pool Size

**Option**: `MaxPooledBufferSize`  
**Default**: `64KB` (64 * 1024)  
**Use When**: Response sizes differ from default

```go
transport := httpcache.NewTransport(cache,
    httpcache.WithMaxPooledBufferSize(128*1024),
)
```

**Impact**:

- Larger values: Better performance for large responses, more memory
- Smaller values: Lower memory usage, may allocate more often
- Best for: Applications with consistent response sizes

### Example: High-Performance Configuration

```go
cache := httpcache.NewMemoryCache()
transport := httpcache.NewTransport(cache,
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmXXHash),  // Faster hashing
    httpcache.WithMaxCacheableResponseSize(20*1024*1024),        // 20MB limit
    httpcache.WithMaxPooledBufferSize(128*1024),                 // 128KB buffers
    httpcache.WithCacheOperationTimeout(15*time.Second),         // 15s timeout
)
transport.EnableDeduplication = true  // Enable request coalescing

client := &http.Client{Transport: transport}
```

### Example: Memory-Constrained Configuration

```go
cache := httpcache.NewMemoryCache()
transport := httpcache.NewTransport(cache,
    httpcache.WithMaxCacheableResponseSize(2*1024*1024),   // 2MB limit
    httpcache.WithMaxPooledBufferSize(32*1024),            // 32KB buffers
    httpcache.WithCacheOperationTimeout(10*time.Second),   // 10s timeout
)

client := &http.Client{Transport: transport}
```

## Best Practices

### For High-Throughput Scenarios

1. **Enable request deduplication**: Reduce backend load for concurrent identical requests

   ```go
   transport.EnableDeduplication = true
   ```

2. **Use xxHash for cache keys**: 2.7x faster than SHA-256

   ```go
   httpcache.WithHashAlgorithm(httpcache.HashAlgorithmXXHash)
   ```

3. **Use memory cache for hot data**: Memory cache provides the best performance (~12-30 ns/op)

4. **Configure appropriate cache size**: Prevent evictions to maintain performance

5. **Monitor allocation rates**: Use profiling to identify bottlenecks

### For Memory-Constrained Environments

1. **Configure max cacheable size**: Prevent caching of very large responses

   ```go
   httpcache.WithMaxCacheableResponseSize(5*1024*1024)  // 5MB
   ```

2. **Adjust buffer pool size**: Balance memory usage vs performance

   ```go
   httpcache.WithMaxPooledBufferSize(32*1024)  // 32KB for smaller footprint
   ```

3. **Use disk cache or Redis**: Trade some performance for reduced memory usage

4. **Set cache eviction**: Use LRU or TTL-based eviction strategies

5. **Monitor memory usage**: Set appropriate limits to prevent OOM

### For Latency-Sensitive Applications

1. **Enable request deduplication**: Prevent duplicate concurrent requests

   ```go
   transport.EnableDeduplication = true
   ```

2. **Use local caches**: Minimize network latency (memory or disk cache)

3. **Enable stale-while-revalidate**: Serve stale responses while updating in background

4. **Configure appropriate timeouts**: Balance between freshness and latency

   ```go
   httpcache.WithCacheOperationTimeout(15*time.Second)
   ```

5. **Monitor cache hit rates**: Low hit rates indicate inefficient caching

## Profiling and Monitoring

To profile httpcache performance in your application:

```go
import (
    "net/http/pprof"
    "runtime/pprof"
)

// CPU profiling
f, _ := os.Create("cpu.prof")
pprof.StartCPUProfile(f)
defer pprof.StopCPUProfile()

// Memory profiling
f, _ := os.Create("mem.prof")
runtime.GC()
pprof.WriteHeapProfile(f)
f.Close()
```

Analyze with:

```bash
go tool pprof cpu.prof
go tool pprof mem.prof
```

For production monitoring, use the built-in metrics:

```go
metrics := httpcache.NewMetrics()
transport := httpcache.NewTransport(cache, httpcache.WithMetrics(metrics))

// Monitor hit rate, latency, errors
fmt.Printf("Hit Rate: %.2f%%\n", metrics.HitRate())
fmt.Printf("Requests: %d\n", metrics.Requests())
```

## Summary

Version 2 delivers substantial performance improvements across all major operations:

- **Header Processing**: 42-73% faster with 50-68% less memory
- **Buffer Operations**: 79-82% faster for large buffers with zero allocations
- **Hash Keys**: 63-85% faster with 90-95% less memory
- **Cache-Control Parsing**: 67-94% faster when cached
- **Key Lookups**: 98.7% faster with memoization

For detailed migration instructions, see the [Migration Guide](./migration-v1-to-v2.md).

## Migration Overview

When upgrading from v1 to v2, consider the following:

### Breaking Changes

- **Hash Algorithm Change**: Switching from SHA-256 to xxHash will invalidate existing cache entries. Plan for cache warmup period.
- **Context-Based API**: Some APIs may require context.Context parameters for improved cancellation support.

### Gradual Migration

1. **Test in Non-Production**: Benchmark v2 with your specific workload
2. **Monitor Metrics**: Compare hit rates, latency, and memory usage
3. **Cache Warmup**: Plan for cache invalidation when changing hash algorithms
4. **Rollback Plan**: Keep v1 deployment ready for quick rollback if needed

### Expected Benefits

- **High-Throughput**: 63-85% faster hash operations, 79-82% faster buffer operations
- **Memory Efficiency**: 50-95% less memory for various operations
- **GC Pressure**: Significantly reduced due to buffer pooling and zero-allocation operations
- **Latency**: Lower P99 latency due to reduced allocation overhead

---

For more information:

- [How It Works](./how-it-works.md) - Implementation details
- [Monitoring](./monitoring.md) - Prometheus metrics integration
- [Advanced Features](./advanced-features.md) - Configuration options
