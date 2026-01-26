<center><img src="httpcache.png" width="320" alt="httpcache" /></center>

# httpcache

[![CI](https://github.com/sandrolain/httpcache/workflows/CI/badge.svg)](https://github.com/sandrolain/httpcache/actions/workflows/ci.yml)
[![Security](https://github.com/sandrolain/httpcache/workflows/Security/badge.svg)](https://github.com/sandrolain/httpcache/actions/workflows/security.yml)
[![Coverage](https://img.shields.io/badge/coverage-95%25-brightgreen.svg)](https://github.com/sandrolain/httpcache)
[![Go Report Card](https://goreportcard.com/badge/github.com/sandrolain/httpcache)](https://goreportcard.com/report/github.com/sandrolain/httpcache)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE.txt)
[![GoDoc](https://godoc.org/github.com/sandrolain/httpcache?status.svg)](https://godoc.org/github.com/sandrolain/httpcache)

**Package httpcache** provides an `http.RoundTripper` implementation that works as a mostly [RFC 9111](https://www.rfc-editor.org/rfc/rfc9111.html) (HTTP Caching) compliant cache for HTTP responses. It improves application performance by reducing redundant HTTP requests and supports various backends for use cases such as API caching, web scraping, and microservices.

> **RFC Compliance**: This implementation follows RFC 9111 (2022), which obsoletes RFC 7234 (2014). See the [compliance features](#features) for details on supported directives and behaviors.

> **Note**: This is a maintained fork of [gregjones/httpcache](https://github.com/gregjones/httpcache), which is no longer actively maintained. This fork aims to modernize the codebase while maintaining backward compatibility, fix bugs, and add new features.

## Use Cases

- **API Caching**: Reduce latency and server load by caching API responses.
- **Web Scraping**: Avoid repeated requests to the same endpoints.
- **Microservices**: Cache responses between services for better performance.
- **Web Applications**: Improve user experience by caching dynamic content.
- **Resource Caching**: Store static or frequently accessed resources locally.

## Table of Contents

- [httpcache](#httpcache)
  - [Use Cases](#use-cases)
  - [Table of Contents](#table-of-contents)
  - [Features](#features)
  - [Thread-Safety](#thread-safety)
  - [Quick Start](#quick-start)
    - [With Encryption (Optional)](#with-encryption-optional)
    - [Transport Options](#transport-options)
  - [Documentation](#documentation)
    - [📚 Core Documentation](#-core-documentation)
    - [🔍 Quick Navigation](#-quick-navigation)
  - [Practical Examples](#practical-examples)
  - [Limitations](#limitations)
    - [Private Directive for Public Caches](#private-directive-for-public-caches)
    - [Authorization Header in Shared Caches](#authorization-header-in-shared-caches)
  - [Performance](#performance)
  - [Testing](#testing)
  - [Contributing](#contributing)
  - [Acknowledgments](#acknowledgments)
  - [License](#license)
  - [Support](#support)

## Features

- ✅ **RFC 9111 Compliant** (~95% compliance) - Implements HTTP Caching standard (obsoletes RFC 7234)
  - ✅ Age header calculation with full Section 4.2.3 algorithm (request_time, response_time, response_delay tracking)
  - ✅ Age header validation per Section 5.1 (handles multiple values, invalid values with logging)
  - ✅ Cache-Control directive validation per Section 4.2.1 (duplicate detection, conflict resolution, value validation)
  - ✅ Warning headers for stale responses (Section 5.5 - deprecated but supported for compatibility)
  - ✅ must-revalidate directive enforcement (Section 5.2.2.1)
  - ✅ Pragma: no-cache support (Section 5.4 - HTTP/1.0 backward compatibility)
  - ✅ Cache invalidation on unsafe methods (Section 4.4)
  - ✅ Content-Location and Location header invalidation (Section 4.4)
  - ✅ Same-origin policy enforcement for cache invalidation
  - ✅ Cache-Control: private directive support (Section 5.2.2.6)
  - ✅ Cache-Control: must-understand directive support (Section 5.2.2.3)
  - ✅ Vary header matching per Section 4.1 (wildcard, whitespace normalization, case-insensitive)
  - ✅ Vary header separation - Optional separate cache entries for response variants (Section 4.1)
  - ✅ Authorization header handling per Section 3.5 (secure caching in shared caches)
- ✅ **Multiple Backends** - Memory, Disk, Redis, LevelDB, Memcache, PostgreSQL, MongoDB, NATS K/V, Hazelcast, Cloud Storage (S3/GCS/Azure)
- ✅ **Multi-Tier Caching** - Combine multiple backends with automatic fallback and promotion
- ✅ **Compression Wrapper** - Automatic Gzip, Brotli, or Snappy compression for cached data
- ✅ **Resilience Features** - Retry policies and circuit breakers using [failsafe-go](https://failsafe-go.dev/)
- ✅ **Built-in Security** - Key hashing (SHA-256 or xxHash) and optional AES-256-GCM encryption with fixed or random salts
- ✅ **High-Performance Hashing** - Configurable hash algorithms: SHA-256 (default, cryptographically secure) or xxHash (~2.7x faster for high-throughput)
- ✅ **Enhanced Encryption** - Random salt mode for improved security (NIST/OWASP compliant) or fixed salt mode for backward compatibility
- ✅ **Internal Metrics** - Zero-dependency metrics collection with optional Prometheus export (hit rate, latency histogram, errors, stale served, deduplication)
- ✅ **Options Pattern** - Clean configuration via `TransportOption` functions (`WithEncryption`, `WithRandomSaltEncryption`, `WithPublicCache`, `WithMetrics`, etc.)
- ✅ **Thread-Safe** - Safe for concurrent use
- ✅ **Zero Dependencies** - Core package uses only Go standard library
- ✅ **Easy Integration** - Drop-in replacement for `http.Client`
- ✅ **ETag & Validation** - Automatic cache revalidation
- ✅ **Stale-If-Error** - Resilient caching with RFC 5861 support
- ✅ **Stale-While-Revalidate** - Async cache updates for better performance (RFC 5861)
- ✅ **Configurable Cache Mode** - Use as private cache (default) or shared/public cache
- ✅ **Request Deduplication** - Coalesce concurrent requests to the same resource (optional `EnableDeduplication` flag)
- ✅ **Memory Protection** - Configurable max cacheable response size (default: 10MB) prevents memory exhaustion from large responses
- ✅ **Buffer Pool Configuration** - Configurable buffer pool size (default: 64KB) for optimal memory/performance trade-off

## Thread-Safety

**The httpcache package is fully thread-safe** and designed for concurrent use across multiple goroutines. All core operations (`Transport.RoundTrip()`, `Transport.Client()`, metrics) are safe for concurrent access.

**Key Points:**

- ✅ Safe to use from multiple goroutines simultaneously
- ✅ All cache backends are thread-safe
- ⚠️ Configure `Transport` fields **before** use (use `WithXxx()` options)

For detailed information including usage examples, best practices, and troubleshooting, see the **[Thread-Safety Guide](./docs/thread-safety.md)**.

## Quick Start

```go
package main

import (
    "fmt"
    "io"
    "net/http"
    "os"
    
    "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/diskcache"
)

func main() {
    // Create a temporary directory for disk cache
    tmpDir, _ := os.MkdirTemp("", "httpcache-*")
    defer os.RemoveAll(tmpDir)
    
    // Create a cached HTTP client using disk cache
    cache := diskcache.New(tmpDir)
    transport := httpcache.NewTransport(cache)
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

### With Encryption (Optional)

```go
// Enable AES-256-GCM encryption for cached data (fixed salt - backward compatible)
transport := httpcache.NewTransport(cache,
    httpcache.WithEncryption("my-secret-passphrase"),
)

// Enable enhanced encryption with random salts (recommended for new deployments)
transport := httpcache.NewTransport(cache,
    httpcache.WithRandomSaltEncryption("my-secret-passphrase"),
)
```

**Encryption Modes:**

- **`WithEncryption()`** - Fixed salt mode (default)
  - Faster encryption/decryption
  - Smaller encrypted data size
  - Backward compatible with existing deployments
  - Suitable for performance-critical scenarios

- **`WithRandomSaltEncryption()`** - Random salt mode (enhanced security)
  - Unique 32-byte salt per encrypted value
  - Protection against rainbow table attacks
  - NIST SP 800-132 and OWASP compliant
  - Recommended for security-critical applications

See [Encryption Security Example](./examples/encryption-security/) for detailed comparison and migration guide.

### Transport Options

```go
// Configure transport with multiple options
transport := httpcache.NewTransport(cache,
    httpcache.WithEncryption("my-secret-passphrase"),     // Enable encryption
    httpcache.WithPublicCache(true),                       // Shared cache mode
    httpcache.WithVarySeparation(true),                    // RFC 9111 Vary handling
    httpcache.WithCacheKeyHeaders([]string{"Accept-Language"}), // Include headers in key
    httpcache.WithMaxCacheableResponseSize(5*1024*1024),   // Limit cacheable size to 5MB (default: 10MB)
    httpcache.WithMaxPooledBufferSize(128*1024),           // Buffer pool size 128KB (default: 64KB)
    httpcache.WithCacheOperationTimeout(60*time.Second),   // Cache write timeout (default: 30s)
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmXXHash), // Use xxHash for 2.7x faster hashing (default: SHA256)
    httpcache.WithMetrics(metrics),                        // Enable metrics collection (optional)
)

// Enable request deduplication for high-traffic scenarios
transport.EnableDeduplication = true  // Coalesce parallel requests to same resource

// Create and enable metrics (optional)
metrics := httpcache.NewTransportMetrics()
transport := httpcache.NewTransport(cache, httpcache.WithMetrics(metrics))

// Read cache metrics
fmt.Printf("Hit rate: %.2f%%\n", metrics.HitRate()*100)
fmt.Printf("Total requests: %d\n", metrics.TotalRequests())

// Read buffer pool metrics (global, always available)
bufferMetrics := httpcache.GetBufferPoolMetrics()
fmt.Printf("Buffer pool hit rate: %.2f%%\n", bufferMetrics.PoolHitRate())
fmt.Printf("Buffer pool discard rate: %.2f%%\n", bufferMetrics.DiscardRate())

// Export to Prometheus (optional - requires separate package)
import prommetrics "github.com/sandrolain/httpcache/wrapper/metrics/prometheus"
collector := prommetrics.NewCollector(prommetrics.CollectorConfig{Metrics: metrics})
stop := collector.Start(context.Background())
defer stop()
```

> **Note**: All cache keys are automatically hashed before storage, protecting sensitive data in URLs. Default algorithm is SHA-256 (cryptographically secure). For high-throughput scenarios, xxHash can provide ~2.7x faster performance with 72% smaller keys.

> **Hash Algorithm**: Two algorithms available:
>
> - **SHA-256** (default): Cryptographically secure, backward compatible
> - **xxHash**: ~2.7x faster, 72% smaller output, recommended for high-throughput scenarios
> ⚠️ **Warning**: Changing algorithms invalidates existing cache entries.

> **Cache Operation Timeout**: By default, cache write operations have a **30-second timeout** to prevent indefinite operations after request completion. The cache operation uses an independent context to allow completing writes even if the client disconnects, but with this reasonable timeout. Set to 0 to disable (not recommended for production).

> **Memory Protection**: By default, responses larger than **10MB** are not cached to prevent memory exhaustion. This limit can be configured with `WithMaxCacheableResponseSize()` or disabled by setting it to 0. Large responses that exceed the limit are still served normally but bypass the cache.

```

## Installation

```bash
go get github.com/sandrolain/httpcache
```

## Documentation

### 📚 Core Documentation

- **[Cache Backends](./docs/backends.md)** - Choose and configure storage backends (Memory, Redis, PostgreSQL, etc.)
- **[How It Works](./docs/how-it-works.md)** - RFC 7234 implementation details, cache headers, and detecting cache hits
- **[Advanced Features](./docs/advanced-features.md)** - Transport configuration, stale-if-error, cache key headers, custom cache control
- **[Performance Optimization](./docs/performance-v2.md)** - v2 performance improvements, benchmark results, and optimization techniques
- **[Resilience Features](./docs/resilience.md)** - Retry policies and circuit breakers for fault-tolerant HTTP clients
- **[Security Considerations](./docs/security.md)** - Multi-user applications, secure cache wrapper, best practices
- **[Monitoring with Prometheus](./docs/monitoring.md)** - Optional metrics integration for production monitoring

### 🔍 Quick Navigation

**Getting Started:**

- [Installation](#installation) (this page)
- [Quick Start](#quick-start) (this page)
- [Cache Backends](./docs/backends.md#usage-examples) - See all backend examples

**Common Tasks:**

- [Detecting cache hits](./docs/how-it-works.md#detecting-cache-hits)
- [User-specific caching](./docs/advanced-features.md#cache-key-headers)
- [Authorization header handling](./docs/advanced-features.md#authorization-header-and-shared-caches)
- [Securing sensitive data](./docs/security.md#secure-cache-wrapper)
- [Monitoring performance](./docs/monitoring.md#quick-start)
- [Performance optimization](./docs/performance.md#best-practices)

**Advanced Topics:**

- [RFC 9111 compliance](./docs/how-it-works.md#rfc-9111-compliance-features)
- [Stale-while-revalidate](./docs/advanced-features.md#stale-while-revalidate-support)
- [Multi-tier caching strategies](./wrapper/multicache/README.md)
- [Compression wrapper](./wrapper/compresscache/README.md) - Gzip, Brotli, Snappy compression
- [Custom cache implementation](./docs/how-it-works.md#custom-cache-implementation)
- [Multi-user considerations](./docs/security.md#private-cache-and-multi-user-applications)
- [Benchmark results and analysis](./docs/performance-v2.md#v1-vs-v2-performance-comparison)

## Practical Examples

See the [`examples/`](./examples) directory for complete, runnable examples:

- **[Basic](./examples/basic/)** - Simple in-memory caching
- **[Disk Cache](./examples/diskcache/)** - Persistent filesystem cache
- **[Redis](./examples/redis/)** - Distributed caching with Redis
- **[LevelDB](./examples/leveldb/)** - High-performance persistent cache
- **[PostgreSQL](./examples/postgresql/)** - SQL-based persistent cache
- **[NATS K/V](./examples/natskv/)** - NATS JetStream Key/Value cache
- **[Hazelcast](./examples/hazelcast/)** - Enterprise distributed cache
- **[FreeCache](./examples/freecache/)** - High-performance in-memory with zero GC
- **[Security Best Practices](./examples/security-best-practices/)** - Secure cache with encryption and key hashing
- **[Compress Cache](./examples/compresscache/)** - Automatic Gzip/Brotli/Snappy compression
- **[Encryption Security](./examples/encryption-security/)** - Fixed vs Random salt encryption comparison and migration guide
- **[XXHash Performance](./examples/xxhash-performance/)** - High-performance hashing for throughput-critical scenarios
- **[Multi-Tier Cache](./examples/multicache/)** - Multi-tiered caching with automatic fallback and promotion
- **[Custom Backend](./examples/custom-backend/)** - Build your own cache backend
- **[Prometheus Metrics](./examples/prometheus/)** - Monitoring cache performance
- **[Cache Key Headers](./examples/cachekeyheaders/)** - User-specific caching with headers
- **[Max Cacheable Size](./examples/maxcacheablesize/)** - Prevent memory exhaustion from large responses

Each example includes:

- Complete working code
- Detailed README
- Use case explanations
- Best practices

## Limitations

### Private Directive for Public Caches

⚠️ **Note**: When configured as a public cache (`IsPublicCache: true`), responses with the `Cache-Control: private` directive are not cached.

**Default Behavior**: By default, httpcache operates as a private cache, which allows caching of responses marked as `private`.

**Public Cache Mode**: When `IsPublicCache` is set to `true`, the cache behaves as a shared cache and respects the `private` directive by not caching such responses.

See [Security Considerations](./docs/security.md#private-cache-and-multi-user-applications) and [Advanced Features - Private vs Public Cache](./docs/advanced-features.md#private-vs-public-cache) for details.

### Authorization Header in Shared Caches

⚠️ **Note**: When configured as a shared/public cache (`IsPublicCache: true`), requests with an `Authorization` header are **NOT cached** unless the response contains one of these directives:

- `Cache-Control: public`
- `Cache-Control: must-revalidate`
- `Cache-Control: s-maxage=<seconds>`

This implements **RFC 9111 Section 3.5** to prevent unauthorized access to cached authenticated responses in shared caches.

**Default Behavior**: Private caches (default) can cache Authorization responses without restrictions.

**Shared Cache Mode**: Requires explicit server permission via the directives above. Additionally, use `CacheKeyHeaders` to separate cache entries per user:

```go
transport.IsPublicCache = true
transport.CacheKeyHeaders = []string{"Authorization"}  // Separate cache per user
```

See [Authorization Header and Shared Caches](./docs/advanced-features.md#authorization-header-and-shared-caches) for detailed examples and security considerations.

## Performance

httpcache v2 is highly optimized for production use with minimal overhead:

- **Memory cache**: ~12 ns/op for Get, ~29 ns/op for Set (0 allocations)
- **Header normalization**: 36-273 ns/op depending on complexity (single-pass algorithm)
- **Vary matching**: 33-712 ns/op depending on header count
- **Overhead vs no-cache**: < 5% for cache hits

**v2 Key Optimizations:**

- **xxHash implementation**: 63-85% faster than SHA-256, 90-95% less memory
- **Buffer pooling**: 79-82% faster for large buffers, zero allocations
- **Cache-Control parsing cache**: 67-94% faster for repeated parsing
- **Cache key memoization**: 98.7% faster for repeated lookups
- **Optimized header normalization**: 42-73% faster than v1

For detailed benchmark results, v1 vs v2 comparisons, and optimization techniques, see the [Performance Documentation](./docs/performance-v2.md) and [Migration Guide](./docs/migration-v1-to-v2.md).

Benchmarks are also available in each backend's `*_bench_test.go` file.

## Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run integration tests (requires Docker)
go test -tags=integration ./...

# Run benchmarks
go test -bench=. ./...
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes with tests
4. Run `golangci-lint run` and `govulncheck ./...`
5. Commit your changes (`git commit -m 'feat: add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

## Acknowledgments

This project is a fork of [gregjones/httpcache](https://github.com/gregjones/httpcache) by Greg Jones, which was archived in 2023. We're grateful for the original work and continue its development with modern Go practices.

Additional acknowledgments:

- [RFC 9111](https://www.rfc-editor.org/rfc/rfc9111.html) - HTTP Caching (obsoletes RFC 7234)
- [RFC 7234](https://www.rfc-editor.org/rfc/rfc7234.html) - HTTP Caching (obsoleted by RFC 9111, still referenced for historical context)
- [RFC 5861](https://www.rfc-editor.org/rfc/rfc5861.html) - HTTP Cache-Control Extensions for Stale Content
- All contributors to the original and forked projects

## License

MIT License - see [LICENSE.txt](LICENSE.txt) for details.

## Support

- **Issues**: [GitHub Issues](https://github.com/sandrolain/httpcache/issues)
- **Discussions**: [GitHub Discussions](https://github.com/sandrolain/httpcache/discussions)
- **Documentation**: This README and the [docs/](./docs) directory
- **Examples**: See [examples/](./examples) for practical use cases
