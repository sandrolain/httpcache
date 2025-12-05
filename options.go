// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"fmt"
	"net/http"
	"time"
)

// TransportOption is a function that configures a Transport.
// Use the With* functions to create TransportOptions.
type TransportOption func(*Transport) error

// WithMarkCachedResponses configures whether responses returned from cache
// should include the X-From-Cache header.
// Default: true when using NewTransport
func WithMarkCachedResponses(mark bool) TransportOption {
	return func(t *Transport) error {
		t.MarkCachedResponses = mark
		return nil
	}
}

// WithSkipServerErrorsFromCache configures whether server errors (5xx status codes)
// should be served from cache. When true, server errors will not be served from cache
// even if they are fresh.
// Default: false
func WithSkipServerErrorsFromCache(skip bool) TransportOption {
	return func(t *Transport) error {
		t.SkipServerErrorsFromCache = skip
		return nil
	}
}

// WithAsyncRevalidateTimeout sets the context timeout for async requests
// triggered by stale-while-revalidate.
// If zero, no timeout is applied to async revalidation requests.
// Default: 0 (no timeout)
func WithAsyncRevalidateTimeout(timeout time.Duration) TransportOption {
	return func(t *Transport) error {
		t.AsyncRevalidateTimeout = timeout
		return nil
	}
}

// WithPublicCache enables public cache mode.
// When true, the cache will NOT store responses with Cache-Control: private directive.
// When false (default), the cache acts as a private cache and CAN store private responses.
// RFC 9111: Private caches (browsers, API clients) can cache private responses.
// Shared caches (CDNs, proxies) must NOT cache private responses.
// Set to true only if using httpcache as a shared/public cache (CDN, reverse proxy).
// Default: false
func WithPublicCache(isPublic bool) TransportOption {
	return func(t *Transport) error {
		t.IsPublicCache = isPublic
		return nil
	}
}

// WithVarySeparation enables RFC 9111 compliant Vary header separation.
// When true, responses with Vary headers create separate cache entries for each variant.
// When false (default), the previous behavior is maintained where variants overwrite each other.
// RFC 9111 Section 4.1: Caches should maintain separate entries for different variants.
// Enable this for full RFC 9111 compliance with content negotiation (Accept-Language, Accept, etc.).
// Note: Enabling this may increase cache storage usage as each variant is stored separately.
// Default: false
func WithVarySeparation(enable bool) TransportOption {
	return func(t *Transport) error {
		t.EnableVarySeparation = enable
		return nil
	}
}

// WithShouldCache allows configuring non-standard caching behavior based on the response.
// The provided function is called to determine whether a non-200 response should be cached.
// This enables caching of responses like 404 Not Found, 301 Moved Permanently, etc.
// If nil, only 200 OK responses are cached (standard behavior).
// Note: This only bypasses the status code check; Cache-Control headers are still respected.
func WithShouldCache(fn func(*http.Response) bool) TransportOption {
	return func(t *Transport) error {
		t.ShouldCache = fn
		return nil
	}
}

// WithCacheKeyHeaders specifies additional request headers to include in the cache key generation.
// This allows creating separate cache entries based on request header values.
// Common use cases include "Authorization" for user-specific caches or "Accept-Language"
// for locale-specific responses.
// Header names are case-insensitive and will be canonicalized.
// Example: []string{"Authorization", "Accept-Language"}
// Note: This is different from the HTTP Vary response header mechanism, which is handled separately.
func WithCacheKeyHeaders(headers []string) TransportOption {
	return func(t *Transport) error {
		t.CacheKeyHeaders = headers
		return nil
	}
}

// WithDisableWarningHeader disables the deprecated Warning header (RFC 7234) in responses.
// RFC 9111 has obsoleted the Warning header field, making it no longer part of the standard.
// When true, Warning headers (110, 111, etc.) will not be added to cached responses.
// Default: false (Warning headers are enabled for backward compatibility).
// Set to true to comply with RFC 9111 and avoid deprecated headers.
func WithDisableWarningHeader(disable bool) TransportOption {
	return func(t *Transport) error {
		t.DisableWarningHeader = disable
		return nil
	}
}

// WithTransport sets the underlying http.RoundTripper used to make requests.
// If nil, http.DefaultTransport is used.
func WithTransport(rt http.RoundTripper) TransportOption {
	return func(t *Transport) error {
		t.Transport = rt
		return nil
	}
}

// WithEncryption enables AES-256-GCM encryption for cached data.
// The passphrase is used to derive an encryption key using scrypt.
// When enabled, all cached data is encrypted before storage and decrypted on retrieval.
// The passphrase must be kept secret and consistent across application restarts.
// Returns an error if the passphrase is empty or encryption initialization fails.
func WithEncryption(passphrase string) TransportOption {
	return func(t *Transport) error {
		if passphrase == "" {
			return fmt.Errorf("encryption passphrase cannot be empty")
		}
		gcm, err := initEncryption(passphrase)
		if err != nil {
			return err
		}
		if t.security == nil {
			t.security = &securityConfig{}
		}
		t.security.gcm = gcm
		t.security.passphrase = passphrase
		return nil
	}
}
