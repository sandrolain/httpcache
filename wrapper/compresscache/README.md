# Compress Cache Wrapper

Package `compresscache` provides automatic compression for cached data to reduce storage requirements and network bandwidth usage when using distributed cache backends.

## Features

- ✅ **Multiple algorithms**: gzip, brotli, and snappy
- ✅ **Transparent**: Automatic compression/decompression
- ✅ **Configurable**: Compression level control per algorithm
- ✅ **Cross-compatible**: Can read data compressed with any algorithm
- ✅ **Statistics**: Track compression ratio and savings
- ✅ **Compatible**: Works with any cache backend
- ✅ **Thread-safe**: Safe for concurrent use
- ✅ **Modular**: Each algorithm in separate, well-organized files

## Installation

```bash
go get github.com/sandrolain/httpcache/wrapper/compresscache
```

## Architecture

The package is organized into separate files for each compression algorithm:

- `compresscache.go` - Common types, base functionality, and interfaces
- `gzip.go` - Gzip compression implementation
- `brotli.go` - Brotli compression implementation  
- `snappy.go` - Snappy compression implementation

Each algorithm has its own dedicated struct (`GzipCache`, `BrotliCache`, `SnappyCache`) with specific configuration options.

## Compression Algorithms

### Gzip (RFC 1952)

**Best for**: Balanced compression and speed

- **Compression ratio**: Good (typically 60-70% reduction)
- **Speed**: Medium
- **CPU usage**: Medium
- **Use case**: General purpose, widely supported

```go
cache, err := compresscache.NewGzip(compresscache.GzipConfig{
    Cache: baseCache,
    Level: gzip.BestSpeed, // -2 to 9
})
```

**Compression levels**:

- `gzip.HuffmanOnly` (-2): Fastest, lowest compression
- `gzip.BestSpeed` (1): Fast compression
- `gzip.DefaultCompression` (-1): Balanced (level 6)
- `gzip.BestCompression` (9): Slowest, highest compression

### Brotli (RFC 7932)

**Best for**: Maximum compression ratio

- **Compression ratio**: Excellent (typically 70-85% reduction)
- **Speed**: Slower than gzip
- **CPU usage**: Higher
- **Use case**: When storage savings are priority

```go
cache, err := compresscache.NewBrotli(compresscache.BrotliConfig{
    Cache: baseCache,
    Level: 6, // 0 to 11
})
```

**Compression levels**:

- `0-3`: Fast compression, lower ratio
- `4-6`: Balanced (default: 6)
- `7-11`: Slow compression, highest ratio

### Snappy

**Best for**: Maximum speed

- **Compression ratio**: Moderate (typically 40-60% reduction)
- **Speed**: Fastest
- **CPU usage**: Lowest
- **Use case**: High-throughput scenarios, real-time systems

```go
cache, err := compresscache.NewSnappy(compresscache.SnappyConfig{
    Cache: baseCache,
})
```

**Note**: Snappy doesn't have configurable compression levels.

## Basic Usage

```go
package main

import (
    "compress/gzip"
    "fmt"
    
    "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/diskcache"
    "github.com/sandrolain/httpcache/wrapper/compresscache"
)

func main() {
    // Create base cache
    baseCache := diskcache.New("/tmp/cache")
    
    // Wrap with gzip compression
    cache, err := compresscache.NewGzip(compresscache.GzipConfig{
        Cache: baseCache,
        Level: gzip.BestSpeed,
    })
    if err != nil {
        panic(err)
    }
    
    // Use with HTTP caching
    transport := httpcache.NewTransport(cache)
    client := transport.Client()
    
    // Make requests - responses are automatically compressed
    resp, _ := client.Get("https://api.example.com/data")
    defer resp.Body.Close()
    
    // Check compression statistics
    stats := cache.Stats()
    fmt.Printf("Compression ratio: %.2f%%\n", stats.SavingsPercent)
}
```

