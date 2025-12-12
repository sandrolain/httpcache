# Stale Marking System

## Overview

The Stale Marking System is a resilience feature that improves cache behavior when backend servers are experiencing errors. Instead of immediately deleting cache entries when errors occur, entries are marked as "stale" and can potentially be served later if conditions permit.

## Motivation

Based on analysis of `lox/httpcache` (see `context/LOX_HTTPCACHE_ANALYSIS.md`), marking entries as stale rather than deleting them provides better resilience:

- **Better stale-if-error support**: Stale entries can be served when origin servers return errors
- **Improved availability**: Content remains available even during server outages
- **Graceful degradation**: Applications can continue serving cached content during issues

## How It Works

### Cache Interface Extensions

Three new methods have been added to the `Cache` interface:

```go
// MarkStale marks a cached response as stale instead of deleting it.
MarkStale(ctx context.Context, key string) error

// IsStale checks if a cached response has been marked as stale.
IsStale(ctx context.Context, key string) (bool, error)

// GetStale retrieves a stale cached response if it exists.
GetStale(ctx context.Context, key string) (responseBytes []byte, ok bool, err error)
```

### Configuration

Enable stale marking via the `WithEnableStaleMarking` option:

```go
cache := httpcache.NewMockCache()
transport := httpcache.NewTransport(cache, 
    httpcache.WithEnableStaleMarking(true),
)
```

**Default**: `false` (backward compatible - entries are deleted immediately)

### Behavior

#### When EnableStaleMarking = true

1. On cache invalidation (errors, non-200 responses): Entry is **marked as stale**
2. Entry remains in cache and can be retrieved via `GetStale()`
3. Supports better `stale-if-error` behavior per RFC 7234

#### When EnableStaleMarking = false (default)

1. On cache invalidation: Entry is **deleted immediately**
2. Traditional behavior - backward compatible
3. No stale entries remain in cache

## Backend Support

### Native Support

All backends now have full native support for stale marking:

#### Core Backends

- **mockCache** - Test implementation with in-memory stale tracking
- **diskcache** - Uses `stale_` prefix for marker files on disk
- **blobcache** - Uses separate blob keys for stale markers (S3/GCS/Azure)
- **redis** - Uses `stale:` prefixed keys with atomic operations
- **leveldbcache** - Uses `stale:` prefixed keys with batch operations
- **mongodb** - Uses `stale` boolean field in cache entry documents
- **postgresql** - Uses `stale` boolean column in cache table
- **natskv** - Uses `stale:` prefixed keys in NATS JetStream K/V store
- **freecache** - Uses `stale:` prefixed cache entries
- **hazelcast** - Uses `stale:` prefixed map keys
- **memcache** - Uses `stale:` prefixed keys (both standard and App Engine)

#### Wrapper Backends

- **compresscache** - All compression types (Gzip, Brotli, Snappy) support stale marking
- **multicache** - Full support across all tiers with automatic promotion
- **prometheus** - Metrics recording for all stale operations

### Using StaleAwareCache Wrapper (Optional)

For custom backends or special cases, you can still use the `StaleAwareCache` wrapper:

```go
customCache := &MyCustomCache{}
staleMarker := httpcache.NewMockCache() // In-memory stale tracking
cache := httpcache.NewStaleAwareCache(customCache, staleMarker)

transport := httpcache.NewTransport(cache,
    httpcache.WithEnableStaleMarking(true),
)
```

This wrapper:

- Delegates main operations to the inner cache
- Tracks stale markers in a separate cache
- Provides full stale marking support without modifying the backend

To use these with stale marking, wrap them with `StaleAwareCache`.

## Implementation Details

### Stale Marking Flow

1. **Cache Entry Creation**

   ```
   Set() → Clears any existing stale marker
   ```

2. **Error/Invalidation**

   ```
   If EnableStaleMarking:
       MarkStale() → Entry kept, marked as stale
   Else:
       Delete() → Entry removed immediately
   ```

3. **Stale Retrieval**

   ```
   GetStale() → Returns data only if marked as stale
   IsStale() → Checks stale status
   ```

### Storage Strategies

Different backends use different strategies for tracking stale entries:

