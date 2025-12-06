// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// cacheControl is a map of Cache-Control directive names to their values.
type cacheControl map[string]string

// parseCacheControl parses the Cache-Control header and returns a map of directives.
// Implements RFC 9111 Section 4.2.1 validation:
// - Duplicate directives: uses the first occurrence, logs warning
// - Conflicting directives: applies the most restrictive, logs warning
// - Invalid values: logs warning but continues processing
func parseCacheControl(headers http.Header, log *slog.Logger) cacheControl {
	cc := cacheControl{}
	seen := make(map[string]bool)
	ccHeader := headers.Get("Cache-Control")

	for _, part := range strings.Split(ccHeader, ",") {
		part = strings.Trim(part, " ")
		if part == "" {
			continue
		}

		var directive, value string
		if strings.ContainsRune(part, '=') {
			keyval := strings.Split(part, "=")
			directive = strings.Trim(keyval[0], " ")
			value = strings.Trim(keyval[1], " ")
		} else {
			directive = part
			value = ""
		}

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

// validateMaxAgeDirective validates max-age or s-maxage directive values
func validateMaxAgeDirective(cc cacheControl, directiveKey, directiveName string, log *slog.Logger) {
	if value, hasDirective := cc[directiveKey]; hasDirective && value != "" {
		// Check if value contains decimal point (float)
		if strings.Contains(value, ".") {
			log.Warn("invalid Cache-Control value (float not allowed)",
				"directive", directiveName,
				"value", value,
				"resolution", "ignoring directive")
			delete(cc, directiveKey)
			return
		}

		if duration, err := time.ParseDuration(value + "s"); err == nil {
			if duration < 0 {
				log.Warn("invalid Cache-Control value (negative)",
					"directive", directiveName,
					"value", value,
					"resolution", "treating as 0")
				cc[directiveKey] = "0"
			}
		} else {
			log.Warn("invalid Cache-Control value (non-numeric)",
				"directive", directiveName,
				"value", value,
				"resolution", "ignoring directive")
			delete(cc, directiveKey)
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
