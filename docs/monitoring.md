# Monitoring and Metrics

httpcache includes an **internal metrics system** with zero external dependencies. Metrics are collected using atomic operations for thread-safety and can be optionally exported to Prometheus or other monitoring systems.

## Architecture

The metrics system has two layers:

1. **Internal Metrics** (`TransportMetrics`) - Zero-dependency collection in the core package
2. **Export Layer** (optional) - Wrappers for Prometheus, OpenTelemetry, etc.

This design ensures:

- ✅ **Zero overhead when disabled** - Simple nil check, no allocation
- ✅ **No external dependencies** - Core package remains dependency-free
- ✅ **Thread-safe** - Atomic operations, no locks
- ✅ **Flexible export** - Export to any monitoring system

## Quick Start

### Basic Metrics

```go
import "github.com/sandrolain/httpcache"

// Create metrics
metrics := httpcache.NewTransportMetrics()

// Enable metrics on transport
transport := httpcache.NewTransport(cache, httpcache.WithMetrics(metrics))
client := &http.Client{Transport: transport}

// Make requests...

// Read metrics
fmt.Printf("Hit rate: %.2f%%\n", metrics.HitRate()*100)
fmt.Printf("Total requests: %d\n", metrics.TotalRequests())
fmt.Printf("Cache hits: %d\n", metrics.CacheHits.Load())
fmt.Printf("Cache misses: %d\n", metrics.CacheMisses.Load())
fmt.Printf("Stale served: %d\n", metrics.StaleServed.Load())
fmt.Printf("Deduplicated: %d\n", metrics.Deduplication.Load())
```

### Prometheus Export

```go
import (
    "context"
    "net/http"
    
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/sandrolain/httpcache"
    prommetrics "github.com/sandrolain/httpcache/wrapper/metrics/prometheus"
)

func main() {
    // 1. Create internal metrics
    metrics := httpcache.NewTransportMetrics()
    
    // 2. Enable on transport
    transport := httpcache.NewTransport(cache, httpcache.WithMetrics(metrics))
    client := &http.Client{Transport: transport}
    
    // 3. Create Prometheus exporter
    collector := prommetrics.NewCollector(prommetrics.CollectorConfig{
        Metrics:   metrics,
        Namespace: "myapp",    // Optional: custom namespace
        Subsystem: "cache",    // Optional: custom subsystem
    })
    
    // 4. Start periodic updates (default: 10s interval)
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    stop := collector.Start(ctx)
    defer stop()
    
    // 5. Expose /metrics endpoint
    http.Handle("/metrics", promhttp.Handler())
    http.ListenAndServe(":9090", nil)
}
```

## Internal Metrics

### Available Metrics

`TransportMetrics` tracks the following:

| Metric | Type | Description |
|--------|------|-------------|
| `CacheHits` | Counter | Number of successful cache hits |
| `CacheMisses` | Counter | Number of cache misses |
| `CacheErrors` | Counter | Number of cache operation errors |
| `StaleServed` | Counter | Stale responses served (stale-if-error) |
| `Deduplication` | Counter | Requests deduplicated via singleflight |
| `CachedBytes` | Gauge | Approximate bytes in cache |
| `CacheLatencyBuckets` | Histogram | Latency distribution (10 buckets) |

### Buffer Pool Metrics

`httpcache` also provides **buffer pool metrics** to monitor internal memory management:

```go
// Get current buffer pool metrics
metrics := httpcache.GetBufferPoolMetrics()

fmt.Printf("Buffer Pool Statistics:\n")
fmt.Printf("  Gets: %d\n", metrics.Gets)
fmt.Printf("  Puts: %d\n", metrics.Puts)
fmt.Printf("  Pool Hits: %d\n", metrics.PoolHits)
fmt.Printf("  Pool Miss: %d\n", metrics.PoolMiss)
fmt.Printf("  Discarded: %d\n", metrics.Discarded)
fmt.Printf("  Hit Rate: %.2f%%\n", metrics.PoolHitRate())
fmt.Printf("  Discard Rate: %.2f%%\n", metrics.DiscardRate())
```

**Buffer Pool Metrics:**

| Field | Description |
|-------|-------------|
| `Gets` | Total buffer retrievals from pool |
| `Puts` | Total buffers returned to pool |
| `PoolHits` | Buffers reused from pool (efficient) |
| `PoolMiss` | New buffers allocated (cache miss) |
| `Discarded` | Buffers too large to pool (> maxPooledBufferSize) |

**Helper Methods:**

- `PoolHitRate()` - Returns pool efficiency as percentage (0-100%)
- `DiscardRate()` - Returns discard rate as percentage of total Puts (0-100%)