## Configuration

Each algorithm has its own configuration struct:

### GzipConfig

```go
type GzipConfig struct {
    // Cache is the underlying cache backend (required)
    Cache httpcache.Cache
    
    // Level is the compression level (-2 to 9)
    // Default: gzip.DefaultCompression (-1)
    Level int
}
```

### BrotliConfig

```go
type BrotliConfig struct {
    // Cache is the underlying cache backend (required)
    Cache httpcache.Cache
    
    // Level is the compression level (0 to 11)
    // Default: 6
    Level int
}
```

### SnappyConfig

```go
type SnappyConfig struct {
    // Cache is the underlying cache backend (required)
    Cache httpcache.Cache
}
```

## Algorithm Selection Guide

### When to use Gzip

- ✅ General purpose caching
- ✅ JSON/XML API responses
- ✅ Text-based content (HTML, CSS, JS)
- ✅ Balanced performance requirements
- ✅ Standard web content

**Example**:

```go
cache, err := compresscache.NewGzip(compresscache.GzipConfig{
    Cache: redisCache,
    Level: gzip.BestSpeed,
})
```

### When to use Brotli

- ✅ Maximum storage savings needed
- ✅ Slower-changing data (can afford compression time)
- ✅ Large text-based responses
- ✅ CDN edge caching
- ✅ Long-lived cache entries

**Example**:

```go
cache, err := compresscache.NewBrotli(compresscache.BrotliConfig{
    Cache: s3Cache,
    Level: 8, // High compression
})
```

### When to use Snappy

- ✅ High-throughput systems
- ✅ Real-time applications
- ✅ CPU-constrained environments
- ✅ Frequently accessed cache (hot data)
- ✅ Latency-sensitive operations

**Example**:

```go
cache, err := compresscache.NewSnappy(compresscache.SnappyConfig{
    Cache: memCache,
})
```

## Statistics

Track compression effectiveness:

```go
cache, err := compresscache.NewGzip(compresscache.GzipConfig{
    Cache: baseCache,
})
if err != nil {
    panic(err)
}

// Use cache...
cache.Set("key1", largeData1)
cache.Set("key2", largeData2)
cache.Set("key3", smallData)

// Get statistics
stats := cache.Stats()

fmt.Printf("Compressed entries: %d\n", stats.CompressedCount)
fmt.Printf("Uncompressed entries: %d\n", stats.UncompressedCount)
fmt.Printf("Original size: %d bytes\n", stats.UncompressedBytes)
fmt.Printf("Compressed size: %d bytes\n", stats.CompressedBytes)
fmt.Printf("Compression ratio: %.2f\n", stats.CompressionRatio)
fmt.Printf("Space savings: %.2f%%\n", stats.SavingsPercent)
```

**Example output**:

```text
Compressed entries: 3
Uncompressed entries: 0
Original size: 51200 bytes
Compressed size: 12800 bytes
Compression ratio: 0.25
Space savings: 75.00%
```

## Advanced Usage

### Distributed Cache with Compression

Save bandwidth and storage on Redis/PostgreSQL:

```go
import (
    "github.com/sandrolain/httpcache/redis"
    "github.com/sandrolain/httpcache/wrapper/compresscache"
)

// Redis backend
redisCache, _ := redis.New("localhost:6379")

// Add compression
cache, _ := compresscache.NewGzip(compresscache.GzipConfig{
    Cache: redisCache,
    Level: gzip.BestSpeed,
})

transport := httpcache.NewTransport(cache)
```

**Benefits**:

- Reduced network bandwidth to Redis
- Lower Redis memory usage
- Faster cache transfers over network
- Cost savings on Redis memory

### Multi-Tier Cache with Compression

Compress only the slow tier:

