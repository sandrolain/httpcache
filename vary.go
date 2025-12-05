// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"net/http"
	"sort"
	"strings"
)

// varyMatches will return false unless all of the cached values for the headers listed in Vary
// match the new request
func varyMatches(cachedResp *http.Response, req *http.Request) bool {
	varyHeaders := headerAllCommaSepValues(cachedResp.Header, "vary")

	// RFC 9111 Section 4.1: A stored response with "Vary: *" always fails to match
	for _, header := range varyHeaders {
		if strings.TrimSpace(header) == "*" {
			return false
		}
	}

	// Check each varied header for matching
	for _, header := range varyHeaders {
		header = http.CanonicalHeaderKey(strings.TrimSpace(header))
		if header == "" || header == "*" {
			continue
		}

		// Get the current request header value
		reqValue := req.Header.Get(header)
		// Get the stored request header value from X-Varied-* headers
		storedValue := cachedResp.Header.Get(headerXVariedPrefix + header)

		// RFC 9111 Section 4.1: If header is absent from request, it matches if also absent in stored request
		// Both empty: match
		// One empty, one not: no match
		if !normalizedHeaderValuesMatch(reqValue, storedValue) {
			return false
		}
	}
	return true
}

// normalizedHeaderValuesMatch implements RFC 9111 Section 4.1 header field matching.
// Header fields match if they can be transformed to be identical by:
// - adding or removing whitespace (where allowed)
// - normalizing values in ways known to have identical semantics
//
// This implementation provides basic normalization that works for most headers.
// For production use, more sophisticated normalization could be added for specific
// header types (e.g., Accept-Language, Accept-Encoding).
func normalizedHeaderValuesMatch(value1, value2 string) bool {
	// Exact match (fast path)
	if value1 == value2 {
		return true
	}

	// Normalize whitespace: trim and collapse internal whitespace
	norm1 := normalizeHeaderValue(value1)
	norm2 := normalizeHeaderValue(value2)

	return norm1 == norm2
}

// normalizeHeaderValue normalizes a header value according to RFC 9111 Section 4.1.
// This handles common whitespace variations while preserving semantics.
func normalizeHeaderValue(value string) string {
	// Trim leading/trailing whitespace
	value = strings.TrimSpace(value)

	// First, normalize all whitespace characters (space, tab, newline) to single space
	var normalized strings.Builder
	prevSpace := false
	for _, r := range value {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				normalized.WriteRune(' ')
				prevSpace = true
			}
		} else {
			normalized.WriteRune(r)
			prevSpace = false
		}
	}

	// Now normalize comma-separated lists: "en, fr" and "en,fr" should match
	// Replace ", " with "," (all comma+space combinations already normalized to single space above)
	result := strings.ReplaceAll(normalized.String(), ", ", ",")

	return result
}

// storeVaryHeaders stores the Vary header values in the response for future cache validation.
// RFC 9111 Section 4.1: Values are normalized before storage to enable proper matching.
func storeVaryHeaders(resp *http.Response, req *http.Request) {
	for _, varyKey := range headerAllCommaSepValues(resp.Header, "vary") {
		varyKey = http.CanonicalHeaderKey(strings.TrimSpace(varyKey))
		if varyKey == "" || varyKey == "*" {
			continue
		}

		reqValue := req.Header.Get(varyKey)
		fakeHeader := headerXVariedPrefix + varyKey

		// RFC 9111 Section 4.1: Normalize the value before storing
		// This ensures that future requests with equivalent (but differently formatted)
		// header values will match correctly
		normalizedValue := normalizeHeaderValue(reqValue)
		resp.Header.Set(fakeHeader, normalizedValue)
	}
}

// cacheKeyWithVary returns the cache key for req, including Vary header values from the cached response.
// This implements RFC 9111 vary separation: separate cache entries for each variant.
// The varyHeaders parameter contains the list of headers specified in the Vary response header.
// RFC 9111 Section 4.1: Header values are normalized before inclusion in the cache key.
func cacheKeyWithVary(req *http.Request, varyHeaders []string) string {
	key := cacheKey(req)

	if len(varyHeaders) == 0 {
		return key
	}

	// Collect vary header values from the request
	var varyParts []string
	for _, header := range varyHeaders {
		canonicalHeader := http.CanonicalHeaderKey(strings.TrimSpace(header))
		if canonicalHeader == "" || canonicalHeader == "*" {
			continue
		}

		value := req.Header.Get(canonicalHeader)
		// RFC 9111 Section 4.1: Normalize value before including in cache key
		normalizedValue := normalizeHeaderValue(value)
		// Include even empty values to ensure proper cache separation
		varyParts = append(varyParts, canonicalHeader+":"+normalizedValue)
	}

	if len(varyParts) > 0 {
		// Sort to ensure consistent key generation
		sort.Strings(varyParts)
		key = key + "|vary:" + strings.Join(varyParts, "|")
	}

	return key
}
