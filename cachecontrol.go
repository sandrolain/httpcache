// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// cacheControl is a map of Cache-Control directive names to their values.
type cacheControl map[string]string

// cacheControlCache stores parsed Cache-Control headers to avoid redundant parsing.
// Key is the raw Cache-Control header value, value is the parsed cacheControl map.
var cacheControlCache sync.Map

// parseCacheControl parses the Cache-Control header and returns a map of directives.
// Results are cached to avoid redundant parsing of the same header value.
// Implements RFC 9111 Section 4.2.1 validation:
// - Duplicate directives: uses the first occurrence, logs warning
// - Conflicting directives: applies the most restrictive, logs warning
// - Invalid values: logs warning but continues processing
func parseCacheControl(headers http.Header, log *slog.Logger) cacheControl {
	// RFC 9110 Section 5.2/5.3: a field value with multiple field lines is
	// combined into one comma-separated list; every line carries semantics.
	lines := headers.Values("Cache-Control")
	ccHeader := strings.Join(lines, ", ")

	// Check cache first
	if cached, ok := cacheControlCache.Load(ccHeader); ok {
		if cc, ok := cached.(cacheControl); ok {
			return cc
		}
	}

	// Parse if not cached
	cc := parseCacheControlInternal(ccHeader, log)

	// Store in cache
	cacheControlCache.Store(ccHeader, cc)

	return cc
}

// parseCacheControlInternal performs the actual parsing of Cache-Control header.
func parseCacheControlInternal(ccHeader string, log *slog.Logger) cacheControl {
	cc := cacheControl{}
	seen := make(map[string]bool)

	for _, part := range splitCacheDirectives(ccHeader) {
		// OWS (Optional Whitespace) is SP / HTAB per RFC 9110 Section 5.6.3.
		part = strings.Trim(part, " \t")
		if part == "" {
			continue
		}

		var directive, value string
		if name, val, found := strings.Cut(part, "="); found {
			directive = strings.Trim(name, " \t")
			value = unquoteCacheValue(strings.Trim(val, " \t"))
		} else {
			directive = part
			value = ""
		}

		// RFC 9111 Section 5.2: cache directives are identified by a token,
		// to be compared case-insensitively.
		directive = strings.ToLower(directive)

		// RFC 9111: Duplicate directives - use first occurrence
		if seen[directive] {
			log.Warn("duplicate Cache-Control directive detected, using first value",
				"directive", directive,
				"ignored_value", value)
			continue
		}

		seen[directive] = true
		cc[directive] = value
	}

	// RFC 9111: Detect conflicting directives and apply most restrictive
	detectConflictingDirectives(cc, log)

	return cc
}

// splitCacheDirectives splits a Cache-Control field value on commas that are
// outside of quoted strings, per the RFC 9111 §5.2 grammar
// (cache-directive = token [ BWS "=" BWS ( token / quoted-string ) ]).
func splitCacheDirectives(header string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for _, r := range header {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && inQuotes:
			current.WriteRune(r)
			escaped = true
		case r == '"':
			inQuotes = !inQuotes
			current.WriteRune(r)
		case r == ',' && !inQuotes:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	parts = append(parts, current.String())
	return parts
}

// unquoteCacheValue removes surrounding DQUOTEs and unescapes quoted-pair
// escapes from a quoted-string value (RFC 9110 Section 5.6.4). Values that are
// not fully quoted are returned unchanged.
func unquoteCacheValue(v string) string {
	if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' || v[len(v)-2] == '\\' {
		return v
	}
	inner := v[1 : len(v)-1]

	var b strings.Builder
	escaped := false
	for _, r := range inner {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		b.WriteRune(r)
	}
	// A trailing backslash without an escape target: keep it as-is.
	if escaped {
		b.WriteRune('\\')
	}
	return b.String()
}

// detectConflictingDirectives checks for conflicting Cache-Control directives
// and applies the most restrictive according to RFC 9111 Section 4.2.1
func detectConflictingDirectives(cc cacheControl, log *slog.Logger) {
	// Conflict: no-cache + max-age
	// no-cache is more restrictive (requires revalidation)
	if _, hasNoCache := cc[cacheControlNoCache]; hasNoCache {
		if maxAge, hasMaxAge := cc[cacheControlMaxAge]; hasMaxAge && maxAge != "" {
			log.Warn(logConflictingDirectives,
				"conflict", "no-cache + max-age",
				"resolution", "no-cache takes precedence (requires revalidation)")
			// Keep no-cache, but also keep max-age for freshness calculation
			// The presence of no-cache will force revalidation regardless
		}
	}

	// Conflict: public + private
	// private is more restrictive (prevents shared cache storage)
	if _, hasPrivate := cc[cacheControlPrivate]; hasPrivate {
		if _, hasPublic := cc[cacheControlPublic]; hasPublic {
			log.Warn(logConflictingDirectives,
				"conflict", "public + private",
				"resolution", "private takes precedence (more restrictive)")
			// Remove public directive as private is more restrictive
			delete(cc, cacheControlPublic)
		}
	}

	// Conflict: no-store + max-age
	// no-store is more restrictive (prevents storage completely)
	if _, hasNoStore := cc[cacheControlNoStore]; hasNoStore {
		if maxAge, hasMaxAge := cc[cacheControlMaxAge]; hasMaxAge && maxAge != "" {
			log.Warn(logConflictingDirectives,
				"conflict", "no-store + max-age",
				"resolution", "no-store takes precedence (prevents caching)")
			// Keep both, but no-store will prevent caching in canStore()
		}
	}

	// Conflict: no-store + must-revalidate
	// no-store is more restrictive (no caching vs stale serving)
	if _, hasNoStore := cc[cacheControlNoStore]; hasNoStore {
		if _, hasMustRevalidate := cc[cacheControlMustRevalidate]; hasMustRevalidate {
			log.Warn(logConflictingDirectives,
				"conflict", "no-store + must-revalidate",
				"resolution", "no-store takes precedence (prevents caching)")
			// must-revalidate is irrelevant if we're not caching
		}
	}

	// Validate max-age and s-maxage values
	validateMaxAgeDirective(cc, cacheControlMaxAge, "max-age", log)
	validateMaxAgeDirective(cc, cacheControlSMaxAge, "s-maxage", log)
}

