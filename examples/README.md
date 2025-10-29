# httpcache Examples

This directory contains practical examples demonstrating different ways to use httpcache.

## Available Examples

### 1. [Basic](./basic/)

The simplest example using in-memory caching. Great for getting started.

**Features:**

- In-memory cache setup
- Basic GET requests
- Cache hit detection
- ETag validation

**When to use:**

- Quick prototyping
- Testing
- Single-instance applications
- When persistence is not needed

### 2. [Disk Cache](./diskcache/)

Persistent caching using filesystem storage.

**Features:**

- Persistent storage
- Survives application restarts
- Multiple clients sharing cache
- Cache directory management

**When to use:**

- Desktop applications
- CLI tools
- When you need persistence
- Single-machine deployments

### 3. [Redis Cache](./redis/)

Distributed caching using Redis.

**Features:**

- Distributed cache
- Connection pooling
- Multiple instances sharing cache
- Production-ready setup

**When to use:**

- Microservices
- Distributed systems
- High availability requirements
- When you need cache sharing across instances

### 4. [LevelDB Cache](./leveldb/)

High-performance persistent cache.

**Features:**

- Fast persistent storage
- Embedded database
- No external dependencies
- Compact storage

**When to use:**

- High-performance requirements
- Embedded applications
- When disk cache is too slow
- When Redis is overkill

### 5. [Freecache](./freecache/)

High-performance, zero-GC overhead caching for large-scale applications.

**Features:**

- Zero GC overhead
- Automatic LRU eviction
- Millions of entries support
- Built-in statistics

**When to use:**

- Caching millions of responses
- Performance-critical applications
- When GC is a bottleneck
- High-concurrency environments

### 6. [Custom Backend](./custom-backend/)

Learn how to create custom cache backends.

**Features:**

- Statistics tracking
- TTL-based expiration
- Decorator pattern examples
- Custom implementations

**When to use:**

- Learning how to extend httpcache
- Need custom functionality
- Building specialized caching strategies
- Adding monitoring/metrics

### 6. [Cache Key Headers](./cachekeyheaders/)

Differentiate cache entries based on request header values.

**Features:**

- Per-user caching with Authorization headers
- Multi-language support with Accept-Language
- Multiple header combinations
- Header-based cache isolation

**When to use:**

- Multi-tenant applications
- User-specific API responses
- Internationalized content
- API versioning by header
- Any scenario requiring cache separation by request headers

### 7. [NATS K/V Cache](./natskv/)

Distributed caching using NATS JetStream Key/Value store.

**Features:**

- Distributed cache with NATS
- JetStream persistence
- Multiple instances sharing cache
- Built-in TTL support
- NATS clustering support

**When to use:**

- Already using NATS in your infrastructure
- Need distributed caching with messaging
- Microservices with NATS communication
- When you want NATS' simplicity over Redis

### 8. [Hazelcast Cache](./hazelcast/)

Distributed caching using Hazelcast in-memory data grid.

**Features:**

- Distributed in-memory cache
- Automatic data distribution across cluster
- High availability with replication
- Scalable architecture
- Enterprise-grade performance

**When to use:**

- Already using Hazelcast in your infrastructure
- Need high-performance distributed caching
- Enterprise applications requiring HA
- When you need automatic data partitioning

## Running Examples

Each example has its own directory with:

- `main.go` - Runnable example code
- `README.md` - Detailed documentation

All examples use the main project's go.mod. To run an example from the project root:

```bash
go run ./examples/<example-name>/main.go
```

Or navigate to the example directory and run:

```bash
cd examples/<example-name>
go run main.go
```

## Quick Comparison

| Backend | Speed | Persistence | Distributed | Setup Complexity | Best For |
|---------|-------|-------------|-------------|------------------|-----|
| Memory | ⚡⚡⚡ | ❌ | ❌ | ⭐ | < 100k entries |
| Freecache | ⚡⚡⚡ | ❌ | ❌ | ⭐ | Millions of entries, zero GC |
| Disk | ⚡ | ✅ | ❌ | ⭐ | Persistence needed |
| LevelDB | ⚡⚡ | ✅ | ❌ | ⭐⭐ | Fast + persistent |
| Redis | ⚡⚡ | ✅* | ✅ | ⭐⭐⭐ | Distributed systems |
| Memcache | ⚡⚡ | ❌ | ✅ | ⭐⭐⭐ | Distributed, no persistence |
| NATS K/V | ⚡⚡ | ✅* | ✅ | ⭐⭐⭐ | NATS users |
| Hazelcast | ⚡⚡⚡ | ✅* | ✅ | ⭐⭐⭐ | Enterprise, HA |

*Redis, NATS K/V, and Hazelcast persistence depends on configuration

## Common Patterns

### Basic Setup

```go
transport := httpcache.NewMemoryCacheTransport()
client := transport.Client()
```

### Custom Cache Backend

```go
cache := customcache.New()
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

### Detecting Cache Hits

```go
resp, _ := client.Get(url)
if resp.Header.Get(httpcache.XFromCache) == "1" {
    // Response came from cache
}
```

### Custom Underlying Transport

```go
customTransport := &http.Transport{
    MaxIdleConns: 100,
    // ... other settings
}
transport := httpcache.NewTransport(cache)
transport.Transport = customTransport
```

## Best Practices

1. **Choose the right backend** for your use case
2. **Use connection pooling** with Redis/Memcache
3. **Monitor cache hit rates** to validate effectiveness
4. **Set appropriate timeouts** on the HTTP client
5. **Handle errors gracefully** from cache operations
6. **Consider cache size limits** to prevent memory issues
7. **Use persistent cache** for expensive or slow APIs

## Testing Your Cache

All examples include verification that the cache is working:

```go
// First request - cache miss
resp1, _ := client.Get(url)
fmt.Printf("From cache: %s\n", resp1.Header.Get(httpcache.XFromCache))
// Output: From cache: 

// Second request - cache hit
resp2, _ := client.Get(url)
fmt.Printf("From cache: %s\n", resp2.Header.Get(httpcache.XFromCache))
// Output: From cache: 1
```

## Contributing

Found a useful pattern or use case? Feel free to contribute additional examples!

1. Create a new directory under `examples/`
2. Include `main.go`, `go.mod`, and `README.md`
3. Make sure the example is runnable and well-documented
4. Update this README with a link to your example

## Need Help?

- Check the [main README](../README.md) for general information
- See the [GoDoc](https://godoc.org/github.com/sandrolain/httpcache) for API documentation
