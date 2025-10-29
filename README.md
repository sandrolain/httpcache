# httpcache

[![CI](https://github.com/sandrolain/httpcache/workflows/CI/badge.svg)](https://github.com/sandrolain/httpcache/actions/workflows/ci.yml)
[![Security](https://github.com/sandrolain/httpcache/workflows/Security/badge.svg)](https://github.com/sandrolain/httpcache/actions/workflows/security.yml)
[![Coverage](https://img.shields.io/badge/coverage-95%25-brightgreen.svg)](https://github.com/sandrolain/httpcache)
[![GoDoc](https://godoc.org/github.com/sandrolain/httpcache?status.svg)](https://godoc.org/github.com/sandrolain/httpcache)
[![Go Report Card](https://goreportcard.com/badge/github.com/sandrolain/httpcache)](https://goreportcard.com/report/github.com/sandrolain/httpcache)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE.txt)

**Package httpcache** provides an `http.RoundTripper` implementation that works as a mostly [RFC 7234](https://tools.ietf.org/html/rfc7234) compliant cache for HTTP responses.

> **Note**: This is a maintained fork of [gregjones/httpcache](https://github.com/gregjones/httpcache), which is no longer actively maintained. This fork aims to modernize the codebase while maintaining backward compatibility, fix bugs, and add new features.

## Table of Contents

- [httpcache](#httpcache)
  - [Table of Contents](#table-of-contents)
  - [Features](#features)
  - [Quick Start](#quick-start)
  - [Installation](#installation)
  - [Documentation](#documentation)
    - [📚 Core Documentation](#-core-documentation)
    - [🔍 Quick Navigation](#-quick-navigation)
  - [Practical Examples](#practical-examples)
  - [Limitations](#limitations)
    - [Vary Header](#vary-header)
    - [Private Directive](#private-directive)
  - [Performance](#performance)
  - [Testing](#testing)
  - [Contributing](#contributing)
  - [Acknowledgments](#acknowledgments)
  - [License](#license)
  - [Support](#support)

## Features

- ✅ **RFC 7234 Compliant** (~95% compliance) - Implements HTTP caching standards
  - ✅ Age header calculation (Section 4.2.3)
  - ✅ Warning headers for stale responses (Section 5.5)
  - ✅ must-revalidate directive enforcement (Section 5.2.2.1)
  - ✅ Pragma: no-cache support (Section 5.4)
  - ✅ Cache invalidation on unsafe methods (Section 4.4)
- ✅ **Multiple Backends** - Memory, Disk, Redis, LevelDB, Memcache, PostgreSQL, NATS K/V, Hazelcast
- ✅ **Multi-Tier Caching** - Combine multiple backends with automatic fallback and promotion
- ✅ **Security Wrapper** - Optional SHA-256 key hashing and AES-256 encryption
- ✅ **Thread-Safe** - Safe for concurrent use
- ✅ **Zero Dependencies** - Core package uses only Go standard library
- ✅ **Easy Integration** - Drop-in replacement for `http.Client`
- ✅ **ETag & Validation** - Automatic cache revalidation
- ✅ **Stale-If-Error** - Resilient caching with RFC 5861 support
- ✅ **Stale-While-Revalidate** - Async cache updates for better performance
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
- [Securing sensitive data](./docs/security.md#secure-cache-wrapper)
- [Monitoring performance](./docs/monitoring.md#quick-start)

**Advanced Topics:**

- [RFC 7234 compliance](./docs/how-it-works.md#rfc-7234-compliance-features)
- [Stale-while-revalidate](./docs/advanced-features.md#stale-while-revalidate-support)
- [Multi-tier caching strategies](./wrapper/multicache/README.md)
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

### Vary Header

⚠️ **Current Limitation**: The `Vary` response header is used for **validation only**, not for creating separate cache entries.

**Workaround**: Use [`CacheKeyHeaders`](./docs/advanced-features.md#cache-key-headers) to create true separate cache entries.

See [How It Works - Vary Header Support](./docs/how-it-works.md#vary-header-support) for details.

### Private Directive

⚠️ **Current Limitation**: The `Cache-Control: private` directive is ignored because httpcache implements a private cache.

**Impact**: In multi-user scenarios, responses marked as `private` will still be cached and potentially shared.

**Workaround**: Use `Cache-Control: no-store` or configure [`CacheKeyHeaders`](./docs/advanced-features.md#cache-key-headers).

See [Security Considerations](./docs/security.md#private-cache-and-multi-user-applications) for details.

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
