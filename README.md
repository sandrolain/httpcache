httpcache
=========

[![CI](https://github.com/sandrolain/httpcache/workflows/CI/badge.svg)](https://github.com/sandrolain/httpcache/actions/workflows/ci.yml)
[![Security](https://github.com/sandrolain/httpcache/workflows/Security/badge.svg)](https://github.com/sandrolain/httpcache/actions/workflows/security.yml)
[![GoDoc](https://godoc.org/github.com/sandrolain/httpcache?status.svg)](https://godoc.org/github.com/sandrolain/httpcache)
[![Go Report Card](https://goreportcard.com/badge/github.com/sandrolain/httpcache)](https://goreportcard.com/report/github.com/sandrolain/httpcache)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE.txt)

**Package httpcache** provides an `http.RoundTripper` implementation that works as a mostly [RFC 7234](https://tools.ietf.org/html/rfc7234) compliant cache for HTTP responses.

> **Note**: This is a maintained fork of [gregjones/httpcache](https://github.com/gregjones/httpcache), which is no longer actively maintained. This fork aims to modernize the codebase while maintaining backward compatibility, fix bugs, and add new features.

## Features

- ✅ **RFC 7234 Compliant** - Implements HTTP caching standards
- ✅ **Multiple Backends** - Memory, Disk, Redis, LevelDB, Memcache
- ✅ **Thread-Safe** - Safe for concurrent use
- ✅ **Zero Dependencies** - Core package uses only Go standard library
- ✅ **Easy Integration** - Drop-in replacement for `http.Client`
- ✅ **ETag & Validation** - Automatic cache revalidation
- ✅ **Stale-If-Error** - Resilient caching with RFC 5861 support
- ✅ **Private Cache** - Suitable for web browsers and API clients

## Quick Start

```go
package main

import (
    "fmt"
    "io"
    "net/http"
    
    "github.com/sandrolain/httpcache"
)

func main() {
    // Create a cached HTTP client
    transport := httpcache.NewMemoryCacheTransport()
    client := transport.Client()
    
    // Make requests - second request will be cached!
    resp, _ := client.Get("https://example.com")
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    
    // Check if response came from cache
    if resp.Header.Get(httpcache.XFromCache) == "1" {
        fmt.Println("Response was cached!")
    }
}
```

## Installation

```bash
go get github.com/sandrolain/httpcache
```

## Cache Backends

httpcache supports multiple storage backends. Choose the one that fits your use case:

### Built-in Backends

| Backend | Speed | Persistence | Distributed | Use Case |
|---------|-------|-------------|-------------|----------|
| **Memory** | ⚡⚡⚡ Fastest | ❌ No | ❌ No | Development, testing, single-instance apps |
| **[Disk](./diskcache)** | ⚡ Slow | ✅ Yes | ❌ No | Desktop apps, CLI tools |
| **[LevelDB](./leveldbcache)** | ⚡⚡ Fast | ✅ Yes | ❌ No | High-performance local cache |
| **[Redis](./redis)** | ⚡⚡ Fast | ✅ Configurable | ✅ Yes | Microservices, distributed systems |
| **[Memcache](./memcache)** | ⚡⚡ Fast | ❌ No | ✅ Yes | Distributed systems, App Engine |

### Third-Party Backends

- [`sourcegraph.com/sourcegraph/s3cache`](https://sourcegraph.com/github.com/sourcegraph/s3cache) - Amazon S3 storage
- [`github.com/die-net/lrucache`](https://github.com/die-net/lrucache) - In-memory with LRU eviction
- [`github.com/die-net/lrucache/twotier`](https://github.com/die-net/lrucache/tree/master/twotier) - Multi-tier caching (e.g., memory + disk)
- [`github.com/birkelund/boltdbcache`](https://github.com/birkelund/boltdbcache) - BoltDB implementation

## Usage Examples

### Memory Cache (Default)

```go
transport := httpcache.NewMemoryCacheTransport()
client := transport.Client()
```

**Best for**: Testing, development, single-instance applications

### Disk Cache

```go
import "github.com/sandrolain/httpcache/diskcache"

cache := diskcache.New("/tmp/my-cache")
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: Desktop applications, CLI tools that run repeatedly

### Redis Cache

```go
import (
    "github.com/gomodule/redigo/redis"
    rediscache "github.com/sandrolain/httpcache/redis"
)

conn, _ := redis.Dial("tcp", "localhost:6379")
cache := rediscache.NewWithClient(conn)
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: Microservices, distributed systems, high availability

### LevelDB Cache

```go
import "github.com/sandrolain/httpcache/leveldbcache"

cache, _ := leveldbcache.New("/path/to/cache")
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: High-performance local caching with persistence

### Custom Transport Configuration

```go
// Use a custom underlying transport
transport := httpcache.NewTransport(cache)
transport.Transport = &http.Transport{
    MaxIdleConns:        100,
    IdleConnTimeout:     90 * time.Second,
    DisableCompression:  false,
}
transport.MarkCachedResponses = true // Add X-From-Cache header

client := &http.Client{
    Transport: transport,
    Timeout:   30 * time.Second,
}
```

## Practical Examples

See the [`examples/`](./examples) directory for complete, runnable examples:

- **[Basic](./examples/basic/)** - Simple in-memory caching
- **[Disk Cache](./examples/diskcache/)** - Persistent filesystem cache
- **[Redis](./examples/redis/)** - Distributed caching with Redis
- **[LevelDB](./examples/leveldb/)** - High-performance persistent cache
- **[Custom Backend](./examples/custom-backend/)** - Build your own cache backend

Each example includes:

- Complete working code
- Detailed README
- Use case explanations
- Best practices

## How It Works

httpcache implements RFC 7234 (HTTP Caching) by:

1. **Intercepting HTTP requests** through a custom `RoundTripper`
2. **Checking cache** for matching responses
3. **Validating freshness** using Cache-Control headers
4. **Revalidating** with ETag/Last-Modified when stale
5. **Updating cache** with new responses

### Cache Headers Supported

- `Cache-Control` (max-age, no-cache, no-store, etc.)
- `ETag` and `If-None-Match`
- `Last-Modified` and `If-Modified-Since`
- `Expires`
- `Vary`
- `stale-if-error` (RFC 5861)

### Detecting Cache Hits

```go
resp, _ := client.Get(url)
if resp.Header.Get(httpcache.XFromCache) == "1" {
    // Response was served from cache
}
```

## Advanced Features

### Stale-If-Error Support

Automatically serve stale cached content when the backend is unavailable:

```go
// Server returns 500, but cached response is served instead
resp, _ := client.Get(url) // Returns cached response, not 500 error
```

This implements [RFC 5861](https://tools.ietf.org/html/rfc5861) for better resilience.

### Vary Header Support

Correctly handles content negotiation:

```go
req, _ := http.NewRequest("GET", url, nil)
req.Header.Set("Accept", "application/json")
resp, _ := client.Do(req)
// Cached separately from "Accept: text/html" requests
```

### Custom Cache Implementation

Implement the `Cache` interface for custom backends:

```go
type Cache interface {
    Get(key string) (responseBytes []byte, ok bool)
    Set(key string, responseBytes []byte)
    Delete(key string)
}
```

See [examples/custom-backend](./examples/custom-backend/) for a complete example.

## Limitations

- **Private cache only** - Not suitable for shared proxy caching
- **No automatic eviction** - MemoryCache grows unbounded (use size-limited backends)
- **GET/HEAD only** - Only caches GET and HEAD requests
- **No range requests** - Range requests bypass the cache

## Performance

Typical performance characteristics:

| Operation | Memory | Disk | LevelDB | Redis (local) |
|-----------|--------|------|---------|---------------|
| Cache Hit | ~1µs | ~1ms | ~100µs | ~1ms |
| Cache Miss | Network latency + ~1µs overhead ||||
| Storage | RAM | Disk | Disk (compressed) | RAM/Disk |

*Benchmarks vary based on response size, hardware, and network conditions.*

## Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run benchmarks
go test -bench=. ./...
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes with tests
4. Submit a pull request

## Documentation

- [GoDoc](https://godoc.org/github.com/sandrolain/httpcache) - API documentation
- [Examples](./examples/) - Practical usage examples

## Acknowledgments

This project is a maintained fork of [gregjones/httpcache](https://github.com/gregjones/httpcache), originally created by [@gregjones](https://github.com/gregjones). The original project was archived in 2023.

We're grateful for the original work and continue to maintain this project with:

- Bug fixes and security updates
- Modern Go practices and tooling
- Enhanced documentation and examples
- Backward compatibility with the original

## License

[MIT License](LICENSE.txt)

Copyright (c) 2012 Greg Jones (original)  
Copyright (c) 2025 Sandro Lain (fork maintainer)

## Support

- 📖 [Documentation](https://godoc.org/github.com/sandrolain/httpcache)
- 💬 [Issues](https://github.com/sandrolain/httpcache/issues)
- 🔧 [Examples](./examples/)
