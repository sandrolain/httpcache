# Documentation

Welcome to the httpcache documentation! This directory contains detailed guides organized by topic.

## 📚 Documentation Index

### Getting Started

- **[Main README](../README.md)** - Project overview, features, quick start, and installation
- **[Cache Backends](./backends.md)** - Choose and configure storage backends
- **[How It Works](./how-it-works.md)** - Understanding the caching mechanism

### Configuration & Usage

- **[Advanced Features](./advanced-features.md)** - Transport configuration, custom cache control, and advanced options
- **[Performance Optimization](./performance-v2.md)** - v2 performance improvements and benchmark results
- **[Migration Guide](./migration-v1-to-v2.md)** - Step-by-step guide for upgrading from v1 to v2
- **[Security Considerations](./security.md)** - Multi-user applications and secure caching
- **[Monitoring](./monitoring.md)** - Prometheus metrics integration

### Practical Resources

- **[Examples](../examples/)** - Complete, runnable examples for all backends and features
- **[Tests](../test/)** - Test utilities and helpers

## 🗺️ Documentation Map

### By Use Case

**I want to...**

- **Start quickly** → [Quick Start](../README.md#quick-start) + [Memory Cache Example](../examples/basic/)
- **Upgrade from v1** → [Migration Guide](./migration-v1-to-v2.md) + [Performance Comparison](./performance-v2.md)
- **Use Redis** → [Redis Backend](./backends.md#redis-cache) + [Redis Example](../examples/redis/)
- **Cache per-user data** → [Cache Key Headers](./advanced-features.md#cache-key-headers) + [Security Guide](./security.md)
- **Add encryption** → [Secure Cache Wrapper](./backends.md#secure-cache-wrapper) + [Security Example](../examples/security-best-practices/)
- **Monitor performance** → [Monitoring Guide](./monitoring.md) + [Prometheus Example](../examples/prometheus/)
- **Optimize performance** → [Performance Guide](./performance-v2.md) + [Benchmark Results](./performance-v2.md#benchmark-results)
- **Handle stale content** → [Stale-If-Error](./advanced-features.md#stale-if-error-support) + [How It Works](./how-it-works.md)

### By Topic

**Core Concepts:**

- [RFC 9111 Implementation](./how-it-works.md#rfc-9111-compliance-features)
- [Cache Headers](./how-it-works.md#cache-headers-supported)
- [Cache Key Generation](./advanced-features.md#cache-key-headers)
- [Freshness Validation](./how-it-works.md)

**Backends:**

- [All Backends Comparison](./backends.md#built-in-backends)
- [Memory](./backends.md#memory-cache-default)
- [Disk](./backends.md#disk-cache)
- [Redis](./backends.md#redis-cache)
- [PostgreSQL](./backends.md#postgresql-cache)
- [NATS K/V](./backends.md#nats-kv-cache)
- [Custom Backend](../examples/custom-backend/)

**Advanced:**

- [Transport Configuration](./advanced-features.md#transport-configuration)
- [Custom Logger](./advanced-features.md#custom-logger)
- [Stale-While-Revalidate](./advanced-features.md#stale-while-revalidate-support)
- [Custom Cache Control](./advanced-features.md#custom-cache-control-with-shouldcache)
- [Vary Header Limitations](./how-it-works.md#vary-header-support)
- [Performance Benchmarks](./performance-v2.md#v1-vs-v2-performance-comparison)

**Security:**

- [Multi-User Applications](./security.md#private-cache-and-multi-user-applications)
- [Secure Cache Wrapper](./security.md#secure-cache-wrapper)
- [Best Practices](./security.md#additional-security-recommendations)

**Operations:**

- [Prometheus Metrics](./monitoring.md)
- [PromQL Queries](./monitoring.md#example-promql-queries)
- [Grafana Dashboards](./monitoring.md#grafana-dashboard)
- [Performance Profiling](./performance-v2.md#profiling-and-monitoring)

## 📖 External Resources

- [RFC 9111 - HTTP Caching](https://www.rfc-editor.org/rfc/rfc9111.html) (current standard, obsoletes RFC 7234)
- [RFC 7234 - HTTP Caching](https://www.rfc-editor.org/rfc/rfc7234.html) (obsoleted by RFC 9111)
- [RFC 5861 - Cache-Control Extensions for Stale Content](https://www.rfc-editor.org/rfc/rfc5861.html)
- [GoDoc API Reference](https://godoc.org/github.com/sandrolain/httpcache)

## 🤝 Contributing

Found an error in the documentation? Want to add examples or clarifications?

1. Check the [Contributing Guide](../README.md#contributing)
2. Open an issue or pull request
3. Help improve the docs for everyone!

## 📝 Documentation Standards

Our documentation follows these principles:

- **Clear and concise** - Get to the point quickly
- **Code examples** - Show, don't just tell
- **Real-world use cases** - Practical scenarios
- **Cross-referenced** - Easy navigation between topics
- **Up-to-date** - Maintained alongside code changes