**Use Cases:**

- **Monitor memory efficiency**: High pool hit rate = good reuse
- **Optimize buffer size**: High discard rate → increase `maxPooledBufferSize`
- **Identify memory pressure**: Track allocation patterns over time
- **Performance tuning**: Correlate with cache performance metrics

**Reset for Testing:**

```go
// Reset buffer pool metrics (useful for testing)
httpcache.ResetBufferPoolMetrics()
```

### Latency Histogram Buckets

Latency is tracked in 10 buckets:

- Bucket 0: < 1ms
- Bucket 1: 1-5ms
- Bucket 2: 5-10ms
- Bucket 3: 10-25ms
- Bucket 4: 25-50ms
- Bucket 5: 50-100ms
- Bucket 6: 100-250ms
- Bucket 7: 250-500ms
- Bucket 8: 500-1000ms
- Bucket 9: > 1000ms

### API Reference

```go
// Create metrics
metrics := httpcache.NewTransportMetrics()

// Read counters
hits := metrics.CacheHits.Load()
misses := metrics.CacheMisses.Load()
errors := metrics.CacheErrors.Load()
stale := metrics.StaleServed.Load()
dedup := metrics.Deduplication.Load()
bytes := metrics.CachedBytes.Load()

// Calculate hit rate (0.0 - 1.0)
hitRate := metrics.HitRate()

// Get total requests
total := metrics.TotalRequests()

// Get consistent snapshot of all metrics
snapshot := metrics.Snapshot()
fmt.Printf("Hit rate: %.2f%%\n", snapshot.HitRate*100)

// Reset all metrics (useful for testing)
metrics.Reset()

// Access latency buckets
for i, count := range metrics.CacheLatencyBuckets {
    boundary := metrics.GetLatencyBucketBoundary(i)
    fmt.Printf("Latency %dms: %d\n", boundary, count.Load())
}
```

## Prometheus Export

### Exported Metrics

When using the Prometheus wrapper, these metrics are exposed:

| Metric | Type | Description |
|--------|------|-------------|
| `httpcache_cache_hits_total` | Gauge | Total cache hits |
| `httpcache_cache_misses_total` | Gauge | Total cache misses |
| `httpcache_cache_errors_total` | Gauge | Total cache errors |
| `httpcache_stale_served_total` | Gauge | Total stale responses |
| `httpcache_deduplication_total` | Gauge | Total deduplicated requests |
| `httpcache_cache_hit_rate` | Gauge | Current hit rate (0-1) |
| `httpcache_cached_bytes` | Gauge | Bytes in cache |
| `httpcache_total_requests` | Gauge | Total requests (hits + misses) |

### Configuration Options

```go
collector := prommetrics.NewCollector(prommetrics.CollectorConfig{
    Metrics:        metrics,           // Required: internal metrics
    Namespace:      "myapp",           // Optional: metric prefix (default: "httpcache")
    Subsystem:      "cache",           // Optional: metric subsystem (default: "")
    UpdateInterval: 5 * time.Second,   // Optional: update frequency (default: 10s)
})
```

### Custom Registry

```go
// Use custom Prometheus registry
registry := prometheus.NewRegistry()
collector := prommetrics.NewCollectorWithRegistry(registry, metrics)

// Expose on separate endpoint
http.Handle("/cache-metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
```

### Example PromQL Queries

```promql
# Current hit rate
httpcache_cache_hit_rate

# Hit rate over time
rate(httpcache_cache_hits_total[5m]) / 
rate(httpcache_total_requests[5m])

# Cache misses per second
rate(httpcache_cache_misses_total[1m])

# Stale responses served
httpcache_stale_served_total

# Deduplication effectiveness
httpcache_deduplication_total

# Cache size in MB
httpcache_cached_bytes / 1024 / 1024
```

## Performance Impact

### With Metrics Enabled

**Cache Metrics:**

- **Cache hit**: ~20ns overhead (2 atomic loads + 1 add + 1 time.Since)
- **Cache miss**: ~20ns overhead (2 atomic loads + 1 add + 1 time.Since)
- **Cache error**: ~15ns overhead (3 atomic operations)
- **Total overhead**: < 0.1% on typical cache operations (100-1000µs)

**Buffer Pool Metrics:**

- **getBuffer()**: ~2-4ns overhead (2 atomic.Add operations)
- **putBuffer()**: ~2-4ns overhead (1-2 atomic.Add operations)
- **GetBufferPoolMetrics()**: ~5-10ns overhead (5 atomic.Load operations)
- **Total overhead**: < 0.1% on buffer pool operations

### With Metrics Disabled

