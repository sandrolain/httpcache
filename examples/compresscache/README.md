# Compress Cache Example

This example demonstrates how to use the `compresscache` wrapper to automatically compress cached HTTP responses, reducing storage requirements and network bandwidth when using distributed cache backends.

## Features Demonstrated

- **Multiple compression algorithms**: Gzip, Brotli, and Snappy
- **Algorithm-specific configuration**: Customizable compression levels
- **Compression statistics**: Track compression ratio and space savings
- **Cross-algorithm compatibility**: Read data compressed with any algorithm
- **Performance comparison**: Benchmark different algorithms

## Running the Example

From the project root directory:

```bash
go run ./examples/compresscache/main.go
```

Or from the examples/compresscache directory:

```bash
go run main.go
```

## Compression Algorithms

### Gzip

**Best for**: Balanced compression and speed

- Good compression ratio (typically 60-70% reduction)
- Medium speed
- General purpose, widely supported
- Configurable compression level (-2 to 9)

### Brotli

**Best for**: Maximum compression ratio

- Excellent compression ratio (typically 70-85% reduction)
- Slower than Gzip
- Best when storage savings are priority
- Configurable compression level (0 to 11)

### Snappy

**Best for**: Maximum speed

- Moderate compression ratio (typically 40-60% reduction)
- Fastest compression/decompression
- Best for high-throughput scenarios
- No compression level (optimized for speed)

## Example Output

```text
=== Gzip Compression (Level: BestSpeed) ===
Original size: 15360 bytes
Compressed size: 5120 bytes
Compression ratio: 0.33
Space savings: 66.67%

=== Brotli Compression (Level: 6) ===
Original size: 15360 bytes
Compressed size: 4096 bytes
Compression ratio: 0.27
Space savings: 73.33%

=== Snappy Compression ===
Original size: 15360 bytes
Compressed size: 7680 bytes
Compression ratio: 0.50
Space savings: 50.00%
```

## Use Cases

### When to use Gzip

- General purpose HTTP caching
- JSON/XML API responses
- Text-based content (HTML, CSS, JavaScript)
- Balanced performance requirements

### When to use Brotli

- Maximum storage savings needed
- Slower-changing data (can afford compression time)
- Large text-based responses
- CDN edge caching
- Long-lived cache entries

### When to use Snappy

- High-throughput systems
- Real-time applications
- CPU-constrained environments
- Frequently accessed cache (hot data)
- Latency-sensitive operations

## Distributed Cache Scenarios

Compression is especially beneficial with distributed cache backends:

### Redis

Save memory and bandwidth:

```go
redisCache, _ := redis.New("localhost:6379")
cache, _ := compresscache.NewGzip(compresscache.GzipConfig{
    Cache: redisCache,
    Level: gzip.BestSpeed,
})
```

**Benefits**:

- Reduced network bandwidth to Redis
- Lower Redis memory usage
- Faster cache transfers
- Cost savings on Redis memory

### PostgreSQL

Reduce database storage:

```go
pgCache, _ := postgresql.New(postgresql.Config{
    ConnectionString: "postgres://...",
})
cache, _ := compresscache.NewBrotli(compresscache.BrotliConfig{
    Cache: pgCache,
    Level: 8, // High compression
})
```

**Benefits**:

- Smaller database size
- Faster backups
- Lower storage costs
- More efficient queries

## Performance Considerations

### CPU vs Storage Tradeoff

- **Snappy**: Low CPU overhead, moderate compression
- **Gzip**: Medium CPU overhead, good compression
- **Brotli**: High CPU overhead, excellent compression

### Recommended Settings

**High-throughput API** (prioritize speed):

```go
cache, _ := compresscache.NewSnappy(compresscache.SnappyConfig{
    Cache: baseCache,
})
```

**Storage-optimized** (prioritize space):

```go
cache, _ := compresscache.NewBrotli(compresscache.BrotliConfig{
    Cache: baseCache,
    Level: 8,
})
```

**Balanced** (general purpose):

```go
cache, _ := compresscache.NewGzip(compresscache.GzipConfig{
    Cache: baseCache,
    Level: gzip.BestSpeed,
})
```

## Multi-Tier Example

Combine with multicache for optimal performance:

```go
// Fast tier: local disk cache
localCache := diskcache.New("/tmp/cache")

// Slow tier: compressed Redis
redisCache, _ := redis.New("localhost:6379")
compressedRedis, _ := compresscache.NewGzip(compresscache.GzipConfig{
    Cache: redisCache,
    Level: gzip.BestSpeed,
})

// Combine tiers
cache := multicache.New(localCache, compressedRedis)
```

## Related Examples

- [Basic Example](../basic/) - Simple HTTP caching
- [Redis Example](../redis/) - Redis backend
- [MultiCache Example](../multicache/) - Multi-tier caching
- [Custom Backend](../custom-backend/) - Custom cache implementations

## Further Reading

- [CompressCache Documentation](../../wrapper/compresscache/README.md)
- [Cache Backends](../../docs/backends.md)
- [Advanced Features](../../docs/advanced-features.md)
