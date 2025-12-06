# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.0] - Unreleased

This is a **major breaking release** that adds `context.Context` support and error returns to the `Cache` interface, enabling modern Go patterns for timeout, cancellation, and error handling.

### Breaking Changes

**Cache Interface Signature Changes**

The `Cache` interface now requires `context.Context` parameter and returns errors:

```go
type Cache interface {
    Get(ctx context.Context, key string) (responseBytes []byte, ok bool, err error)
    Set(ctx context.Context, key string, responseBytes []byte) error
    Delete(ctx context.Context, key string) error
}
```

**Migration Required**: All custom `Cache` implementations must be updated to match the new interface signatures.

**NewTransport Signature Change**

`NewTransport` now accepts optional `TransportOption` functions:

```go
// Old signature (still compatible, no options passed)
NewTransport(c Cache) *Transport

// New signature with options pattern
NewTransport(c Cache, opts ...TransportOption) *Transport
```

### Changed

- All 11 backend implementations updated with context and error support:
  - `MemoryCache`, `DiskCache`, `Redis`, `PostgreSQL`, `MongoDB`
  - `NATS K/V`, `LevelDB`, `Freecache`, `Hazelcast`, `Memcache`, `Blobcache`
- All 3 wrapper implementations updated:
  - `MultiCache`, `CompressCache` (gzip/brotli/snappy), `Prometheus Metrics`
- Context propagation via `req.Context()` in HTTP transport operations
- In-memory caches accept context for interface compliance (ignored internally)
- External backends use context for timeouts and cancellation

### Added

- **Built-in Security Features**: Cache key hashing and optional encryption integrated into core httpcache
  - **SHA-256 Key Hashing**: All cache keys are automatically hashed before being passed to the backend, preventing sensitive data in cache keys from being exposed
  - **AES-256-GCM Encryption**: Optional encryption of cached data via `WithEncryption(passphrase)` option
  - Uses scrypt for secure key derivation from passphrase
- **Options Pattern for Transport Configuration**: New `TransportOption` functional options for cleaner configuration:
  - `WithEncryption(passphrase)` - Enable AES-256-GCM encryption
  - `WithMarkCachedResponses(bool)` - Control X-From-Cache header
  - `WithSkipServerErrorsFromCache(bool)` - Skip 5xx responses from cache
  - `WithAsyncRevalidateTimeout(duration)` - Set timeout for async revalidation
  - `WithPublicCache(bool)` - Enable public/shared cache mode
  - `WithVarySeparation(bool)` - Enable RFC 9111 Vary header separation
  - `WithShouldCache(fn)` - Custom caching logic for non-200 responses
  - `WithCacheKeyHeaders(headers)` - Include headers in cache key
  - `WithDisableWarningHeader(bool)` - Disable deprecated Warning header
  - `WithTransport(rt)` - Set underlying RoundTripper
- `IsEncryptionEnabled() bool` method on Transport to check encryption status
- Timeout and cancellation support for all cache operations
- Error propagation from cache backends (no more silent failures)
- Context value passing for tracing/logging integration

### Documentation

- Migration guide for v1.x → v2.0 (see TODO.md)
- Updated examples demonstrating context usage

### Reference

