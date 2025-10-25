# Prometheus Metrics Example

This example demonstrates how to use httpcache with Prometheus metrics collection.

## Features Demonstrated

- Creating a Prometheus metrics collector
- Instrumenting a cache backend (memory cache in this example)
- Instrumenting the HTTP transport
- Exposing metrics via HTTP endpoint
- Making HTTP requests that generate cache hits and misses
- Viewing metrics in Prometheus format

## Prerequisites

```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

## Running the Example

```bash
cd examples/prometheus
go run main.go
```

The example will:

1. Start a metrics server on <http://localhost:9090/metrics>
2. Make several HTTP requests to httpbin.org with different caching behaviors
3. Display cache hit/miss information for each request
4. Keep running to allow you to view the metrics

## Viewing Metrics

While the example is running, open your browser or use curl:

```bash
curl http://localhost:9090/metrics
```

You'll see metrics like:

```
# HELP httpcache_cache_requests_total Total number of cache operations
# TYPE httpcache_cache_requests_total counter
httpcache_cache_requests_total{backend="memory",operation="get",result="hit"} 11
httpcache_cache_requests_total{backend="memory",operation="get",result="miss"} 4
httpcache_cache_requests_total{backend="memory",operation="set",result="success"} 4

# HELP httpcache_http_requests_total Total number of HTTP requests
# TYPE httpcache_http_requests_total counter
httpcache_http_requests_total{cache_status="hit",method="GET"} 11
httpcache_http_requests_total{cache_status="miss",method="GET"} 4

# HELP httpcache_cache_operation_duration_seconds Cache operation duration in seconds
# TYPE httpcache_cache_operation_duration_seconds histogram
httpcache_cache_operation_duration_seconds_bucket{backend="memory",operation="get",le="0.0001"} 15
...

# HELP httpcache_http_response_size_bytes_total Total size of HTTP responses in bytes
# TYPE httpcache_http_response_size_bytes_total counter
httpcache_http_response_size_bytes_total{cache_status="hit",method="GET"} 4321
httpcache_http_response_size_bytes_total{cache_status="miss",method="GET"} 4321
```

## Available Metrics

### Cache Metrics

1. **httpcache_cache_requests_total** (counter)
   - Labels: `backend`, `operation`, `result`
   - Tracks cache operations (get, set, delete)

2. **httpcache_cache_operation_duration_seconds** (histogram)
   - Labels: `backend`, `operation`
   - Measures cache operation latency

3. **httpcache_cache_size_bytes** (gauge)
   - Labels: `backend`
   - Current cache size in bytes

4. **httpcache_cache_entries** (gauge)
   - Labels: `backend`
   - Current number of cached entries

### HTTP Metrics

5. **httpcache_http_requests_total** (counter)
   - Labels: `method`, `cache_status`
   - Total HTTP requests with cache status

6. **httpcache_http_request_duration_seconds** (histogram)
   - Labels: `method`, `cache_status`
   - HTTP request duration

7. **httpcache_http_response_size_bytes_total** (counter)
   - Labels: `method`, `cache_status`
   - Total response sizes

8. **httpcache_stale_responses_total** (counter)
   - Labels: `method`
   - Responses served from stale cache

## Example PromQL Queries

### Cache Hit Rate

```promql
rate(httpcache_cache_requests_total{result="hit"}[5m]) /
rate(httpcache_cache_requests_total{operation="get"}[5m]) * 100
```

### P95 Cache Latency

```promql
histogram_quantile(0.95,
  rate(httpcache_cache_operation_duration_seconds_bucket[5m]))
```

### HTTP Requests by Cache Status

```promql
sum by (cache_status) (httpcache_http_requests_total)
```

### Bandwidth Saved by Caching

```promql
httpcache_http_response_size_bytes_total{cache_status="hit"}
```

### Average Response Size by Cache Status

```promql
rate(httpcache_http_response_size_bytes_total[5m]) /
rate(httpcache_http_requests_total[5m])
```

## Integration with Existing Prometheus Setup

If your application already has a Prometheus metrics endpoint, you can integrate httpcache metrics:

```go
// Use the default Prometheus registry (shared with your app)
collector := prommetrics.NewCollector()

// Or use a custom registry
customRegistry := prometheus.NewRegistry()
collector := prommetrics.NewCollectorWithConfig(prommetrics.CollectorConfig{
    Registry: customRegistry,
    Namespace: "myapp", // Custom namespace instead of "httpcache"
})

// Your existing metrics endpoint will include httpcache metrics
http.Handle("/metrics", promhttp.Handler())
```

## Grafana Dashboard

You can create a Grafana dashboard with panels for:

1. **Cache Hit Rate** - Line graph showing cache efficiency over time
2. **Request Latency** - P50, P95, P99 percentiles for cache operations
3. **Cache Size** - Gauge showing current cache memory usage
4. **Traffic Volume** - Stacked graph of cache hits vs misses
5. **Bandwidth Savings** - Total bytes served from cache vs origin

Import the dashboard JSON from `grafana-dashboard.json` (coming soon).

## Production Considerations

1. **Cardinality**: Be mindful of label cardinality. The default labels are low-cardinality.

2. **Backend Label**: Use meaningful backend names (e.g., "redis-prod", "memcache-session") to differentiate multiple cache instances.

3. **Custom Namespaces**: Use custom namespaces if you have multiple httpcache instances:

   ```go
   prommetrics.NewCollectorWithConfig(prommetrics.CollectorConfig{
       Namespace: "api_cache",
   })
   ```

4. **Metrics Retention**: Configure Prometheus retention based on your needs (default is 15 days).

5. **Alerting**: Set up alerts for:
   - Low cache hit rate
   - High cache operation latency
   - Cache size approaching limits

## See Also

- [Main README](../../README.md) - httpcache documentation
- [Prometheus Documentation](https://prometheus.io/docs/introduction/overview/)
- [PromQL Query Examples](https://prometheus.io/docs/prometheus/latest/querying/examples/)