- **Zero overhead**: Only a nil check (`if t.Metrics != nil`)
- Compiler optimizes the branch away
- **Note**: Buffer pool metrics are always active (global, minimal overhead)

## Thread-Safety

All metrics use `atomic.Int64` for lock-free concurrent access:

```go
// Safe from multiple goroutines
for i := 0; i < 1000; i++ {
    go func() {
        metrics.CacheHits.Add(1)
    }()
}
```

The `Snapshot()` method provides a consistent point-in-time view:

```go
// Get consistent snapshot (all values from same logical time)
snapshot := metrics.Snapshot()
log.Printf("Hits: %d, Misses: %d, Rate: %.2f%%",
    snapshot.CacheHits,
    snapshot.CacheMisses,
    snapshot.HitRate*100,
)
```

## Monitoring Best Practices

### 1. Alert on Low Hit Rate

```yaml
# Prometheus alert for cache hit rate
- alert: LowCacheHitRate
  expr: httpcache_cache_hit_rate < 0.5
  for: 5m
  annotations:
    summary: "Cache hit rate below 50%"
```

### 2. Monitor Cache Errors

```yaml
- alert: HighCacheErrors
  expr: rate(httpcache_cache_errors_total[5m]) > 10
  for: 2m
  annotations:
    summary: "High cache error rate"
```

### 3. Track Stale Responses

```promql
# Percentage of stale responses
httpcache_stale_served_total / httpcache_total_requests * 100
```

### 4. Deduplication Effectiveness

```promql
# How many requests were saved by deduplication
httpcache_deduplication_total
```

### 5. Cache Size Monitoring

```yaml
- alert: CacheSizeTooLarge
  expr: httpcache_cached_bytes > 1e9  # 1GB
  for: 5m
  annotations:
    summary: "Cache size exceeds 1GB"
```

### 6. Buffer Pool Monitoring

Monitor buffer pool efficiency to optimize memory usage:

```go
// Periodic monitoring
go func() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        metrics := httpcache.GetBufferPoolMetrics()
        
        log.Printf("Buffer Pool: hits=%d miss=%d rate=%.1f%% discarded=%d",
            metrics.PoolHits,
            metrics.PoolMiss,
            metrics.PoolHitRate(),
            metrics.Discarded)
        
        // Alert if pool efficiency is low
        if metrics.PoolHitRate() < 80.0 {
            log.Warn("Low buffer pool hit rate - consider tuning")
        }
        
        // Alert if discard rate is high
        if metrics.DiscardRate() > 10.0 {
            log.Warn("High buffer discard rate - increase maxPooledBufferSize")
        }
    }
}()
```

**Buffer Pool Alerts:**

- **Low hit rate** (< 80%): Pool not reusing buffers efficiently
- **High discard rate** (> 10%): Consider increasing `maxPooledBufferSize` with `WithMaxPooledBufferSize()` option
- **High allocation rate**: May indicate memory pressure

## Grafana Dashboard

Example dashboard panels:

### Hit Rate Panel

```promql
httpcache_cache_hit_rate * 100
```

### Request Rate Panel

```promql
sum by (status) (
  rate(httpcache_cache_hits_total[5m]),
  rate(httpcache_cache_misses_total[5m])
)
```

### Latency Distribution (using internal histogram)

You can export latency buckets to create distribution graphs in Grafana.

## Migration from v1.x

If you were using the old Prometheus wrapper:

**Before (v1.x):**

```go
collector := prommetrics.NewCollector()
cache := prommetrics.NewInstrumentedCache(baseCache, "disk", collector)
transport := httpcache.NewTransport(cache)
instrumentedTransport := prommetrics.NewInstrumentedTransport(transport, collector)
client := instrumentedTransport.Client()
```

**After (v2.0):**

```go
metrics := httpcache.NewTransportMetrics()
transport := httpcache.NewTransport(cache, httpcache.WithMetrics(metrics))
client := &http.Client{Transport: transport}

// Optional: Export to Prometheus
collector := prommetrics.NewCollector(prommetrics.CollectorConfig{Metrics: metrics})
stop := collector.Start(context.Background())
defer stop()
```

**Benefits of v2.0:**

- ✅ Simpler API (no wrapper types)
- ✅ Zero dependencies in core
- ✅ Better performance (atomic operations)
- ✅ Easier testing (direct metric access)
- ✅ More flexible (export to any system)

## Examples

See complete working examples:

- [Basic Metrics](../examples/metrics/) - Simple metrics usage
- [Prometheus Integration](../examples/prometheus/) - Full Prometheus setup

For more information on monitoring setup, see [`examples/prometheus/README.md`](../examples/prometheus/README.md).