| Backend | Strategy | Details |
|---------|----------|---------|
| mockCache | In-memory map | `stales map[string]bool` |
| diskcache | Marker files | `stale_<key>` file presence |
| blobcache | Marker blobs | `stale_<key>` blob with "1" |
| redis | Separate keys | `rediscache:stale:<key>` |
| leveldbcache | Prefix keys | `stale:<key>` |
| mongodb | Document field | `stale: bool` in cacheEntry |

## Example Usage

### Basic Setup

```go
package main

import (
    "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/redis"
)

func main() {
    // Create cache backend
    cache, _ := redis.New(redis.Config{
        Address: "localhost:6379",
    })

    // Enable stale marking
    transport := httpcache.NewTransport(cache,
        httpcache.WithEnableStaleMarking(true),
        httpcache.WithMarkCachedResponses(true),
    )

    client := transport.Client()
    
    // Requests will benefit from stale marking
    // When server errors occur, stale content can be served
    resp, _ := client.Get("https://api.example.com/data")
    defer resp.Body.Close()
}
```

### With stale-if-error

```go
// Server response headers
Cache-Control: max-age=3600, stale-if-error=86400

// Behavior:
// 1. Response cached for 1 hour
// 2. After 1 hour, content is stale
// 3. If server returns error, stale content served for up to 24 hours
// 4. With EnableStaleMarking=true, entry remains available for serving
```

### Wrapping Existing Backend

```go
// Wrap a backend that doesn't have native stale support
innerCache := postgresql.New(config)
staleTracker := httpcache.NewMockCache()
cache := httpcache.NewStaleAwareCache(innerCache, staleTracker)

transport := httpcache.NewTransport(cache,
    httpcache.WithEnableStaleMarking(true),
)
```

## Testing

Tests are in `httpcache_stale_marking_test.go`:

```bash
# Run stale marking tests
go test -v -run TestStaleMarking

# Test specific backend
go test -v ./diskcache
go test -v ./redis
```

## Migration Guide

### From Previous Versions

**No breaking changes** - stale marking is opt-in:

1. Update httpcache: `go get github.com/sandrolain/httpcache@latest`
2. Optionally enable stale marking:

   ```go
   httpcache.WithEnableStaleMarking(true)
   ```

3. Existing code works unchanged (default: `false`)

### Enabling Stale Marking

```go
// Before (default behavior)
transport := httpcache.NewTransport(cache)

// After (with stale marking)
transport := httpcache.NewTransport(cache,
    httpcache.WithEnableStaleMarking(true),
)
```

## Performance Considerations

### Storage Overhead

- **In-memory backends**: Minimal overhead (one boolean per entry)
- **Disk backends**: Additional marker files/keys (small)
- **Database backends**: Additional field or key

### Operation Costs

- `MarkStale()`: Similar cost to `Delete()` (creates marker instead of removing)
- `IsStale()`: Single lookup operation
- `GetStale()`: Two lookups (stale check + data retrieval)

### Cleanup

Stale markers are cleaned up:

- On `Delete()`: Both entry and marker removed
- On `Set()`: Marker cleared when new data set
- Backend-specific TTL (if supported by backend)

## Best Practices

1. **Enable for Production APIs**: Improves availability during outages
2. **Combine with stale-if-error**: Set appropriate cache headers
3. **Monitor Stale Serving**: Track when stale content is served
4. **Backend Selection**: All backends have native support for optimal performance
5. **Testing**: Test failure scenarios to verify stale serving works

## Limitations

1. **Not a substitute for proper error handling**: Stale content is supplementary
2. **Storage growth**: Stale entries consume additional space for markers
3. **Opt-in feature**: Must explicitly enable with `WithEnableStaleMarking(true)`
4. **Marker overhead**: Each stale entry requires a small marker (separate key or field)

## Future Improvements

- [ ] Add TTL for stale markers
- [ ] Implement for all backends natively
- [ ] Add metrics for stale serving
- [ ] Automatic cleanup of old stale entries
- [ ] Configurable stale serving policies

## References

- RFC 7234 Section 4.2.4 (Serving Stale Responses)
- RFC 5861 (stale-if-error extension)
- `context/LOX_HTTPCACHE_ANALYSIS.md` (analysis and motivation)
- `httpcache_stale_marking_test.go` (tests and examples)