// validateMaxAgeDirective validates max-age or s-maxage directive values.
// RFC 9111 Section 1.2.2: a delta-seconds value that overflows the greatest
// representable integer MUST be treated as 2147483648, never as a negative.
func validateMaxAgeDirective(cc cacheControl, directiveKey, directiveName string, log *slog.Logger) {
	// RFC 9111 Section 1.2.2: the greatest representable delta-seconds value.
	const maxDeltaSeconds = int64(2147483648)

	if value, hasDirective := cc[directiveKey]; hasDirective && value != "" {
		// delta-seconds is 1*DIGIT; ParseInt also accepts a leading plus sign.
		if value[0] == '+' {
			log.Warn("invalid Cache-Control value (sign prefix)",
				"directive", directiveName,
				"value", value,
				"resolution", "ignoring directive")
			delete(cc, directiveKey)
			return
		}

		// Check if value contains decimal point (float)
		if strings.Contains(value, ".") {
			log.Warn("invalid Cache-Control value (float not allowed)",
				"directive", directiveName,
				"value", value,
				"resolution", "ignoring directive")
			delete(cc, directiveKey)
			return
		}

		n, err := strconv.ParseInt(value, 10, 64)
		switch {
		case err == nil && n > maxDeltaSeconds:
			log.Warn("Cache-Control value overflow, clamping to 2147483648",
				"directive", directiveName,
				"value", value,
				"resolution", "clamped to 2147483648 (RFC 9111 Section 1.2.2)")
			cc[directiveKey] = "2147483648"
		case err != nil && errors.Is(err, strconv.ErrRange):
			// Value exceeds int64: treat as the RFC-defined maximum
			log.Warn("Cache-Control value overflow, clamping to 2147483648",
				"directive", directiveName,
				"value", value,
				"resolution", "clamped to 2147483648 (RFC 9111 Section 1.2.2)")
			cc[directiveKey] = "2147483648"
		case err != nil:
			log.Warn("invalid Cache-Control value (non-numeric)",
				"directive", directiveName,
				"value", value,
				"resolution", "ignoring directive")
			delete(cc, directiveKey)
		case n < 0:
			log.Warn("invalid Cache-Control value (negative)",
				"directive", directiveName,
				"value", value,
				"resolution", "treating as 0")
			cc[directiveKey] = "0"
		}
	}
}

// canStore determines if a response can be stored in the cache based on Cache-Control directives.
// isPublicCache: true if this is a shared/public cache, false for private cache (default)
// canStore returns whether the response can be stored in the cache.
// RFC 9111 Section 3: Storing Responses in Caches
// RFC 9111 Section 5.2.2.3: must-understand directive
// RFC 9111 Section 3.5: Storing Responses to Authenticated Requests
func canStore(req *http.Request, reqCacheControl, respCacheControl cacheControl, isPublicCache bool, statusCode int, log *slog.Logger) (canStore bool) {
	// RFC 9111 Section 5.2.2.3: must-understand directive
	// When must-understand is present, the cache can only store the response if:
	// 1. The status code is understood by the cache, AND
	// 2. All other cache directives are comprehended
	//
	// If must-understand is present and the status code is not understood,
	// the cache MUST NOT store the response, even if other directives would permit it.
	//
	// If must-understand is present and the status code IS understood,
	// then no-store is effectively ignored (the response can be cached).
	if _, hasMustUnderstand := respCacheControl[cacheControlMustUnderstand]; hasMustUnderstand {
		if !understoodStatusCodes[statusCode] {
			// Status code not understood → MUST NOT cache
			return false
		}
		// Status code understood → proceed with caching
		// (this effectively overrides no-store when must-understand is present)
	} else {
		// Normal behavior when must-understand is NOT present
		if _, ok := respCacheControl[cacheControlNoStore]; ok {
			return false
		}
		if _, ok := reqCacheControl[cacheControlNoStore]; ok {
			return false
		}
	}

	// RFC 9111 Section 3.5: Storing Responses to Authenticated Requests
	// A shared cache MUST NOT use a cached response to a request with an Authorization
	// header field unless the response contains a Cache-Control field with the "public",
	// "must-revalidate", or "s-maxage" response directive.
	if isPublicCache && req.Header.Get("Authorization") != "" {
		_, hasPublic := respCacheControl[cacheControlPublic]
		_, hasMustRevalidate := respCacheControl[cacheControlMustRevalidate]
		_, hasSMaxAge := respCacheControl[cacheControlSMaxAge]

		if !hasPublic && !hasMustRevalidate && !hasSMaxAge {
			log.Debug("refusing to cache Authorization request in shared cache",
				"url", req.URL.String(),
				"reason", "no public/must-revalidate/s-maxage directive")
			return false
		}
	}

	// RFC 9111: Check Cache-Control: private directive
	if _, hasPrivate := respCacheControl[cacheControlPrivate]; hasPrivate {
		// Public/shared caches MUST NOT store responses with private directive
		if isPublicCache {
			return false
		}
		// Private caches CAN store responses with Cache-Control: private
	}

	return true
}
