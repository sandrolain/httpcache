// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"net/http"
	"sort"
	"strings"
)

const (
	headerXVariedPresentPrefix = "X-Varied-Present-"
	headerConnectionName       = "Connection"
)

// varyMatches will return false unless all of the cached values for the headers listed in Vary
// match the new request
func varyMatches(cachedResp *http.Response, req *http.Request) bool {
	varyHeaders := headerAllCommaSepValues(cachedResp.Header, "vary")

	// Fast path: no vary headers
	if len(varyHeaders) == 0 {
		return true
	}

	// Single pass through vary headers: check for "*" and match values simultaneously
	for _, header := range varyHeaders {
		header = strings.TrimSpace(header)

		// RFC 9111 Section 4.1: A stored response with "Vary: *" always fails to match
		if header == "*" {
			return false
		}

		if header == "" {
			continue
		}

		canonicalHeader := http.CanonicalHeaderKey(header)

		// Get the current request header value. Unknown fields are kept exact,
		// including their individual field lines, because their whitespace and
		// combination semantics are not known to the cache.
		reqValues := req.Header.Values(canonicalHeader)
		reqPresent := len(reqValues) > 0
		reqValue := varyFieldValue(canonicalHeader, reqValues)
		// Get the stored request header value from X-Varied-* headers
		storedValue := cachedResp.Header.Get(headerXVariedPrefix + canonicalHeader)
		storedPresent, knownPresence := storedVaryHeaderPresence(cachedResp.Header, canonicalHeader)
		if !knownPresence || reqPresent != storedPresent ||
			(!supportsVaryNormalization(canonicalHeader) && !hasCurrentVaryMetadata(cachedResp.Header, canonicalHeader)) {
			return false
		}

		// RFC 9111 Section 4.1: If header is absent from request, it matches if also absent in stored request
		// Both empty: match
		// One empty, one not: no match
		if supportsVaryNormalization(canonicalHeader) {
			reqValue = normalizeVaryHeaderValue(canonicalHeader, reqValue)
			if !normalizedHeaderValuesMatch(reqValue, storedValue) {
				return false
			}
		} else if reqValue != storedValue {
			return false
		}
	}
	return true
}

// storedVaryHeaderPresence reports whether the original request contained the
// nominated field. Older cached entries have no explicit presence metadata;
// non-empty values and absent value headers are unambiguous, while an empty
// stored value is ambiguous and must be revalidated rather than reused.
func storedVaryHeaderPresence(header http.Header, name string) (present, known bool) {
	marker := headerXVariedPresentPrefix + name
	if values, ok := header[marker]; ok {
		if len(values) == 1 && values[0] == "v1:1" {
			return true, true
		}
		if len(values) == 1 && values[0] == "v1:0" {
			return false, true
		}
		return false, false
	}
	if values, ok := header[headerXVariedPrefix+name]; ok {
		if len(values) == 0 || values[0] == "" {
			return false, false
		}
		return true, true
	}
	return false, true
}

func hasCurrentVaryMetadata(header http.Header, name string) bool {
	values := header[headerXVariedPresentPrefix+name]
	return len(values) == 1 && (values[0] == "v1:0" || values[0] == "v1:1")
}

func combinedHeaderValues(header http.Header, name string) string {
	values := header.Values(name)
	trimmed := make([]string, len(values))
	for i := range values {
		trimmed[i] = strings.TrimSpace(values[i])
	}
	return strings.Join(trimmed, ", ")
}

func normalizeVaryHeaderValue(name, value string) string {
	if !supportsVaryNormalization(name) {
		return value
	}
	value = normalizeHeaderValue(value)
	switch http.CanonicalHeaderKey(name) {
	case "Accept-Charset", "Accept-Encoding", "Accept-Language", "Allow", headerConnectionName,
		"Content-Encoding", "Content-Language", "TE", "Trailer", "Transfer-Encoding", "Upgrade":
		return strings.ToLower(value)
	default:
		return value
	}
}

func supportsVaryNormalization(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Accept", "Accept-Charset", "Accept-Encoding", "Accept-Language", headerConnectionName,
		"Content-Encoding", "Content-Language", "Trailer":
		return true
	default:
		return false
	}
}

func varyFieldValue(name string, values []string) string {
	if supportsVaryNormalization(name) {
		return combinedHeaderValues(http.Header{name: values}, name)
	}
	if len(values) == 1 && values[0] != "" && !strings.Contains(values[0], "|") && !strings.HasPrefix(values[0], varyLinesEncodingPrefix) {
		return values[0]
	}
	if len(values) == 0 {
		return ""
	}
	return encodeHeaderValues(varyLinesEncodingPrefix, values)
}