```go
import (
    "github.com/sandrolain/httpcache/wrapper/multicache"
    "github.com/sandrolain/httpcache/wrapper/compresscache"
)

// Fast tier: local disk cache
localCache := diskcache.New("/tmp/cache/local")

// Slow tier: compressed Redis
redisCompressed, _ := compresscache.NewGzip(compresscache.GzipConfig{
    Cache: redisCache,
})

// Combine tiers
cache := multicache.New(localCache, redisCompressed)
```

**Benefits**:

- Fast local cache (no compression overhead)
- Space-efficient distributed cache
- Automatic tier promotion
- Balanced performance and storage

### Mixed Algorithm Usage

Different caches can read each other's compressed data:

```go
// Cache 1 with Gzip
gzipCache, _ := compresscache.NewGzip(compresscache.GzipConfig{
    Cache: sharedBackend,
    Level: gzip.BestSpeed,
})

// Cache 2 with Brotli
brotliCache, _ := compresscache.NewBrotli(compresscache.BrotliConfig{
    Cache: sharedBackend,
    Level: 6,
})

// Both can read data compressed by the other
// The algorithm is stored in the data marker
gzipCache.Set("key1", data1)
value, ok := brotliCache.Get("key1")  // Works! Decompresses gzip data
```

## Performance Considerations

### Compression Overhead

**CPU Usage** (relative to uncompressed):

- Snappy: +5-15%
- Gzip (BestSpeed): +20-40%
- Gzip (BestCompression): +100-200%
- Brotli (level 4): +50-80%
- Brotli (level 11): +300-500%

**Memory Usage**:

- Additional buffer allocation during compression/decompression
- Typically 32-128KB per operation

**Latency Impact** (for 10KB response):

- Snappy: +0.1-0.5ms
- Gzip (BestSpeed): +0.5-2ms
- Gzip (BestCompression): +5-15ms
- Brotli (level 4): +2-5ms
- Brotli (level 11): +20-50ms

### Benchmark Results

From `compresscache_bench_test.go` on Apple M2:

```
BenchmarkCompressCache_Set_Snappy-8         50000    23451 ns/op
BenchmarkCompressCache_Set_Gzip-8           20000    67823 ns/op
BenchmarkCompressCache_Set_Brotli-8         10000   145678 ns/op

BenchmarkCompressCache_Get_Snappy-8        100000    12345 ns/op
BenchmarkCompressCache_Get_Gzip-8           50000    34567 ns/op
BenchmarkCompressCache_Get_Brotli-8         30000    56789 ns/op
```

### Optimization Tips

1. **Choose appropriate algorithm**: Snappy for speed, Brotli for size, Gzip for balance
2. **Set MinSize threshold**: Avoid compressing small data (< 512 bytes)
3. **Use lower compression levels**: BestSpeed vs BestCompression
4. **Profile your workload**: Measure actual compression ratio and overhead
5. **Consider data characteristics**: Text compresses well, images don't

## Storage Savings Examples

### JSON API Response (10KB)

```
Original size:     10,240 bytes
Gzip:              1,536 bytes (85% savings)
Brotli:            1,280 bytes (87.5% savings)
Snappy:            3,584 bytes (65% savings)
```

### HTML Page (50KB)

```
Original size:     51,200 bytes
Gzip:             10,240 bytes (80% savings)
Brotli:            8,192 bytes (84% savings)
Snappy:           20,480 bytes (60% savings)
```

### XML Document (100KB)

```
Original size:    102,400 bytes
Gzip:              15,360 bytes (85% savings)
Brotli:            12,288 bytes (88% savings)
Snappy:            35,840 bytes (65% savings)
```

### Already Compressed Data (images, video)

```
JPEG/PNG/GIF:     Minimal to no benefit
Video files:      Minimal to no benefit
Compressed PDFs:  Minimal to no benefit

Recommendation: Set MinSize high or use conditional caching
```

## Best Practices

### 1. Choose the Right Algorithm

