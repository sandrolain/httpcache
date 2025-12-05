# Monitoring with Prometheus

httpcache includes **optional** Prometheus metrics integration to monitor cache performance, HTTP requests, and resource usage. The metrics system is:

- **Zero-dependency by default** - Metrics are opt-in and don't add dependencies to the core package
- **Non-intrusive** - Works with any existing cache backend
- **Production-ready** - Battle-tested metric types and labels
- **Integration-friendly** - Easily integrates with existing Prometheus setups

## Quick Start

```go
import (
    "github.com/sandrolain/httpcache"
    prommetrics "github.com/sandrolain/httpcache/wrapper/metrics/prometheus"
)

// Create metrics collector
collector := prommetrics.NewCollector()

// Wrap your cache (using disk cache as example)
cache := diskcache.New("/tmp/cache")
instrumentedCache := prommetrics.NewInstrumentedCache(cache, "disk", collector)

// Wrap your transport
transport := httpcache.NewTransport(instrumentedCache)
instrumentedTransport := prommetrics.NewInstrumentedTransport(transport, collector)

// Use the instrumented client
client := instrumentedTransport.Client()

// Metrics are automatically exposed via Prometheus default registry
// Access them at your /metrics endpoint
```

## Available Metrics

### Cache Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `httpcache_cache_requests_total` | Counter | `backend`, `operation`, `result` | Total cache operations (get/set/delete) |
| `httpcache_cache_operation_duration_seconds` | Histogram | `backend`, `operation` | Cache operation latency |
| `httpcache_cache_size_bytes` | Gauge | `backend` | Current cache size in bytes |
| `httpcache_cache_entries` | Gauge | `backend` | Number of cached entries |

### HTTP Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `httpcache_http_requests_total` | Counter | `method`, `cache_status` | HTTP requests by cache hit/miss/revalidated |
| `httpcache_http_request_duration_seconds` | Histogram | `method`, `cache_status` | HTTP request duration |
| `httpcache_http_response_size_bytes_total` | Counter | `method`, `cache_status` | Total response sizes |
| `httpcache_stale_responses_total` | Counter | `method` | Stale responses served (RFC 5861) |

## Example PromQL Queries

### Cache Hit Rate

```promql
rate(httpcache_cache_requests_total{result="hit"}[5m]) /
rate(httpcache_cache_requests_total{operation="get"}[5m]) * 100
```

### P95 Latency

```promql
histogram_quantile(0.95,
  rate(httpcache_cache_operation_duration_seconds_bucket[5m]))
```

### Bandwidth Saved

```promql
httpcache_http_response_size_bytes_total{cache_status="hit"}
```

### Traffic Distribution

```promql
sum by (cache_status) (rate(httpcache_http_requests_total[5m]))
```

## Integration with Existing Metrics

If your application already has Prometheus metrics, httpcache metrics are automatically included:

```go
// Your existing Prometheus setup
http.Handle("/metrics", promhttp.Handler())

// httpcache metrics use the default registry
collector := prommetrics.NewCollector()

// All metrics are exposed together at /metrics
```

For custom namespaces or registries:

```go
customRegistry := prometheus.NewRegistry()
collector := prommetrics.NewCollectorWithConfig(prommetrics.CollectorConfig{
    Registry:  customRegistry,
    Namespace: "myapp", // Use "myapp_cache_requests_total" instead of "httpcache_..."
})
```

## Configuration

### Custom Histogram Buckets

```go
collector := prommetrics.NewCollectorWithConfig(prommetrics.CollectorConfig{
    HistogramBuckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
})
```

### Custom Namespace

```go
collector := prommetrics.NewCollectorWithConfig(prommetrics.CollectorConfig{
    Namespace: "myapp", // Metrics will be prefixed with "myapp_"
})
```

### Custom Registry

```go
registry := prometheus.NewRegistry()
collector := prommetrics.NewCollectorWithConfig(prommetrics.CollectorConfig{
    Registry: registry,
})

// Expose on separate endpoint
http.Handle("/httpcache-metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
```

## Grafana Dashboard

See [`examples/prometheus/README.md`](../examples/prometheus/README.md) for Grafana dashboard recommendations and sample queries.

## Production Considerations

1. **Label Cardinality**: Keep label values bounded to avoid metric explosion
2. **Namespaces**: Use custom namespaces when running multiple instances
3. **Alerting**: Set up alerts for low hit rates or high latencies
4. **Sampling**: Consider sampling for very high-traffic applications

For a complete working example, see [`examples/prometheus/`](../examples/prometheus/).
