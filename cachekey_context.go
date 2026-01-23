package httpcache

import (
	"context"
	"net/http"
)

// cacheKeyContextKey is the context key for storing the computed cache key.
// This allows us to compute the cache key once and reuse it throughout
// the request lifecycle, avoiding repeated expensive hash computations.
type cacheKeyContextKey struct{}

// getCacheKey returns the cache key for the request, computing it once and
// storing it in the request context for subsequent retrievals.
// This optimization avoids recomputing the cache key (which may include
// header hashing and URL string operations) multiple times per request.
//
// The cache key is computed using cacheKeyWithHeaders on first call,
// then memoized in the request context for subsequent calls.
func (t *Transport) getCacheKey(req *http.Request) (string, *http.Request) {
	// Check if key is already in context
	if key, ok := req.Context().Value(cacheKeyContextKey{}).(string); ok {
		return key, req
	}

	// Compute cache key with headers if configured
	key := cacheKeyWithHeaders(req, t.CacheKeyHeaders)

	// Store in context for future retrievals
	ctx := context.WithValue(req.Context(), cacheKeyContextKey{}, key)
	req = req.WithContext(ctx)

	return key, req
}

// getCacheKeyFromContext retrieves the cache key from the request context
// without computing it. Returns empty string if not present.
// This is useful for functions that receive a request with an already
// computed cache key in the context.
func getCacheKeyFromContext(req *http.Request) string {
	if key, ok := req.Context().Value(cacheKeyContextKey{}).(string); ok {
		return key
	}
	return ""
}
