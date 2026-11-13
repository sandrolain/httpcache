// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// timer is an interface for time-related operations, allowing for testing.
type timer interface {
	since(d time.Time) time.Duration
}

type realClock struct{}

func (c *realClock) since(d time.Time) time.Duration {
	return time.Since(d)
}

// currentAgeForFreshness computes the RFC 9111 Section 4.2.3 current_age for
// freshness decisions. It uses calculateAge, which applies the full algorithm
// (Age header, response_delay, resident_time) for stored responses with timing
// metadata, and falls back to the simplified Date-based calculation (plus any
// Age header) for legacy cached entries without metadata.
func currentAgeForFreshness(respHeaders http.Header, clock timer, log *slog.Logger) time.Duration {
	if age, err := calculateAge(respHeaders, clock, log); err == nil {
		if age < 0 {
			return 0
		}
		return age
	}
	// No Date header (or unparsable): fall back to 0; callers treat the
	// response as stale when freshness cannot be established.
	return 0
}

// prohibitsStale reports whether an explicit in-protocol directive prohibits
// reusing the stale response without successful validation
// (RFC 9111 Section 4.2.4: a cache MUST NOT generate a stale response if it is
// prohibited by such a directive; the most restrictive directive wins).
//
//   - must-revalidate applies to every cache (Section 5.2.2.1).
//   - proxy-revalidate is equivalent to must-revalidate for shared caches
//     (Section 5.2.2.8).
//   - s-maxage incorporates the proxy-revalidate semantics in shared caches
//     (Section 5.2.2.10).
func prohibitsStale(respCacheControl cacheControl, shared bool) bool {
	if _, ok := respCacheControl[cacheControlMustRevalidate]; ok {
		return true
	}
	if shared {
		if _, ok := respCacheControl[cacheControlProxyRevalidate]; ok {
			return true
		}
		if _, ok := respCacheControl[cacheControlSMaxAge]; ok {
			return true
		}
	}
	return false
}

// getFreshness will return one of fresh/stale/transparent based on the cache-control
// values of the request and the response
//
// fresh indicates the response can be returned
// stale indicates that the response needs validating before it is returned
// transparent indicates the response should not be used to fulfil the request
//
// shared indicates whether the cache operates in shared (public) cache mode:
// s-maxage then takes priority over max-age/Expires (RFC 9111 Section 4.2.1),
// and proxy-revalidate/s-maxage prohibit serving stale without validation.
//
// RFC 9111 Note: This is a private cache implementation by default.
// - Cache-Control: private - Allowed (private caches CAN store these responses)
// - Cache-Control: public - Ignored (has no additional effect in private caches)
// - s-maxage - Ignored unless the cache is shared (IsPublicCache)
func getFreshness(respHeaders, reqHeaders http.Header, clock timer, log *slog.Logger, shared bool) (freshness int) {
	respCacheControl := parseCacheControl(respHeaders, log)
	reqCacheControl := parseCacheControl(reqHeaders, log)

	// Check cache control directives and Pragma
	if result, done := checkCacheControl(respCacheControl, reqCacheControl, reqHeaders); done {
		return result
	}

	if _, err := Date(respHeaders); err != nil {
		return stale
	}
	// RFC 9111 Section 4.2: response_is_fresh = freshness_lifetime > current_age,
	// where current_age is the full Section 4.2.3 algorithm (including the
	// upstream Age header).
	currentAge := currentAgeForFreshness(respHeaders, clock, log)

	// Calculate response lifetime
	date, dateErr := Date(respHeaders)
	if dateErr != nil {
		return stale
	}
	lifetime := calculateLifetime(respCacheControl, respHeaders, date, shared)

	// Adjust age based on request controls and enforce must-revalidate
	responseAge := currentAge
	var returnFresh bool
	currentAge, lifetime, returnFresh = adjustAgeForRequestControls(respCacheControl, reqCacheControl, currentAge, lifetime, shared)
	if returnFresh {
		return fresh
	}

	if lifetime > currentAge {
		return fresh
	}

	// Check for stale-while-revalidate directive.
	// RFC 9111 Section 4.2.4: an explicit directive prohibiting stale reuse
	// (must-revalidate; proxy-revalidate/s-maxage in shared caches) wins over
	// stale-while-revalidate: a stale response MUST NOT be reused without
	// successful validation, even inside the SWR window.
	requestMaxAgeExceeded := false
	if requestMaxAge, ok := reqCacheControl[cacheControlMaxAge]; ok {
		if limit, err := time.ParseDuration(requestMaxAge + "s"); err != nil || responseAge > limit {
			requestMaxAgeExceeded = true
		}
	}
	if stalewhilerevalidate, ok := respCacheControl[cacheControlStaleWhileRevalidate]; ok && !requestMaxAgeExceeded && !prohibitsStale(respCacheControl, shared) {
		// If the cached response isn't too stale, we can return it and refresh asynchronously
		stalewhilerevalidateDuration, err := time.ParseDuration(stalewhilerevalidate + "s")
		if err == nil {
			if lifetime+stalewhilerevalidateDuration > currentAge {
				return staleWhileRevalidate
			}
		}
	}

	return stale
}

