<center><img src="httpcache.png" width="320" alt="httpcache" /></center>

# httpcache

[![CI](https://github.com/sandrolain/httpcache/workflows/CI/badge.svg)](https://github.com/sandrolain/httpcache/actions/workflows/ci.yml)
[![Security](https://github.com/sandrolain/httpcache/workflows/Security/badge.svg)](https://github.com/sandrolain/httpcache/actions/workflows/security.yml)
[![Coverage](https://img.shields.io/badge/coverage-95%25-brightgreen.svg)](https://github.com/sandrolain/httpcache)
[![Go Report Card](https://goreportcard.com/badge/github.com/sandrolain/httpcache)](https://goreportcard.com/report/github.com/sandrolain/httpcache)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE.txt)
[![GoDoc](https://godoc.org/github.com/sandrolain/httpcache?status.svg)](https://godoc.org/github.com/sandrolain/httpcache)

**Package httpcache** provides an `http.RoundTripper` implementation that works as a mostly [RFC 7234](https://tools.ietf.org/html/rfc7234) compliant cache for HTTP responses. It improves application performance by reducing redundant HTTP requests and supports various backends for use cases such as API caching, web scraping, and microservices.

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
  - [Quick Start](#quick-start)
  - [Installation](#installation)
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

- ✅ **RFC 7234 Compliant** (~95% compliance) - Implements HTTP caching standards
  - ✅ Age header calculation with full RFC 9111 Section 4.2.3 algorithm (request_time, response_time, response_delay tracking)
  - ✅ Age header validation per RFC 9111 Section 5.1 (handles multiple values, invalid values with logging)
  - ✅ Cache-Control directive validation per RFC 9111 Section 4.2.1 (duplicate detection, conflict resolution, value validation)
  - ✅ Warning headers for stale responses (Section 5.5)
  - ✅ must-revalidate directive enforcement (Section 5.2.2.1)
  - ✅ Pragma: no-cache support (Section 5.4)
  - ✅ Cache invalidation on unsafe methods (Section 4.4)
  - ✅ Content-Location and Location header invalidation (RFC 9111 Section 4.4)
  - ✅ Same-origin policy enforcement for cache invalidation
  - ✅ Cache-Control: private directive support (RFC 9111 Section 5.2.2.6)
  - ✅ Cache-Control: must-understand directive support (RFC 9111 Section 5.2.2.3)
  - ✅ Vary header matching per RFC 9111 Section 4.1 (wildcard, whitespace normalization, case-insensitive)
  - ✅ Vary header separation - Optional separate cache entries for response variants (RFC 9111 Section 4.1)
- ✅ **Multiple Backends** - Memory, Disk, Redis, LevelDB, Memcache, PostgreSQL, MongoDB, NATS K/V, Hazelcast, Cloud Storage (S3/GCS/Azure)
- ✅ **Multi-Tier Caching** - Combine multiple backends with automatic fallback and promotion
- ✅ **Compression Wrapper** - Automatic Gzip, Brotli, or Snappy compression for cached data
- ✅ **Security Wrapper** - Optional SHA-256 key hashing and AES-256 encryption
- ✅ **Thread-Safe** - Safe for concurrent use
- ✅ **Zero Dependencies** - Core package uses only Go standard library
- ✅ **Easy Integration** - Drop-in replacement for `http.Client`
- ✅ **ETag & Validation** - Automatic cache revalidation
- ✅ **Stale-If-Error** - Resilient caching with RFC 5861 support
- ✅ **Stale-While-Revalidate** - Async cache updates for better performance
- ✅ **Configurable Cache Mode** - Use as private cache (default) or shared/public cache
- ✅ **RFC 9111 Authorization Handling** - Secure caching of authenticated requests in shared caches

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

## Documentation

### 📚 Core Documentation

- **[Cache Backends](./docs/backends.md)** - Choose and configure storage backends (Memory, Redis, PostgreSQL, etc.)
- **[How It Works](./docs/how-it-works.md)** - RFC 7234 implementation details, cache headers, and detecting cache hits
- **[Advanced Features](./docs/advanced-features.md)** - Transport configuration, stale-if-error, cache key headers, custom cache control
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

**Advanced Topics:**

- [RFC 7234 compliance](./docs/how-it-works.md#rfc-7234-compliance-features)
- [Stale-while-revalidate](./docs/advanced-features.md#stale-while-revalidate-support)
- [Multi-tier caching strategies](./wrapper/multicache/README.md)
- [Compression wrapper](./wrapper/compresscache/README.md) - Gzip, Brotli, Snappy compression
- [Custom cache implementation](./docs/how-it-works.md#custom-cache-implementation)
- [Multi-user considerations](./docs/security.md#private-cache-and-multi-user-applications)

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
- **[Multi-Tier Cache](./examples/multicache/)** - Multi-tiered caching with automatic fallback and promotion
- **[Custom Backend](./examples/custom-backend/)** - Build your own cache backend
- **[Prometheus Metrics](./examples/prometheus/)** - Monitoring cache performance
- **[Cache Key Headers](./examples/cachekeyheaders/)** - User-specific caching with headers

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

- **Memory cache**: ~100ns per operation
- **Disk cache**: ~1-5ms per operation (depends on filesystem)
- **Redis cache**: ~1-3ms per operation (network latency dependent)
- **Overhead vs no-cache**: < 5% for cache hits

Benchmarks are available in each backend's `*_bench_test.go` file.

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

- [RFC 7234](https://tools.ietf.org/html/rfc7234) - HTTP Caching specification
- [RFC 5861](https://tools.ietf.org/html/rfc5861) - HTTP Cache-Control Extensions for Stale Content
- All contributors to the original and forked projects

## License

MIT License - see [LICENSE.txt](LICENSE.txt) for details.

## Support

- **Issues**: [GitHub Issues](https://github.com/sandrolain/httpcache/issues)
- **Discussions**: [GitHub Discussions](https://github.com/sandrolain/httpcache/discussions)
- **Documentation**: This README and the [docs/](./docs) directory
- **Examples**: See [examples/](./examples) for practical use cases
