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

var clock timer = &realClock{}

// getFreshness will return one of fresh/stale/transparent based on the cache-control
// values of the request and the response
//
// fresh indicates the response can be returned
// stale indicates that the response needs validating before it is returned
// transparent indicates the response should not be used to fulfil the request
//
// RFC 9111 Note: This is a private cache implementation.
// - Cache-Control: private - Allowed (private caches CAN store these responses)
// - Cache-Control: public - Ignored (has no additional effect in private caches)
// - s-maxage - Ignored (only applies to shared caches)
func getFreshness(respHeaders, reqHeaders http.Header, log *slog.Logger) (freshness int) {
	respCacheControl := parseCacheControl(respHeaders, log)
	reqCacheControl := parseCacheControl(reqHeaders, log)

	// Check cache control directives and Pragma
	if result, done := checkCacheControl(respCacheControl, reqCacheControl, reqHeaders); done {
		return result
	}

	date, err := Date(respHeaders)
	if err != nil {
		return stale
	}
	currentAge := clock.since(date)

	// Calculate response lifetime
	lifetime := calculateLifetime(respCacheControl, respHeaders, date)

	// Adjust age based on request controls and enforce must-revalidate
	var returnFresh bool
	currentAge, lifetime, returnFresh = adjustAgeForRequestControls(respCacheControl, reqCacheControl, currentAge, lifetime)
	if returnFresh {
		return fresh
	}

	if lifetime > currentAge {
		return fresh
	}

	// Check for stale-while-revalidate directive
	if stalewhilerevalidate, ok := respCacheControl[cacheControlStaleWhileRevalidate]; ok {
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

// checkCacheControl checks for no-cache directives, Pragma: no-cache, and only-if-cached
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
	if _, ok := reqCacheControl[cacheControlOnlyIfCached]; ok {
		return fresh, true
	}
	return 0, false
}

// calculateLifetime calculates the response lifetime based on max-age or Expires header
func calculateLifetime(respCacheControl cacheControl, respHeaders http.Header, date time.Time) time.Duration {
	var lifetime time.Duration
	var zeroDuration time.Duration

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
			expires, err := time.Parse(time.RFC1123, expiresHeader)
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
// and enforces must-revalidate directive from response
func adjustAgeForRequestControls(respCacheControl, reqCacheControl cacheControl, currentAge time.Duration, lifetime time.Duration) (time.Duration, time.Duration, bool) {
	if maxAge, ok := reqCacheControl[cacheControlMaxAge]; ok {
		// the client is willing to accept a response whose age is no greater than the specified time in seconds
		parsedLifetime, err := time.ParseDuration(maxAge + "s")
		if err != nil {
			// Invalid max-age should force stale
			lifetime = 0
		} else {
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

	// RFC 7234 Section 5.2.2.1: must-revalidate
	// "once it has become stale, a cache MUST NOT use the response to satisfy
	// subsequent requests without successful validation on the origin server"
	// This overrides max-stale from the request
	if _, mustRevalidate := respCacheControl[cacheControlMustRevalidate]; mustRevalidate {
		// Ignore max-stale when must-revalidate is present
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
func isActuallyStale(respHeaders http.Header, log *slog.Logger) bool {
	respCacheControl := parseCacheControl(respHeaders, log)

	date, err := Date(respHeaders)
	if err != nil {
		return true // No date means we can't determine freshness, treat as stale
	}

	currentAge := clock.since(date)
	lifetime := calculateLifetime(respCacheControl, respHeaders, date)

	// Check if stale-while-revalidate extends freshness
	if stalewhilerevalidate, ok := respCacheControl[cacheControlStaleWhileRevalidate]; ok {
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
func checkStaleIfErrorLifetime(respHeaders http.Header, lifetime time.Duration) bool {
	date, err := Date(respHeaders)
	if err != nil {
		return false
	}
	currentAge := clock.since(date)
	return lifetime > currentAge
}

// canStaleOnError determines if a stale response can be returned on error
// cache control extension: https://tools.ietf.org/html/rfc5861
func canStaleOnError(respHeaders, reqHeaders http.Header, log *slog.Logger) bool {
	respCacheControl := parseCacheControl(respHeaders, log)
	reqCacheControl := parseCacheControl(reqHeaders, log)

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
		return checkStaleIfErrorLifetime(respHeaders, lifetime)
	}

	return false
}