// checkCacheControl checks for no-cache directives and Pragma: no-cache.
// RFC 7234 Section 5.4: Pragma: no-cache is treated as Cache-Control: no-cache for HTTP/1.0 compatibility
func checkCacheControl(respCacheControl, reqCacheControl cacheControl, reqHeaders http.Header) (int, bool) {
	if _, ok := reqCacheControl[cacheControlNoCache]; ok {
		return transparent, true
	}
	// RFC 7234 Section 5.4: "When the Cache-Control header field is not present in a request,
	// caches MUST consider the no-cache request pragma-directive as having the same effect
	// as if "Cache-Control: no-cache" were present"
	if len(reqCacheControl) == 0 {
		if strings.EqualFold(reqHeaders.Get(headerPragma), pragmaNoCache) {
			return transparent, true
		}
	}
	if _, ok := respCacheControl[cacheControlNoCache]; ok {
		return stale, true
	}
	return 0, false
}

// calculateLifetime calculates the response lifetime based on s-maxage (shared
// caches), max-age or Expires header
func calculateLifetime(respCacheControl cacheControl, respHeaders http.Header, date time.Time, shared bool) time.Duration {
	var lifetime time.Duration
	var zeroDuration time.Duration

	// RFC 9111 Section 4.2.1: "If the cache is shared and the s-maxage response
	// directive is present, use its value, in seconds" — s-maxage takes
	// priority over max-age and Expires in shared caches.
	if shared {
		if sMaxAge, ok := respCacheControl[cacheControlSMaxAge]; ok {
			parsedLifetime, err := time.ParseDuration(sMaxAge + "s")
			if err != nil {
				return zeroDuration
			}
			return parsedLifetime
		}
	}

	// If a response includes both an Expires header and a max-age directive,
	// the max-age directive overrides the Expires header, even if the Expires header is more restrictive.
	if maxAge, ok := respCacheControl[cacheControlMaxAge]; ok {
		parsedLifetime, err := time.ParseDuration(maxAge + "s")
		if err != nil {
			lifetime = zeroDuration
		} else {
			lifetime = parsedLifetime
		}
	} else {
		expiresHeader := respHeaders.Get("Expires")
		if expiresHeader != "" {
			expires, err := parseHTTPDate(expiresHeader)
			if err != nil {
				lifetime = zeroDuration
			} else {
				lifetime = expires.Sub(date)
			}
		}
	}

	return lifetime
}

// adjustAgeForRequestControls adjusts the current age based on request cache control directives
// and enforces directives that prohibit reuse of stale responses
// (must-revalidate, and in shared caches proxy-revalidate/s-maxage per
// RFC 9111 Sections 4.2.4, 5.2.2.1, 5.2.2.8 and 5.2.2.10)
func adjustAgeForRequestControls(respCacheControl, reqCacheControl cacheControl, currentAge time.Duration, lifetime time.Duration, shared bool) (time.Duration, time.Duration, bool) {
	if maxAge, ok := reqCacheControl[cacheControlMaxAge]; ok {
		// Request max-age limits the age the client will accept; it cannot
		// extend the response's own freshness lifetime (RFC 9111 §5.2.1.1).
		parsedLifetime, err := time.ParseDuration(maxAge + "s")
		if err != nil {
			parsedLifetime = 0
		}
		if parsedLifetime < lifetime {
			lifetime = parsedLifetime
		}
	}

	if minfresh, ok := reqCacheControl["min-fresh"]; ok {
		//  the client wants a response that will still be fresh for at least the specified number of seconds.
		minfreshDuration, err := time.ParseDuration(minfresh + "s")
		if err == nil {
			currentAge = currentAge + minfreshDuration
		}
	}

	// RFC 9111 Section 5.2.2.1: must-revalidate
	// "once it has become stale, a cache MUST NOT use the response to satisfy
	// subsequent requests without successful validation on the origin server"
	// This overrides max-stale from the request. In shared caches the same
	// applies to proxy-revalidate and s-maxage (Sections 5.2.2.8, 5.2.2.10).
	if prohibitsStale(respCacheControl, shared) {
		// Ignore max-stale when a stale-prohibiting directive is present
		return currentAge, lifetime, false
	}

	if maxstale, ok := reqCacheControl["max-stale"]; ok {
		// Indicates that the client is willing to accept a response that has exceeded its expiration time.
		if maxstale == "" {
			return currentAge, lifetime, true // Return fresh for any stale response
		}
		maxstaleDuration, err := time.ParseDuration(maxstale + "s")
		if err == nil {
			currentAge = currentAge - maxstaleDuration
		}
	}

	return currentAge, lifetime, false
}

