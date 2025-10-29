# Documentation

Welcome to the httpcache documentation! This directory contains detailed guides organized by topic.

## 📚 Documentation Index

### Getting Started

- **[Main README](../README.md)** - Project overview, features, quick start, and installation
- **[Cache Backends](./backends.md)** - Choose and configure storage backends
- **[How It Works](./how-it-works.md)** - Understanding the caching mechanism

### Configuration & Usage

- **[Advanced Features](./advanced-features.md)** - Transport configuration, custom cache control, and advanced options
- **[Security Considerations](./security.md)** - Multi-user applications and secure caching
- **[Monitoring](./monitoring.md)** - Prometheus metrics integration

### Practical Resources

- **[Examples](../examples/)** - Complete, runnable examples for all backends and features
- **[Tests](../test/)** - Test utilities and helpers

## 🗺️ Documentation Map

### By Use Case

**I want to...**

- **Start quickly** → [Quick Start](../README.md#quick-start) + [Memory Cache Example](../examples/basic/)
- **Use Redis** → [Redis Backend](./backends.md#redis-cache) + [Redis Example](../examples/redis/)
- **Cache per-user data** → [Cache Key Headers](./advanced-features.md#cache-key-headers) + [Security Guide](./security.md)
- **Add encryption** → [Secure Cache Wrapper](./backends.md#secure-cache-wrapper) + [Security Example](../examples/security-best-practices/)
- **Monitor performance** → [Monitoring Guide](./monitoring.md) + [Prometheus Example](../examples/prometheus/)
- **Handle stale content** → [Stale-If-Error](./advanced-features.md#stale-if-error-support) + [How It Works](./how-it-works.md)

### By Topic

**Core Concepts:**

- [RFC 7234 Implementation](./how-it-works.md#rfc-7234-compliance-features)
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

**Security:**

- [Multi-User Applications](./security.md#private-cache-and-multi-user-applications)
- [Secure Cache Wrapper](./security.md#secure-cache-wrapper)
- [Best Practices](./security.md#additional-security-recommendations)

**Operations:**

- [Prometheus Metrics](./monitoring.md)
- [PromQL Queries](./monitoring.md#example-promql-queries)
- [Grafana Dashboards](./monitoring.md#grafana-dashboard)

## 📖 External Resources

- [RFC 7234 - HTTP Caching](https://tools.ietf.org/html/rfc7234)
- [RFC 5861 - Cache-Control Extensions for Stale Content](https://tools.ietf.org/html/rfc5861)
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
