# Cache Prewarmer

The Cache Prewarmer is a utility that allows you to prefetch and cache resources before they are needed by users. This helps ensure optimal performance by having frequently accessed resources ready in the cache.

## Features

- **Sequential and concurrent prewarming**: Warm cache entries one at a time or in parallel with configurable workers
- **Sitemap support**: Parse XML sitemaps and sitemap indexes to automatically extract URLs
- **Progress callbacks**: Monitor prewarming progress in real-time
- **Force refresh**: Option to bypass existing cache entries and refresh content
- **Detailed statistics**: Track success rates, errors, and timing information
- **Context support**: Full context cancellation and timeout support

## Installation

The prewarmer is included as part of the httpcache package:

```go
import "github.com/sandrolain/httpcache/wrapper/prewarmer"
```

## Basic Usage

### Create a Prewarmer

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/freecache"
    "github.com/sandrolain/httpcache/wrapper/prewarmer"
)

func main() {
    // Create a cache transport
    cache := freecache.New(100 * 1024 * 1024) // 100MB
    transport := httpcache.NewTransport(cache)
    client := transport.Client()

    // Create a prewarmer with configuration
    pw, err := prewarmer.New(prewarmer.Config{
        Client: client,
    })
    if err != nil {
        log.Fatal(err)
    }

    // URLs to prewarm
    urls := []string{
        "https://example.com/",
        "https://example.com/about",
        "https://example.com/contact",
    }

    // Prewarm the cache
    stats, err := pw.Prewarm(context.Background(), urls)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Prewarmed %d URLs: %d successful, %d failed\n",
        stats.Total, stats.Successful, stats.Failed)
}
```

### Concurrent Prewarming

For faster prewarming of many URLs, use concurrent prewarming:

```go
// Prewarm with 10 concurrent workers
stats, err := pw.PrewarmConcurrent(context.Background(), urls, 10)
if err != nil {
    log.Fatal(err)
}
```

### Sitemap-based Prewarming

Automatically extract URLs from XML sitemaps:

```go
// Prewarm from a sitemap (supports sitemap indexes)
stats, err := pw.PrewarmFromSitemap(context.Background(), "https://example.com/sitemap.xml", 5)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Prewarmed %d URLs from sitemap\n", stats.Total)
```

## Configuration Options

```go
config := prewarmer.Config{
    // Required: HTTP client with httpcache Transport
    Client: client,
    
    // Optional: Custom User-Agent header (default: "httpcache-prewarmer/1.0")
    UserAgent: "MyApp-Prewarmer/1.0",
    
    // Optional: Request timeout (default: 30 seconds)
    Timeout: 30 * time.Second,
    
    // Optional: Force refresh to bypass cache and fetch fresh content
    ForceRefresh: true,
}

pw, err := prewarmer.New(config)
if err != nil {
    log.Fatal(err)
}
```

## Progress Monitoring

Track prewarming progress with callbacks:

```go
callback := func(result *prewarmer.Result, current, total int) {
    status := "✓"
    if result.Error != nil {
        status = "✗"
    }
    fmt.Printf("[%d/%d] %s %s (%s)\n", 
        current, total, status, result.URL, result.Duration)
}

stats, err := pw.PrewarmWithCallback(context.Background(), urls, callback)
if err != nil {
    log.Fatal(err)
}
```

## Context Cancellation

Support graceful cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()

stats, err := pw.PrewarmConcurrent(ctx, urls, 10)

// Check if cancelled
if err != nil {
    fmt.Printf("Prewarming cancelled: %d/%d completed\n", 
        stats.Successful+stats.Failed, stats.Total)
}
```

## Result and Statistics

### Result Structure

Each URL returns a `Result`:

```go
type Result struct {
    URL        string        // The URL that was prewarmed
    Success    bool          // Whether the request succeeded
    StatusCode int           // HTTP status code
    Duration   time.Duration // Time taken for the request
    Size       int64         // Response body size in bytes
    Error      error         // Any error that occurred
    FromCache  bool          // Whether the response came from cache
}
```

### Stats Structure

Overall statistics:

```go
type Stats struct {
    Total         int           // Total URLs attempted
    Successful    int           // Number of successful requests
    Failed        int           // Number of failed requests
    FromCache     int           // Number of responses from cache
    TotalDuration time.Duration // Total time taken
    TotalBytes    int64         // Total bytes downloaded
    Errors        []error       // All errors encountered
}
```

## Best Practices

1. **Use concurrent prewarming** for large URL sets to improve speed
2. **Set appropriate timeouts** to prevent hanging on slow resources
3. **Monitor progress** for long-running prewarm operations
4. **Use sitemaps** when available for automatic URL discovery
5. **Schedule prewarming** during low-traffic periods
6. **Handle errors gracefully** - some URLs may fail and that's okay

## Integration with Cron/Schedulers

Example of scheduling prewarming:

```go
func warmCache() {
    cache := freecache.New(100 * 1024 * 1024)
    transport := httpcache.NewTransport(cache)
    client := transport.Client()

    pw, err := prewarmer.New(prewarmer.Config{
        Client:       client,
        Timeout:      30 * time.Second,
        ForceRefresh: true,
    })
    if err != nil {
        log.Printf("Failed to create prewarmer: %v", err)
        return
    }

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
    defer cancel()

    stats, err := pw.PrewarmFromSitemap(ctx, "https://example.com/sitemap.xml", 10)
    if err != nil {
        log.Printf("Prewarm error: %v", err)
        return
    }

    log.Printf("Cache warmed: %d/%d successful in %s",
        stats.Successful, stats.Total, stats.TotalDuration)
}
```

## Error Handling

The prewarmer handles errors gracefully and continues processing:

```go
stats, err := pw.Prewarm(ctx, urls)
if err != nil {
    log.Printf("Prewarm operation error: %v", err)
}

// Check individual errors
for _, e := range stats.Errors {
    log.Printf("URL error: %v", e)
}
```

## See Also

- [Example](../examples/prewarmer/main.go)
- [HTTP Caching Documentation](./how-it-works.md)
- [Cache Backends](./backends.md)