// isActuallyStale checks if a response is actually stale (ignoring client's max-stale tolerance)
func isActuallyStale(respHeaders http.Header, clock timer, log *slog.Logger, shared bool) bool {
	respCacheControl := parseCacheControl(respHeaders, log)

	date, err := Date(respHeaders)
	if err != nil {
		return true // No date means we can't determine freshness, treat as stale
	}

	currentAge := currentAgeForFreshness(respHeaders, clock, log)
	lifetime := calculateLifetime(respCacheControl, respHeaders, date, shared)

	// Check if stale-while-revalidate extends freshness.
	// Directives prohibiting stale reuse (RFC 9111 Section 4.2.4) override the
	// SWR window: the response counts as stale even inside it.
	if stalewhilerevalidate, ok := respCacheControl[cacheControlStaleWhileRevalidate]; ok && !prohibitsStale(respCacheControl, shared) {
		stalewhilerevalidateDuration, err := time.ParseDuration(stalewhilerevalidate + "s")
		if err == nil {
			if lifetime+stalewhilerevalidateDuration > currentAge {
				return false // Still within stale-while-revalidate window
			}
		}
	}

	return lifetime <= currentAge
}

// freshnessString converts freshness int to string representation
func freshnessString(freshness int) string {
	switch freshness {
	case fresh:
		return freshnessStringFresh
	case stale:
		return freshnessStringStale
	case staleWhileRevalidate:
		return freshnessStringStaleWhileRevalidate
	case transparent:
		return freshnessStringTransparent
	default:
		return freshnessStringUnknown
	}
}

// parseStaleIfError parses the stale-if-error directive from cache control
func parseStaleIfError(cacheControl cacheControl) (time.Duration, bool, bool) {
	staleMaxAge, ok := cacheControl["stale-if-error"]
	if !ok {
		return 0, false, false
	}

	if staleMaxAge == "" {
		return 0, true, true // No value means accept any stale response
	}

	lifetime, err := time.ParseDuration(staleMaxAge + "s")
	if err != nil {
		return 0, false, true // Invalid duration
	}

	return lifetime, false, true
}

// checkStaleIfErrorLifetime checks if the response is within the stale-if-error lifetime
func checkStaleIfErrorLifetime(respHeaders http.Header, staleWindow time.Duration, clock timer, log *slog.Logger, shared bool) bool {
	date, err := Date(respHeaders)
	if err != nil {
		return false
	}
	// Use the full RFC 9111 §4.2.3 current_age (includes the Age header)
	currentAge := currentAgeForFreshness(respHeaders, clock, log)
	responseCC := parseCacheControl(respHeaders, log)
	freshnessLifetime := calculateLifetime(responseCC, respHeaders, date, shared)
	staleDuration := currentAge - freshnessLifetime
	return staleDuration >= 0 && staleDuration <= staleWindow
}

// canStaleOnError determines if a stale response can be returned on error
// cache control extension: https://tools.ietf.org/html/rfc5861
func canStaleOnError(respHeaders, reqHeaders http.Header, clock timer, log *slog.Logger, shared bool) bool {
	respCacheControl := parseCacheControl(respHeaders, log)
	reqCacheControl := parseCacheControl(reqHeaders, log)

	// RFC 9111 Section 4.2.4: a cache MUST NOT generate a stale response if it
	// is prohibited by an explicit in-protocol directive (e.g. must-revalidate;
	// in shared caches proxy-revalidate or s-maxage). Stale-if-error cannot
	// override it: if disconnected, the cache MUST generate an error response
	// rather than reuse the stale response (Section 5.2.2.1).
	if prohibitsStale(respCacheControl, shared) {
		return false
	}

	lifetime := time.Duration(-1)

	// Check response cache control
	if respLifetime, acceptAny, found := parseStaleIfError(respCacheControl); found {
		if acceptAny {
			return true
		}
		lifetime = respLifetime
	}

	// Check request cache control
	if reqLifetime, acceptAny, found := parseStaleIfError(reqCacheControl); found {
		if acceptAny {
			return true
		}
		lifetime = reqLifetime
	}

	// Check if within lifetime
	if lifetime >= 0 {
		return checkStaleIfErrorLifetime(respHeaders, lifetime, clock, log, shared)
	}

	return false
}
