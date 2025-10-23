# TODO

This document outlines potential future improvements and features for the httpcache project. These are ideal goals and aspirations, not guaranteed commitments or promises. Implementation will depend on available time, resources, and community contributions.

---

## Dependencies

- [ ] Update existing dependencies to latest stable versions
- [ ] Review and resolve any vulnerabilities in dependencies
- [ ] Evaluate migration of current backend dependencies to better-maintained modules
- [ ] Configure Dependabot for automated dependency updates

---

## Integration from Original Repository

- [ ] Review Pull Requests from the original repository (gregjones/httpcache)
- [ ] Evaluate and integrate relevant PRs
- [ ] Analyze open issues in the original repository
- [ ] Integrate relevant fixes and improvements

---

## Cache Backends

- [ ] Integration/maintenance of additional cache backends:
  - [ ] PostgreSQL backend
  - [ ] MongoDB backend
  - [ ] S3-compatible storage backend
  - [ ] etcd backend
- [ ] Evaluate cloud-native backends (AWS ElastiCache, Google Cloud Memorystore, Azure Cache)
- [ ] Improve documentation for implementing custom backends
- [ ] Comparative benchmarks between different backends

---

## Testing

- [ ] Add integration tests for external systems:
  - [ ] Redis: tests with cluster and sentinel
  - [ ] Memcache: tests with multiple instances
  - [ ] LevelDB: advanced concurrency tests
  - [ ] DiskCache: disk space management tests
- [ ] Stress and load testing
- [ ] Failover and recovery tests
- [ ] Compatibility tests with different versions of external backends
- [ ] Add fuzzing tests for robustness

---

## Documentation

- [ ] Complete guide for selecting the appropriate backend
- [ ] Best practices for production use
- [ ] Configuration examples for common scenarios
- [ ] Migration guide from v1.x (if needed)
- [ ] Documentation for metrics and monitoring

---

## Features

- [ ] Support for context.Context in cache operations
- [ ] Built-in metrics and monitoring
- [ ] Size limits and LRU eviction for MemoryCache
- [ ] Configurable TTL for backends that support it
- [ ] Automatic compression/decompression of responses
- [ ] Cache warming support
- [ ] Distributed cache invalidation

---

## Versioning

- [ ] Establish semantic versioning strategy
- [ ] Create CHANGELOG.md with proper version tracking
- [ ] Tag releases appropriately (v1.x.x, v2.x.x)
- [ ] Document breaking changes and migration paths
- [ ] Implement version compatibility tests
- [ ] Create release automation workflow

---

## Performance

- [ ] Optimize serialization/deserialization operations
- [ ] Reduce memory allocations
- [ ] Continuous benchmarking and performance tracking
- [ ] Profiling and optimization of identified bottlenecks

---

## Security

- [ ] Complete security audit
- [ ] Rate limiting implementation
- [ ] Protection against cache poisoning
- [ ] Robust input validation
- [ ] Security policy and responsible disclosure

---

## Priority

### High Priority

1. Dependencies update
2. Integration of PRs from original repository
3. Integration tests for external systems

### Medium Priority

1. Integration of new backends
2. Documentation improvements
3. Additional features (context, metrics)

### Low Priority

1. Performance optimization
2. Cache warming and distributed invalidation

---

**Note**: These items represent ideal goals and aspirations for the project. They are not commitments or guarantees. Implementation depends on available time, resources, and community contributions.

*Last updated: 2025-10-23*
