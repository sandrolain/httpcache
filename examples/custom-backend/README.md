# Custom Cache Backend Example

This example demonstrates how to create custom cache backends by implementing the `httpcache.Cache` interface.

## Features Demonstrated

- **StatsCache**: Wrapper that adds statistics tracking (hits, misses, sets, deletes)
- **TTLCache**: Cache with automatic expiration based on time-to-live
- Decorator pattern for extending cache functionality
- Background cleanup for expired entries

## Running the Example

From the project root directory:

```bash
go run ./examples/custom-backend/main.go
```

Or from the examples/custom-backend directory:

```bash
go run main.go
```

## Cache Interface

To create a custom cache backend, implement this interface:

```go
type Cache interface {
    Get(key string) (responseBytes []byte, ok bool)
    Set(key string, responseBytes []byte)
    Delete(key string)
}
```

## Use Cases

Custom cache backends are useful for:

- **Monitoring**: Track cache hits/misses and performance
- **Size limits**: Implement LRU or LFU eviction policies
- **TTL support**: Automatic expiration independent of HTTP headers
- **Compression**: See [CompressCache example](../compresscache/) for production-ready compression
- **Encryption**: Secure sensitive cached data
- **Multi-tier**: Combine multiple cache backends (e.g., memory + disk)
- **Database**: Store cache in PostgreSQL, MySQL, etc.

## Pattern: Cache Wrapper

The decorator pattern allows you to add functionality without modifying the original cache:

```go
type WrappedCache struct {
    underlying httpcache.Cache
    // ... additional fields
}

func (c *WrappedCache) Get(key string) ([]byte, bool) {
    // Add custom logic before/after
    return c.underlying.Get(key)
}
```

## Example Implementations

### 1. Statistics Cache

Tracks performance metrics:

- Cache hits and misses
- Hit rate calculation
- Set and delete operations
- Thread-safe counters

### 2. TTL Cache

Automatic expiration:

- Time-based expiration
- Background cleanup goroutine
- Independent of HTTP cache headers
- Configurable TTL duration

## Ideas for Other Custom Caches

- **Size-limited cache**: LRU eviction when size limit reached
- **Compressed cache**: See [CompressCache](../compresscache/) for production implementation
- **Encrypted cache**: Encrypt sensitive data at rest
- **Logging cache**: Log all cache operations for debugging
- **Circuit breaker cache**: Fail fast on repeated errors
- **Multi-level cache**: L1 (memory) + L2 (disk/redis)
- **Sharded cache**: Distribute across multiple backends