const varyLinesEncodingPrefix = "!vary-lines-v1!"

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
// Optimized for single-pass processing with minimal allocations.
func normalizeHeaderValue(value string) string {
	// Trim leading/trailing whitespace
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}

	// Single pass: normalize whitespace and handle comma-space inline
	var normalized strings.Builder
	normalized.Grow(len(value)) // Pre-allocate capacity

	prevSpace := false
	lastWasComma := false
	inQuotes := false
	escaped := false

	for i := 0; i < len(value); i++ {
		c := value[i]
		if inQuotes {
			normalized.WriteByte(c)
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inQuotes = false
			}
			continue
		}
		if c == '"' {
			normalized.WriteByte(c)
			inQuotes = true
			prevSpace = false
			lastWasComma = false
			continue
		}

		// Handle whitespace characters
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			// Skip spaces after comma
			if lastWasComma {
				continue
			}
			// Skip multiple consecutive spaces
			if !prevSpace {
				// Skip space before comma
				if !hasCommaAhead(value, i) {
					normalized.WriteByte(' ')
					prevSpace = true
				}
			}
		} else {
			normalized.WriteByte(c)
			prevSpace = false
			lastWasComma = (c == ',')
		}
	}

	return normalized.String()
}

// hasCommaAhead checks if there's a comma ahead in the string, skipping whitespace.
func hasCommaAhead(s string, pos int) bool {
	inQuotes := false
	escaped := false
	for j := pos + 1; j < len(s); j++ {
		c := s[j]
		if inQuotes {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inQuotes = false
			}
			continue
		}
		if c == '"' {
			inQuotes = true
			continue
		}
		if c == ',' {
			return true
		}
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
	}
	return false
}

// storeVaryHeaders stores the Vary header values in the response for future cache validation.
// RFC 9111 Section 4.1: Values are normalized before storage to enable proper matching.
func storeVaryHeaders(resp *http.Response, req *http.Request) {
	for _, varyKey := range headerAllCommaSepValues(resp.Header, "vary") {
		varyKey = http.CanonicalHeaderKey(strings.TrimSpace(varyKey))
		if varyKey == "" || varyKey == "*" {
			continue
		}

		reqValues := req.Header.Values(varyKey)
		reqValue := varyFieldValue(varyKey, reqValues)
		fakeHeader := headerXVariedPrefix + varyKey
		present := len(req.Header.Values(varyKey)) > 0

		// RFC 9111 Section 4.1: Normalize the value before storing
		// This ensures that future requests with equivalent (but differently formatted)
		// header values will match correctly
		normalizedValue := normalizeVaryHeaderValue(varyKey, reqValue)
		resp.Header.Set(fakeHeader, normalizedValue)
		if present {
			resp.Header.Set(headerXVariedPresentPrefix+varyKey, "v1:1")
		} else {
			resp.Header.Set(headerXVariedPresentPrefix+varyKey, "v1:0")
		}
	}
}

// cacheKeyWithVary returns the cache key for req, including Vary header values from the cached response.
// This implements RFC 9111 vary separation: separate cache entries for each variant.
// The varyHeaders parameter contains the list of headers specified in the Vary response header.
// RFC 9111 Section 4.1: Header values are normalized before inclusion in the cache key.
func cacheKeyWithVary(req *http.Request, varyHeaders []string) string {
	return cacheKeyWithVaryBase(cacheKey(req), req, varyHeaders)
}

// cacheKeyWithVaryBase adds Vary values to an existing base key, preserving
// any CacheKeyHeaders scoping already present in that key.
func cacheKeyWithVaryBase(baseKey string, req *http.Request, varyHeaders []string) string {
	key := baseKey

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

		values := req.Header.Values(canonicalHeader)
		value := varyFieldValue(canonicalHeader, values)
		presence := "absent"
		if len(values) > 0 {
			presence = "present"
		}
		// RFC 9111 Section 4.1: Normalize only values whose known field syntax
		// permits it; preserve unknown field values exactly.
		normalizedValue := normalizeVaryHeaderValue(canonicalHeader, value)
		if strings.Contains(normalizedValue, "|") || strings.HasPrefix(normalizedValue, varyLinesEncodingPrefix) {
			normalizedValue = encodeHeaderValues(varyLinesEncodingPrefix, []string{normalizedValue})
		}
		// Include presence explicitly: absent and explicitly empty fields differ.
		varyParts = append(varyParts, canonicalHeader+":"+presence+":"+normalizedValue)
	}

	if len(varyParts) > 0 {
		// Sort to ensure consistent key generation
		sort.Strings(varyParts)
		key = key + "|vary:" + strings.Join(varyParts, "|")
	}

	return key
}
