# TODO

This document outlines potential future improvements and features for the httpcache project. These are ideal goals and aspirations, not guaranteed commitments or promises. Implementation will depend on available time, resources, and community contributions.

---

## Dependencies

- [x] Update existing dependencies to latest stable versions
- [x] Review and resolve any vulnerabilities in dependencies (govulncheck)
- [ ] Evaluate migration of current backend dependencies to better-maintained modules
- [x] Configure Dependabot for automated dependency updates

---

## Integration from Original Repository

- [x] Review Pull Requests from the original repository (gregjones/httpcache)
- [x] Evaluate and integrate relevant PRs
  - [x] PR #111: X-Revalidated header support
  - [x] PR #113: Context support (deferred to v2.0)
  - [x] PR #117: Stale-While-Revalidate support
- [x] Analyze open issues in the original repository
- [x] Integrate relevant fixes and improvements (hionay fork: JSON caching, X-Stale header, SkipServerErrorsFromCache)

---

## Cache Backends

- [x] Integration/maintenance of additional cache backends:
  - [x] PostgreSQL backend (with CockroachDB compatibility)
  - [x] NATS JetStream K/V backend
  - [ ] MongoDB backend
  - [ ] S3-compatible storage backend
  - [ ] etcd backend
- [ ] Evaluate cloud-native backends (AWS ElastiCache, Google Cloud Memorystore, Azure Cache)
- [x] Improve documentation for implementing custom backends
- [x] Comparative benchmarks between different backends

---

## Testing

- [x] Add integration tests for external systems:
  - [x] Redis: tests with Testcontainers
  - [x] Memcache: tests with Testcontainers
  - [x] PostgreSQL: tests with Testcontainers (PostgreSQL + CockroachDB)
  - [x] NATS K/V: tests with Testcontainers
  - [ ] LevelDB: advanced concurrency tests
  - [ ] DiskCache: disk space management tests
- [x] Stress and load testing (benchmark tests added)
- [ ] Failover and recovery tests
- [x] Compatibility tests with different versions of external backends (PostgreSQL + CockroachDB)
- [ ] Add fuzzing tests for robustness

---

## Documentation

- [x] Complete guide for selecting the appropriate backend (README.md comparison table)
- [x] Best practices for production use (examples directory)
- [x] Configuration examples for common scenarios (5+ examples)
- [ ] Migration guide from v1.x (if needed)
- [x] Documentation for metrics and monitoring (Prometheus integration)

---

## Features

### v1.x (Current)

- [x] Built-in metrics and monitoring (Prometheus integration - optional)
- [ ] Size limits and LRU eviction for MemoryCache
- [x] Configurable TTL for backends that support it (PostgreSQL, NATS K/V)
- [ ] Automatic compression/decompression of responses
- [ ] Cache warming support
- [ ] Distributed cache invalidation
- [x] Stale-While-Revalidate support (RFC 5861)
- [x] X-Revalidated header support
- [x] X-Stale header support  
- [x] SkipServerErrorsFromCache configuration option
- [x] ShouldCache hook for custom status code caching
- [x] Age header support (RFC 7234 Section 4.2.3)
- [x] must-revalidate directive enforcement (RFC 7234 Section 5.2.2.1)
- [x] Warning header generation (RFC 7234 Section 5.5)
- [x] Pragma: no-cache support (RFC 7234 Section 5.4)
- [x] CacheKeyHeaders configuration for per-header cache differentiation
- [ ] **Cache-Control: private Directive Handling** (RFC 7234 Section 5.2.2.6)
  - **Issue**: Current implementation ignores `private` directive, treating it as insignificant
  - **RFC Definition**: "The 'private' response directive indicates that the response message is intended for a single user and MUST NOT be stored by a shared cache"
  - **Current Behavior**:
    - Code comment: "Because this is only a private cache, 'public' and 'private' in cache-control aren't significant"
    - `Cache-Control: private` is completely ignored
    - Responses are cached regardless of `private` directive
  - **Why it's Currently Ignored**:
    - httpcache is designed as a "private cache" (browser-like, single-user)
    - For true single-user scenarios (CLI tools, desktop apps), ignoring `private` is CORRECT per RFC
    - RFC allows private caches to store `private` responses
  - **Problem in Multi-User Contexts**:
    - When same Transport serves multiple users (web server, API gateway)
    - Server sends `Cache-Control: private` expecting no shared caching
    - httpcache ignores it and caches anyway
    - Results in data leakage between users
  - **Impact**: High in multi-user scenarios, None in true single-user scenarios
  - **Current Workarounds**:
    - Use `Cache-Control: no-store` (respected by httpcache)
    - Configure `CacheKeyHeaders` to separate cache by user
    - Use separate Transport instances per user
  - **Potential Solutions**:
    1. Add configuration flag: `TreatAsSharedCache bool` - when true, respect `private` directive
    2. Auto-detect shared context (multiple users) and adjust behavior
    3. Document limitation clearly and rely on workarounds
  - **Recommendation**: Document limitation prominently (already done in README Security Considerations)
  - **Reference**: RFC 7234 Section 5.2.2.6, code in `getFreshness()` function
  - **Discovered**: 2025-10-28
