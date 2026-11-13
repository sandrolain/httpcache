# Project context

`httpcache` is a Go `http.RoundTripper` implementation of an HTTP response cache, forked from `github.com/gregjones/httpcache`. The project aims to improve RFC 9111 compliance while preserving compatibility with existing users.

The default transport behaves as a private cache. Set `Transport.IsPublicCache` to enable shared-cache semantics, including `s-maxage` lifetime precedence and shared-cache stale-reuse restrictions.
