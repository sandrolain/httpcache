# Project context

`httpcache` is a Go `http.RoundTripper` implementation of an HTTP response cache, forked from `github.com/gregjones/httpcache`. The project aims to improve RFC 9111 compliance while preserving compatibility with existing users.

The default transport behaves as a private cache. Set `Transport.IsPublicCache` to enable shared-cache semantics, including `s-maxage` lifetime precedence and shared-cache stale-reuse restrictions.

In shared-cache mode, an incoming `Authorization` request can reuse a cached response only when that response explicitly permits shared use with `public`, `must-revalidate`, or `s-maxage`; this applies to anonymous entries already in the cache as well as responses fetched for authorized requests. `CacheKeyHeaders` can additionally isolate entries by credential. Ordinary single non-empty `CacheKeyHeaders` values retain the existing key format; empty and multi-line values use versioned, length-prefixed key segments and may cause one cache miss after upgrade. New Vary entries record presence metadata; legacy entries for unknown Vary fields without metadata are conservatively refetched or revalidated.

Request deduplication, when enabled, is scoped to requests with identical method, Host override, cache key, and exact complete header maps. Each caller has an independent response body and cancellation path. `Vary: *` responses and responses above a nonzero `MaxCacheableResponseSize` are not shared with waiters, which retry independently; zero explicitly permits unlimited buffering.
