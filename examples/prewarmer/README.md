# Prewarmer Example

This example demonstrates how to use the Cache Prewarmer to prefetch and cache resources.

## Features Demonstrated

- Creating a prewarmer with custom configuration
- Sequential prewarming with progress callback
- Concurrent prewarming with multiple workers
- Force refresh mode
- Verifying cache population
- Context cancellation handling

## Running the Example

```bash
cd examples/prewarmer
go run main.go
```

## Expected Output

```
=== Sequential Prewarming ===
[1/3] ✓ https://httpbin.org/get (status=200, duration=245ms, cached=false)
[2/3] ✓ https://httpbin.org/headers (status=200, duration=198ms, cached=false)
[3/3] ✓ https://httpbin.org/user-agent (status=200, duration=187ms, cached=false)

Sequential Stats:
  Total:      3
  Successful: 3
  Failed:     0
  Duration:   630ms

=== Concurrent Prewarming (Force Refresh) ===
[1/3] ✓ https://httpbin.org/get (status=200, duration=210ms, cached=false)
[2/3] ✓ https://httpbin.org/headers (status=200, duration=195ms, cached=false)
[3/3] ✓ https://httpbin.org/user-agent (status=200, duration=188ms, cached=false)

Concurrent (refresh) Stats:
  Total:      3
  Successful: 3
  Failed:     0
  Duration:   215ms

=== Verifying Cache Population ===
https://httpbin.org/get - X-From-Cache: 1
https://httpbin.org/headers - X-From-Cache: 1
https://httpbin.org/user-agent - X-From-Cache: 1
```

## Configuration Options

```go
config := &prewarmer.Config{
    UserAgent:    "MyApp-Prewarmer/1.0", // Custom User-Agent
    Timeout:      30 * time.Second,       // Request timeout
    ForceRefresh: false,                  // Use cached versions if available
}
```

## Sitemap Support

The prewarmer can automatically parse XML sitemaps:

```go
stats, err := pw.PrewarmFromSitemap(ctx, "https://example.com/sitemap.xml", 10)
```

## See Also

- [Prewarmer Documentation](../../docs/prewarmer.md)
- [Basic Example](../basic/main.go)
