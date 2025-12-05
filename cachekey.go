// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"net/http"
	"sort"
	"strings"
)

// cacheKey returns the cache key for req.
func cacheKey(req *http.Request) string {
	if req.Method == http.MethodGet {
		return req.URL.String()
	} else {
		return req.Method + " " + req.URL.String()
	}
}

// cacheKeyWithHeaders returns the cache key for req, including specified header values.
// This is used when CacheKeyHeaders is configured to differentiate cache entries
// based on request header values.
func cacheKeyWithHeaders(req *http.Request, headers []string) string {
	key := cacheKey(req)

	// Append header values to the key if headers are specified
	if len(headers) > 0 {
		var headerParts []string
		for _, header := range headers {
			canonicalHeader := http.CanonicalHeaderKey(header)
			value := req.Header.Get(canonicalHeader)
			if value != "" {
				headerParts = append(headerParts, canonicalHeader+":"+value)
			}
		}
		if len(headerParts) > 0 {
			// Sort header parts to ensure consistent key generation
			sort.Strings(headerParts)
			key = key + "|" + strings.Join(headerParts, "|")
		}
	}

	return key
}
