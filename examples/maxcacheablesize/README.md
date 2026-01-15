# MaxCacheableResponseSize Example

This example demonstrates how to use the `MaxCacheableResponseSize` feature to prevent memory exhaustion from large HTTP responses.

## What it does

The example shows three scenarios:

1. **Small API response** (~5KB) - Will be cached normally
2. **Large file download** (1GB) - Will be streamed without caching
3. **Repeated small request** - Served from cache

## Configuration

```go
transport := httpcache.NewTransport(
    cache,
    httpcache.WithMaxCacheableResponseSize(5*1024*1024), // 5MB limit
    httpcache.WithMarkCachedResponses(true),
)
```

## Why is this important?

Without `MaxCacheableResponseSize`, the cache would try to buffer the entire 1GB file in memory before deciding whether to cache it. This could lead to:

- Out of memory errors
- Excessive memory usage
- Poor performance

With the limit set to 5MB, large files are automatically streamed without buffering, preventing these issues.

## Running the example

```bash
go run main.go
```

## Expected output

```
Fetching small API response...
Small response: 5234 bytes, Cached: 

Fetching large file...
Large file: Read 1024 bytes (streaming, not cached)

Fetching small API response again...
Small response: 5234 bytes, Cached: 1

✅ Memory leak prevention working correctly!
Small responses are cached, large ones are streamed.
```

## Recommendations

- **Default**: 10MB (suitable for most API responses)
- **APIs only**: 5-20MB
- **With large files**: Keep default or increase based on requirements
- **Production**: Always set a reasonable limit