- Inspired by [PR #113](https://github.com/gregjones/httpcache/pull/113) from original repository

---

## [1.4.0] - 2024-11-24

This release updates all documentation to reference **RFC 9111** (HTTP Caching, June 2022) as the primary standard, which obsoletes RFC 7234 (2014). It also includes several RFC 9111 compliance improvements implemented in previous releases.

### Changed

**RFC 9111 Documentation Update**

- Updated all documentation references from RFC 7234 to RFC 9111 (current HTTP Caching standard)
- Updated RFC links from deprecated `tools.ietf.org` to official `www.rfc-editor.org` URLs
- Clarified that RFC 9111 obsoletes RFC 7234 throughout documentation
- Updated package comments to reference RFC 9111 and clarify private vs shared/public cache behavior

**Files Updated:**

- `README.md` - Main description, features list, and acknowledgments
- `docs/README.md` - Core concepts and external resources
- `docs/how-it-works.md` - Implementation details and compliance features
- `CHANGELOG.md` - Updated v1.2.0 release notes
- `httpcache.go` - Package documentation

### Added

**RFC 9111 Compliance Features** (from v1.3.0 commits)

- `DisableWarningHeader` configuration flag for RFC 9111 compliance (Warning header obsoleted in RFC 9111)
- Enhanced Authorization header handling documentation for shared caches per RFC 9111 Section 3.5
- Vary header matching per RFC 9111 Section 4.1 (wildcard support, case-insensitive, whitespace normalization)
- Cache-Control directive validation per RFC 9111 Section 4.2.1 (duplicate detection, conflict resolution)
- Age header calculation per RFC 9111 Section 4.2.3 (request_time, response_time, response_delay tracking)
- must-understand directive support per RFC 9111 Section 5.2.2.3
- Vary header separation - optional separate cache entries for response variants
- Public cache mode with proper private directive handling per RFC 9111 Section 5.2.2.6
- Cache invalidation improvements per RFC 9111 Section 4.4

### Documentation

- Comprehensive RFC 9111 compliance documentation
- Warning headers marked as "deprecated but supported for compatibility"
- Authorization handling explicitly documented in features list
- Enhanced security considerations for shared caches
- Updated all section references to use RFC 9111 numbering

### Notes

- Implementation was already ~95% RFC 9111 compliant
- This release primarily updates **documentation** to reflect the current standard
- RFC 7234 references maintained for historical context
- All code changes for RFC 9111 compliance were implemented in v1.3.0

---

## [1.3.0] - 2024-11-04

This release adds new cache backends, compression wrapper, and comprehensive Prometheus metrics integration. Major focus on enterprise-ready features and production monitoring.

### Added

**New Cache Backends**

- **MongoDB backend** - Persistent distributed caching with TTL support and automatic expiration
- **BlobCache backend** - Cloud storage support (AWS S3, Google Cloud Storage, Azure Blob Storage)
- **Hazelcast backend** - Enterprise distributed in-memory data grid integration
- **FreeCache backend** - High-performance in-memory cache with zero GC overhead
- **NATS K/V backend** - NATS JetStream Key/Value store integration

**Wrappers and Utilities**

- **CompressCache wrapper** - Automatic compression with Gzip, Brotli, and Snappy support
- **MultiCache wrapper** - Multi-tier caching strategies with automatic fallback and promotion
- **SecureCache wrapper** - Moved to `wrapper/` directory with enhanced encryption support
- **Prometheus metrics** - Comprehensive cache metrics (hits, misses, errors, latency, size)

**Features and Improvements**

- Redis connection pooling and advanced configuration options
- Prometheus metrics integration tests with Testcontainers
- Dynamic nonce size for AES-256-GCM encryption in SecureCache
- Comprehensive documentation for all backends and wrappers
- Logo and images for project branding

### Changed

- Moved Prometheus metrics from `metrics/prometheus` to `wrapper/prometheus` for consistency
- Moved SecureCache to `wrapper/` directory structure
- Updated Go version to 1.25 in CI workflows
- Improved README with updated use cases and better descriptions
- Removed deprecated build constraints in tests (updated to modern Go)

### Fixed

- CI workflow now skips cache for golangci-lint action (prevents stale cache issues)
- CI workflow ignores image files in checks
- Improved error handling in cache backends

### Dependencies

- Updated `aws-sdk-go` and related AWS dependencies to latest versions
- Added dependencies for new backends (MongoDB, Hazelcast, NATS, etc.)

### Documentation

- Added comprehensive documentation in `docs/` directory:
  - Advanced features guide
  - Backend comparison and usage guide
  - How it works (technical details)
  - Monitoring with Prometheus
  - Security considerations
- Added examples for all new backends
- Updated README with backend comparison table

### Tests

- Added integration tests for Prometheus metrics using Testcontainers
- Added benchmark tests for MemoryCache, FreeCache, and other backends
- Improved test coverage for new features

### Performance

- FreeCache backend provides zero-GC overhead for high-performance scenarios
- CompressCache wrapper reduces storage/bandwidth by 60-70% (Gzip), 70-80% (Brotli)
- MultiCache enables tiered caching strategies for optimal performance

---

## [1.2.0] - 2025-10-29

This release achieves **~95% RFC 7234 compliance** with comprehensive HTTP caching standards implementation, new advanced features, and enhanced security documentation.

### Added

**RFC 7234 Compliance**

- **Age header** calculation and automatic addition to cached responses (Section 4.2.3)
- **Warning headers** (110: Response is Stale, 111: Revalidation Failed) per Section 5.5
- **must-revalidate** directive enforcement preventing stale responses (Section 5.2.2.1)
- **Pragma: no-cache** HTTP/1.0 backward compatibility support (Section 5.4)
- **Cache invalidation** on unsafe methods (POST, PUT, DELETE, PATCH) affecting Request-URI, Location, and Content-Location (Section 4.4)

**Advanced Features**

- **Stale-While-Revalidate** (RFC 5861): serve stale responses immediately while revalidating asynchronously in background
- **CacheKeyHeaders**: differentiate cache entries by request headers (Authorization, Accept-Language, etc.) for multi-user/multi-tenant applications
- **ShouldCache hook**: custom logic to cache non-standard status codes (404, redirects, etc.)
- **SkipServerErrorsFromCache**: option to never serve 5xx errors from cache
- **AsyncRevalidateTimeout**: configurable timeout for background revalidation requests

**Developer Experience**

- New headers: `X-Cache-Freshness` (fresh/stale/stale-while-revalidate), `X-Revalidated`, `X-Stale`
- Fixed JSON caching without Content-Length header
- Moved MemoryCache to dedicated file

### Documentation

- Comprehensive security section in README covering multi-user scenarios, cache key exposure, and `Cache-Control: private` limitations
- New `cachekeyheaders` example demonstrating user isolation
- Expanded README with RFC 7234 compliance details, cache headers, and configuration options
- Coverage badge showing 95% test coverage

### Fixed

- JSON responses without Content-Length now cache correctly
- Documented Vary header limitation (validation-only, not separate entries)

### Tests

- 6 new test files covering all RFC 7234 features: age, warning, must-revalidate, pragma, invalidation, cachekeyheaders
- Test coverage increased to **~95%**

### Changed

- Updated Go version to 1.25.3
- CI/CD workflows now include `develop` branch
- Enhanced `.gitignore` for build artifacts

### Known Limitations

- `Cache-Control: private` directive ignored (by design for private cache; use `CacheKeyHeaders` or `no-store` in multi-user scenarios)
- Vary header validates but doesn't create separate entries (use `CacheKeyHeaders` instead)

### Performance

All new features add negligible overhead (~0ns impact on request latency). Background revalidation has zero impact on response time.

### Credits

Inspired by PR #111 (X-Revalidated), PR #117 (Stale-While-Revalidate), and hionay fork improvements.

---

## [1.1.0] - 2025-10-23

First release under new maintenance. This version includes infrastructure improvements, enhanced documentation, and better testing coverage while maintaining full backward compatibility with the original project.

### Added

- GitHub Actions workflows for CI, security scanning (gosec, govulncheck), and automated releases
- Comprehensive examples in `examples/` directory (basic, diskcache, leveldb, redis, custom-backend)
- TODO.md for project roadmap and future improvements
- Enhanced logging with `log/slog` across all cache implementations
- Test coverage improvements and cache freshness logic tests

### Changed

- Updated Go version to 1.25 in CI workflows
- Modernized `.gitignore` to include `*.out` files
- Refactored golangci-lint configuration (action v8)
- Improved test execution with explicit shell specification
- Excluded examples from test coverage metrics

### Documentation

- Expanded and modernized README with clearer examples and better structure
- Removed outdated architecture guide reference
- Updated disk cache documentation to reflect SHA-256 hashing algorithm
- Added comprehensive inline documentation

### Fixed

- Corrected disk cache hashing algorithm documentation (SHA-256)
- Enhanced caching logic and validation
- Improved error handling across implementations

---

## [1.0.0] - 2018-11-30

Original release by gregjones/httpcache. Archived and no longer maintained as of 2023.

This represents the baseline implementation of RFC 7234 HTTP caching for Go, featuring:

- Core HTTP cache implementation with `http.RoundTripper` interface
- Multiple backend support: Memory, Disk, Redis, LevelDB, Memcache
- ETag and Last-Modified validation
- Stale-if-error support (RFC 5861)
- Vary header support
- Cache-Control directive handling

For the complete history of the original project, see the [archived repository](https://github.com/gregjones/httpcache).

---

[1.4.0]: https://github.com/sandrolain/httpcache/compare/v1.3.0...v1.4.0
[1.3.0]: https://github.com/sandrolain/httpcache/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/sandrolain/httpcache/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/sandrolain/httpcache/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/gregjones/httpcache/tree/901d90724c7919163f472a9812253fb26761123d