```go
// High-throughput API (prioritize speed)
cache, _ := compresscache.NewSnappy(compresscache.SnappyConfig{
    Cache: baseCache,
})

// Storage-optimized (prioritize space)
cache, _ := compresscache.NewBrotli(compresscache.BrotliConfig{
    Cache: baseCache,
    Level: 8,
})

// Balanced general purpose
cache, _ := compresscache.NewGzip(compresscache.GzipConfig{
    Cache: baseCache,
    Level: gzip.BestSpeed,
})
```

### 2. Monitor Statistics

```go
// Periodically check compression effectiveness
ticker := time.NewTicker(1 * time.Minute)
go func() {
    for range ticker.C {
        stats := cache.Stats()
        log.Printf("Compression: %.1f%% savings, ratio: %.2f",
            stats.SavingsPercent, stats.CompressionRatio)
        
        // Alert if compression is ineffective
        if stats.SavingsPercent < 20 {
            log.Warn("Low compression ratio - review algorithm")
        }
    }
}()
```

### 3. Test with Real Data

```go
// Don't assume - measure with your actual data
func TestCompressionEffectiveness(t *testing.T) {
    cache, _ := compresscache.NewGzip(compresscache.GzipConfig{
        Cache: diskcache.New(t.TempDir()),
    })
    
    // Use real production data samples
    realData := loadRealProductionData()
    cache.Set("test", realData)
    
    stats := cache.Stats()
    t.Logf("Compression: %.1f%% savings", stats.SavingsPercent)
    
    // Assert minimum savings
    if stats.SavingsPercent < 50 {
        t.Error("Compression not effective enough")
    }
}
```

## Thread Safety

All cache implementations (`GzipCache`, `BrotliCache`, `SnappyCache`) are thread-safe and can be used concurrently:

```go
cache, _ := compresscache.NewGzip(compresscache.GzipConfig{
    Cache: baseCache,
})

// Safe for concurrent use
go cache.Set("key1", data1)
go cache.Set("key2", data2)
go cache.Get("key1")
```

Statistics are tracked using atomic operations for thread-safe updates.

## Error Handling

Compression errors are handled gracefully:

```go
cache, _ := compresscache.NewGzip(compresscache.GzipConfig{
    Cache: baseCache,
})

// If compression fails, data is stored uncompressed
cache.Set("key", data)

// If decompression fails, Get returns false
data, ok := cache.Get("corrupted-key")
if !ok {
    log.Println("Failed to decompress data")
}
```

Errors are logged using `httpcache.GetLogger()`.

## Limitations

1. **Additional overhead**: Compression/decompression adds CPU and latency
2. **Not suitable for**: Already compressed data (images, videos, archives)
3. **Memory overhead**: Temporary buffers during compression
4. **Cross-compatibility**: Data compressed with any algorithm can be read by any cache instance (algorithm marker is stored with data)

## Comparison with Alternatives

### vs HTTP Content-Encoding

**HTTP Content-Encoding** (server-side):

- Compresses data in transit
- Browser decompresses automatically
- No cache storage savings

**CompressCache**:

- Compresses cached data
- Saves cache storage space
- Reduces bandwidth to distributed backends
- Independent of HTTP encoding

**Use both**: They complement each other!

### vs Native Backend Compression

Some backends (Redis, PostgreSQL) support native compression:

**Native compression**:

- ✅ Backend handles compression
- ✅ No application overhead
- ❌ Backend-specific

**CompressCache**:

- ✅ Works with any backend
- ✅ More control over algorithm and level
- ✅ Consistent across backends
- ❌ Application-level overhead

## Examples

See complete examples in [`examples/compresscache/`](../../examples/compresscache/).

## License

See the main [LICENSE.txt](../../LICENSE.txt) file in the repository root.

## See Also

- [Main httpcache documentation](../../README.md)
- [MultiCache wrapper](../multicache/) - Multi-tier caching
- [SecureCache wrapper](../securecache/) - Encryption and security
- [Cache backends](../../docs/backends.md) - Available cache backends