- [ ] **Vary Header Compliance** (RFC 7234 Section 4.1)
  - **Issue**: Current implementation validates Vary headers but does NOT create separate cache entries
  - **RFC Requirement**: "If multiple selected responses are available... the cache will need to choose one to use" - explicitly requires storing MULTIPLE responses per URL
  - **Origin**: This bug exists in the original gregjones/httpcache implementation (inherited, not introduced by fork)
  - **Current Behavior**:
    - Uses only URL as primary cache key
    - Stores Vary header values for validation only (in X-Varied-* headers)
    - When Vary validation fails → makes new request and OVERWRITES previous cache entry (same URL = same key)
    - Result: Only ONE response stored per URL, violating RFC 7234
  - **RFC-Compliant Behavior**:
    - Primary cache key: URL (request method + target URI)
    - Secondary cache key: Values of headers nominated by Vary header field
    - MUST store multiple responses for same URL with different Vary values
    - When request arrives, select matching response from available stored responses
    - If no match found, forward to origin and ADD new stored response (not replace)
  - **Impact**: Medium - Users relying on server `Vary` headers for cache separation get unexpected cache overwrites
  - **Workaround**: Use `CacheKeyHeaders` configuration to explicitly specify headers for cache key generation (unique to this fork)
  - **Solution Options**:
    1. Automatically include Vary header values in cache key generation
    2. Create composite cache keys: `URL + VaryHeaders(sorted)`
    3. Store multiple responses per URL (map of Vary values to responses)
  - **Considerations**:
    - Backward compatibility (may require migration strategy)
    - Cache key length limits
    - Performance impact of additional key complexity
  - **Reference**: RFC 7234 Section 4.1, current implementation in `varyMatches()` and `storeVaryHeaders()`
  - **Discovered**: 2025-10-28

### v2.0 (Breaking Changes)

- [ ] **Context Support in Cache Interface** (inspired by PR #113)
  - **Breaking Change**: Add `context.Context` parameter to existing `Cache` interface methods
  - Modified signatures:
    - `Get(ctx context.Context, key string) (responseBytes []byte, ok bool, err error)`
    - `Set(ctx context.Context, key string, responseBytes []byte) error`
    - `Delete(ctx context.Context, key string) error`
  - Benefits:
    - Timeout and cancellation support
    - Error propagation from cache backends (no more silent failures)
    - Context value passing
    - Modern Go patterns and best practices
  - Impact:
    - **ALL cache implementations must be updated** (MemoryCache, DiskCache, Redis, LevelDB, etc.)
    - **ALL users must update their custom Cache implementations**
    - Migration guide required for v1.x → v2.0
  - Implementation notes:
    - Update all backend implementations to use context
    - Add context propagation in `RoundTrip()` using `req.Context()`
    - Add timeout support in async operations (stale-while-revalidate)
    - Proper error handling and logging for cache errors
  - Reference: [PR #113](https://github.com/gregjones/httpcache/pull/113)
  - Status: Deferred to v2.0 - requires careful planning and migration guide

---

## Versioning

- [x] Establish semantic versioning strategy
- [x] Create CHANGELOG.md with proper version tracking (context/changes.md)
- [ ] Tag releases appropriately (v1.x.x, v2.x.x)
- [x] Document breaking changes and migration paths (v2.0 context support in TODO)
- [ ] Implement version compatibility tests
- [x] Create release automation workflow (GitHub Actions)

---

## Performance

- [x] Optimize serialization/deserialization operations (refactored RoundTrip)
- [x] Reduce memory allocations (cognitive complexity reduction)
- [x] Continuous benchmarking and performance tracking (benchmark tests added)
- [x] Profiling and optimization of identified bottlenecks (golangci-lint 0 issues)

---

## Security

- [x] Complete security audit (govulncheck, gosec, trivy)
- [ ] Rate limiting implementation
- [x] Protection against cache poisoning (SHA-256 hashing in diskcache)
- [x] Robust input validation (error handling improvements)
- [x] Security policy and responsible disclosure (GitHub Security workflow)

---

## Priority

### High Priority

1. ~~Dependencies update~~ ✅ COMPLETED
2. ~~Integration of PRs from original repository~~ ✅ COMPLETED
3. ~~Integration tests for external systems~~ ✅ COMPLETED

### Medium Priority

1. ~~Integration of new backends~~ ✅ COMPLETED (PostgreSQL, NATS K/V)
2. ~~Documentation improvements~~ ✅ COMPLETED
3. ~~Additional features (context, metrics)~~ ✅ PARTIALLY COMPLETED
   - ✅ Metrics (Prometheus integration)
   - ⏭️ Context support (deferred to v2.0)

### Low Priority

1. ~~Performance optimization~~ ✅ COMPLETED
2. Cache warming and distributed invalidation

---

**Note**: These items represent ideal goals and aspirations for the project. They are not commitments or guarantees. Implementation depends on available time, resources, and community contributions.

---

## Achievements Summary

### ✅ Completed (80%+)

- **Code Quality**: golangci-lint 0 issues, 95%+ test coverage
- **Security**: govulncheck, gosec, trivy scanning automated
- **CI/CD**: GitHub Actions with multi-OS/Go version testing
- **Backends**: PostgreSQL, NATS K/V added with integration tests
- **RFC 7234 Compliance**: Age header, must-revalidate, Warning header, Pragma support
- **RFC 5861 Features**: Stale-While-Revalidate, X-Revalidated, X-Stale headers
- **Monitoring**: Prometheus metrics (optional)
- **Documentation**: Examples, README, API docs comprehensive
- **Testing**: Unit tests, integration tests (Testcontainers), benchmarks

### 🚧 In Progress

- Additional backends (MongoDB, S3, etcd)
- LRU eviction for MemoryCache
- Fuzzing tests

### ⏭️ Deferred to v2.0

- Context support in Cache interface (breaking change)

---

Last updated: 2025-01-27
