// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC-compliant cache for http responses.
//
// It is only suitable for use as a 'private' cache (i.e. for a web-browser or an API-client
// and not for a shared proxy).
package httpcache

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	stale = iota
	fresh
	transparent
	staleWhileRevalidate
	// XFromCache is the header added to responses that are returned from the cache
	XFromCache = "X-From-Cache"
	// XRevalidated is the header added to responses that got revalidated
	XRevalidated = "X-Revalidated"
	// XStale is the header added to responses that are stale
	XStale = "X-Stale"
	// XFreshness is the header added to responses indicating the freshness state
	XFreshness = "X-Cache-Freshness"
	// XCachedTime is the internal header used to store when a response was cached
	XCachedTime = "X-Cached-Time"
	// XRequestTime stores when the HTTP request was initiated (for Age calculation per RFC 9111)
	XRequestTime = "X-Request-Time"
	// XResponseTime stores when the HTTP response was received (for Age calculation per RFC 9111)
	XResponseTime = "X-Response-Time"

	methodGET    = "GET"
	methodHEAD   = "HEAD"
	methodPOST   = "POST"
	methodPUT    = "PUT"
	methodPATCH  = "PATCH"
	methodDELETE = "DELETE"

	headerXVariedPrefix   = "X-Varied-"
	headerLastModified    = "last-modified"
	headerETag            = "etag"
	headerAge             = "Age"
	headerWarning         = "Warning"
	headerLocation        = "Location"
	headerContentLocation = "Content-Location"

	cacheControlOnlyIfCached         = "only-if-cached"
	cacheControlNoCache              = "no-cache"
	cacheControlStaleWhileRevalidate = "stale-while-revalidate"
	cacheControlMaxAge               = "max-age"
	cacheControlNoStore              = "no-store"
	cacheControlPrivate              = "private"
	cacheControlMustUnderstand       = "must-understand"
	cacheControlPublic               = "public"
	cacheControlMustRevalidate       = "must-revalidate"
	cacheControlSMaxAge              = "s-maxage"

	headerPragma  = "Pragma"
	pragmaNoCache = "no-cache"

	// Log messages
	logConflictingDirectives = "conflicting Cache-Control directives detected"

	// RFC 7234 Section 5.5: Warning header codes
	warningResponseIsStale     = `110 - "Response is Stale"`
	warningRevalidationFailed  = `111 - "Revalidation Failed"`
	warningDisconnectedOp      = `112 - "Disconnected Operation"`
	warningHeuristicExpiration = `113 - "Heuristic Expiration"`

	// Freshness state strings
	freshnessStringFresh                = "fresh"
	freshnessStringStale                = "stale"
	freshnessStringStaleWhileRevalidate = "stale-while-revalidate"
	freshnessStringTransparent          = "transparent"
	freshnessStringUnknown              = "unknown"
)

// RFC 9111 Section 5.2.2.3: HTTP status codes that are understood by this cache.
// When must-understand directive is present, only responses with these status codes
// can be cached, even if other cache directives would normally prevent caching.
var understoodStatusCodes = map[int]bool{
	200: true, // OK
	203: true, // Non-Authoritative Information
	204: true, // No Content
	206: true, // Partial Content
	300: true, // Multiple Choices
	301: true, // Moved Permanently
	404: true, // Not Found
	405: true, // Method Not Allowed
	410: true, // Gone
	414: true, // URI Too Long
	501: true, // Not Implemented
}

// A Cache interface is used by the Transport to store and retrieve responses.
type Cache interface {
	// Get returns the []byte representation of a cached response and a bool
	// set to true if the value isn't empty
	Get(key string) (responseBytes []byte, ok bool)
	// Set stores the []byte representation of a response against a key
	Set(key string, responseBytes []byte)
	// Delete removes the value associated with the key
	Delete(key string)
}

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

// CachedResponse returns the cached http.Response for req if present, and nil
// otherwise.
func CachedResponse(c Cache, req *http.Request) (resp *http.Response, err error) {
	cachedVal, ok := c.Get(cacheKey(req))
	if !ok {
		return
	}

	b := bytes.NewBuffer(cachedVal)
	return http.ReadResponse(bufio.NewReader(b), req)
}

// cachedResponseWithKey returns the cached http.Response for the given cache key if present, and nil otherwise.
// This is an internal function used when CacheKeyHeaders is configured.
func cachedResponseWithKey(c Cache, req *http.Request, key string) (resp *http.Response, err error) {
	cachedVal, ok := c.Get(key)
	if !ok {
		return
	}

	b := bytes.NewBuffer(cachedVal)
	return http.ReadResponse(bufio.NewReader(b), req)
}

// Transport is an implementation of http.RoundTripper that will return values from a cache
// where possible (avoiding a network request) and will additionally add validators (etag/if-modified-since)
// to repeated requests allowing servers to return 304 / Not Modified
type Transport struct {
	// The RoundTripper interface actually used to make requests
	// If nil, http.DefaultTransport is used
	Transport http.RoundTripper
	Cache     Cache
	// If true, responses returned from the cache will be given an extra header, X-From-Cache
	MarkCachedResponses bool
	// If true, server errors (5xx status codes) will not be served from cache
	// even if they are fresh. This forces a new request to the server.
	// Default is false to maintain backward compatibility.
	SkipServerErrorsFromCache bool
	// AsyncRevalidateTimeout is the context timeout for async requests triggered by stale-while-revalidate.
	// If zero, no timeout is applied to async revalidation requests.
	AsyncRevalidateTimeout time.Duration
	// IsPublicCache enables public cache mode (default: false for private cache).
	// When true, the cache will NOT store responses with Cache-Control: private directive.
	// When false (default), the cache acts as a private cache and CAN store private responses.
	// RFC 9111: Private caches (browsers, API clients) can cache private responses.
	// Shared caches (CDNs, proxies) must NOT cache private responses.
	// Set to true only if using httpcache as a shared/public cache (CDN, reverse proxy).
	IsPublicCache bool
	// EnableVarySeparation enables RFC 9111 compliant Vary header separation (default: false).
	// When true, responses with Vary headers create separate cache entries for each variant.
	// When false (default), the previous behavior is maintained where variants overwrite each other.
	// RFC 9111 Section 4.1: Caches should maintain separate entries for different variants.
	// Enable this for full RFC 9111 compliance with content negotiation (Accept-Language, Accept, etc.).
	// Note: Enabling this may increase cache storage usage as each variant is stored separately.
	EnableVarySeparation bool
	// ShouldCache allows configuring non-standard caching behaviour based on the response.
	// If set, this function is called to determine whether a non-200 response should be cached.
	// This enables caching of responses like 404 Not Found, 301 Moved Permanently, etc.
	// If nil, only 200 OK responses are cached (standard behavior).
	// The function receives the http.Response and should return true to cache it.
	// Note: This only bypasses the status code check; Cache-Control headers are still respected.
	ShouldCache func(*http.Response) bool
	// CacheKeyHeaders specifies additional request headers to include in the cache key generation.
	// This allows creating separate cache entries based on request header values.
	// Common use cases include "Authorization" for user-specific caches or "Accept-Language"
	// for locale-specific responses.
	// Header names are case-insensitive and will be canonicalized.
	// Example: []string{"Authorization", "Accept-Language"}
	// Note: This is different from the HTTP Vary response header mechanism, which is handled separately.
	CacheKeyHeaders []string
}

// NewTransport returns a new Transport with the
// provided Cache implementation and MarkCachedResponses set to true
func NewTransport(c Cache) *Transport {
	return &Transport{Cache: c, MarkCachedResponses: true}
}

// Client returns an *http.Client that caches responses.
func (t *Transport) Client() *http.Client {
	return &http.Client{Transport: t}
}

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

// addValidatorsToRequest adds conditional request headers (If-None-Match, If-Modified-Since)
// to revalidate a stale cached response
func addValidatorsToRequest(req *http.Request, cachedResp *http.Response) *http.Request {
	etag := cachedResp.Header.Get(headerETag)
	lastModified := cachedResp.Header.Get(headerLastModified)

	// Only add validators if they're not already present
	needsEtag := etag != "" && req.Header.Get(headerETag) == ""
	needsLastModified := lastModified != "" && req.Header.Get(headerLastModified) == ""

	if !needsEtag && !needsLastModified {
		return req
	}

	req2 := cloneRequest(req)
	if needsEtag {
		req2.Header.Set("if-none-match", etag)
	}
	if needsLastModified {
		req2.Header.Set("if-modified-since", lastModified)
	}
	return req2
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

// asyncRevalidate triggers an asynchronous revalidation of the cached response
func (t *Transport) asyncRevalidate(req *http.Request) {
	bgContext := context.Background()
	var cancelContext context.CancelFunc

	if t.AsyncRevalidateTimeout > 0 {
		bgContext, cancelContext = context.WithTimeout(bgContext, t.AsyncRevalidateTimeout)
	}

	noCacheRequest := req.Clone(bgContext)
	noCacheRequest.Header.Set("cache-control", cacheControlNoCache)

	go func() {
		if cancelContext != nil {
			defer cancelContext()
		}

		GetLogger().Debug("starting async revalidation", "url", req.URL.String())

		resp, err := t.RoundTrip(noCacheRequest)
		if err != nil {
			GetLogger().Warn("async revalidation failed", "url", req.URL.String(), "error", err)
			return
		}
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				GetLogger().Warn("failed to close async revalidation response body", "url", req.URL.String(), "error", closeErr)
			}
		}()

		// Drain the response body to complete the request and allow caching
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			GetLogger().Warn("failed to drain async revalidation response", "url", req.URL.String(), "error", err)
		} else {
			GetLogger().Debug("async revalidation completed", "url", req.URL.String())
		}
	}()
}

// handleCachedResponse processes a cached response based on its freshness
// Returns the request (possibly modified with validators) and whether to use cache directly
func (t *Transport) handleCachedResponse(cachedResp *http.Response, req *http.Request) (*http.Request, bool) {
	if !varyMatches(cachedResp, req) {
		return req, false
	}

	// Don't serve server errors (5xx) from cache if SkipServerErrorsFromCache is enabled
	if t.SkipServerErrorsFromCache && cachedResp.StatusCode >= http.StatusInternalServerError {
		return req, false
	}

	freshness := getFreshness(cachedResp.Header, req.Header)

	// Add freshness header if marking cached responses
	if t.MarkCachedResponses {
		cachedResp.Header.Set(XFreshness, freshnessString(freshness))
	}

	// Calculate and set Age header (RFC 7234 Section 4.2.3)
	if age, err := calculateAge(cachedResp.Header); err == nil {
		cachedResp.Header.Set(headerAge, formatAge(age))
	}

	if freshness == fresh {
		// Check if it's actually stale but served due to max-stale
		if isActuallyStale(cachedResp.Header) {
			// RFC 7234 Section 5.5: Add Warning 110 (Response is Stale)
			addStaleWarning(cachedResp)
		}
		return req, true
	}

	if freshness == staleWhileRevalidate {
		// RFC 7234 Section 5.5: Add Warning 110 (Response is Stale)
		addStaleWarning(cachedResp)
		// Trigger async revalidation
		t.asyncRevalidate(req)
		return req, true
	}

	if freshness == stale {
		return addValidatorsToRequest(req, cachedResp), false
	}

	return req, false
}

// handleNotModifiedResponse updates the cached response with new headers from a 304 response
func handleNotModifiedResponse(cachedResp *http.Response, newResp *http.Response, markRevalidated bool) *http.Response {
	endToEndHeaders := getEndToEndHeaders(newResp.Header)
	for _, header := range endToEndHeaders {
		cachedResp.Header[header] = newResp.Header[header]
	}
	if markRevalidated {
		cachedResp.Header[XRevalidated] = []string{"1"}
	}

	// Recalculate and update Age header after revalidation (RFC 7234 Section 4.2.3)
	if age, err := calculateAge(cachedResp.Header); err == nil {
		cachedResp.Header.Set(headerAge, formatAge(age))
	}

	return cachedResp
}

// shouldReturnStaleOnError checks if a stale cached response should be returned due to an error
func shouldReturnStaleOnError(err error, resp *http.Response, cachedResp *http.Response, req *http.Request) bool {
	if req.Method != methodGET || cachedResp == nil {
		return false
	}

	hasError := err != nil
	hasServerError := resp != nil && resp.StatusCode >= 500

	if !hasError && !hasServerError {
		return false
	}

	return canStaleOnError(cachedResp.Header, req.Header)
}

// performRequest executes the HTTP request using the provided transport
func performRequest(transport http.RoundTripper, req *http.Request, onlyIfCached bool) (*http.Response, error) {
	if onlyIfCached {
		return newGatewayTimeoutResponse(req), nil
	}

	// RFC 9111 Section 4.2.3: Track request_time for Age calculation
	requestTime := time.Now().UTC()

	resp, err := transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// RFC 9111 Section 4.2.3: Track response_time for Age calculation
	responseTime := time.Now().UTC()

	// Store timing information in response headers for Age calculation
	// Only if response has headers (defensive check)
	if resp != nil && resp.Header != nil {
		resp.Header.Set(XRequestTime, requestTime.Format(time.RFC3339))
		resp.Header.Set(XResponseTime, responseTime.Format(time.RFC3339))
	}

	return resp, nil
}

// storeVaryHeaders stores the Vary header values in the response for future cache validation
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

// setupCachingBody wraps the response body to cache it when fully read
func (t *Transport) setupCachingBody(resp *http.Response, cacheKey string) {
	resp.Body = &cachingReadCloser{
		R: resp.Body,
		OnEOF: func(r io.Reader) {
			resp := *resp
			resp.Body = io.NopCloser(r)
			// Add cached timestamp (backward compatibility with X-Cached-Time)
			// X-Request-Time and X-Response-Time are already set by performRequest
			resp.Header.Set(XCachedTime, resp.Header.Get(XResponseTime))
			respBytes, err := httputil.DumpResponse(&resp, true)
			if err == nil {
				t.Cache.Set(cacheKey, respBytes)
			}
		},
	}
}

// setupCachingBodyMultiple stores the cached response under multiple cache keys when the
// response body is fully read. This is used for Vary separation where we also keep
// a manifest or pointer under the base key to allow discovery of variant keys.
func (t *Transport) setupCachingBodyMultiple(resp *http.Response, cacheKeys []string) {
	resp.Body = &cachingReadCloser{
		R: resp.Body,
		OnEOF: func(r io.Reader) {
			respCopy := *resp
			respCopy.Body = io.NopCloser(r)
			// Add cached timestamp (backward compatibility with X-Cached-Time)
			// X-Request-Time and X-Response-Time are already set by performRequest
			respCopy.Header.Set(XCachedTime, respCopy.Header.Get(XResponseTime))
			respBytes, err := httputil.DumpResponse(&respCopy, true)
			if err == nil {
				for _, k := range cacheKeys {
					t.Cache.Set(k, respBytes)
				}
			}
		},
	}
}

// storeCachedResponse caches the response immediately
func (t *Transport) storeCachedResponse(resp *http.Response, cacheKey string) {
	// Add cached timestamp (backward compatibility with X-Cached-Time)
	// X-Request-Time and X-Response-Time are already set by performRequest
	resp.Header.Set(XCachedTime, resp.Header.Get(XResponseTime))
	respBytes, err := httputil.DumpResponse(resp, true)
	if err == nil {
		t.Cache.Set(cacheKey, respBytes)
	}
}

// processCachedResponse handles the logic when a valid cached response exists
func (t *Transport) processCachedResponse(cachedResp *http.Response, req *http.Request, transport http.RoundTripper, cacheKey string) (*http.Response, error) {
	if t.MarkCachedResponses {
		cachedResp.Header.Set(XFromCache, "1")
	}

	modifiedReq, useCache := t.handleCachedResponse(cachedResp, req)
	if useCache {
		return cachedResp, nil
	}

	resp, err := performRequest(transport, modifiedReq, false)

	// Handle 304 Not Modified
	if err == nil && req.Method == methodGET && resp.StatusCode == http.StatusNotModified {
		// Drain and close the 304 response body since we're using the cached response
		if resp != nil {
			if drainErr := drainDiscardedBody(resp.Body); drainErr != nil {
				GetLogger().Warn("error draining 304 response body", "error", drainErr)
			}
		}
		return handleNotModifiedResponse(cachedResp, resp, t.MarkCachedResponses), nil
	}

	if shouldReturnStaleOnError(err, resp, cachedResp, req) {
		// Drain and close the error response body since we're using the cached response
		if resp != nil {
			if drainErr := drainDiscardedBody(resp.Body); drainErr != nil {
				GetLogger().Warn("error draining stale response body", "error", drainErr)
			}
		}
		if t.MarkCachedResponses {
			cachedResp.Header.Set(XStale, "1")
		}
		// RFC 7234 Section 5.5: Add Warning 111 (Revalidation Failed)
		addRevalidationFailedWarning(cachedResp)
		return cachedResp, nil
	}

	if err != nil || resp.StatusCode != http.StatusOK {
		t.Cache.Delete(cacheKey)
	}

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// processUncachedRequest handles the logic when no valid cached response exists
func processUncachedRequest(transport http.RoundTripper, req *http.Request) (*http.Response, error) {
	reqCacheControl := parseCacheControl(req.Header)
	_, onlyIfCached := reqCacheControl[cacheControlOnlyIfCached]
	return performRequest(transport, req, onlyIfCached)
}

// storeResponseInCache stores the response in cache if applicable
func (t *Transport) storeResponseInCache(resp *http.Response, req *http.Request, cacheKey string, cacheable bool) {
	respCacheControl := parseCacheControl(resp.Header)
	reqCacheControl := parseCacheControl(req.Header)

	if !cacheable || !canStore(req, reqCacheControl, respCacheControl, t.IsPublicCache, resp.StatusCode) {
		t.Cache.Delete(cacheKey)
		return
	}

	// RFC 9111 Section 5.2.2.3: must-understand directive
	// When must-understand is present and status code is understood, always cache
	_, hasMustUnderstand := respCacheControl[cacheControlMustUnderstand]
	mustUnderstandAllowsCaching := hasMustUnderstand && understoodStatusCodes[resp.StatusCode]

	// Check if we should cache based on status code
	// RFC 7231 section 6.1: Cacheable by default status codes
	shouldCache := resp.StatusCode == http.StatusOK ||
		resp.StatusCode == http.StatusNonAuthoritativeInfo || // 203
		resp.StatusCode == http.StatusNoContent || // 204
		resp.StatusCode == http.StatusPartialContent || // 206
		resp.StatusCode == http.StatusMultipleChoices || // 300
		resp.StatusCode == http.StatusMovedPermanently || // 301
		resp.StatusCode == http.StatusNotFound || // 404
		resp.StatusCode == http.StatusMethodNotAllowed || // 405
		resp.StatusCode == http.StatusGone || // 410
		resp.StatusCode == http.StatusRequestURITooLong || // 414
		resp.StatusCode == http.StatusNotImplemented || // 501
		mustUnderstandAllowsCaching // must-understand overrides status code check

	// Allow custom override via ShouldCache hook
	if !shouldCache && t.ShouldCache != nil {
		shouldCache = t.ShouldCache(resp)
	}

	if !shouldCache {
		t.Cache.Delete(cacheKey)
		return
	}

	storeVaryHeaders(resp, req)

	// RFC 9111 Vary Separation: If EnableVarySeparation is true and response has Vary headers,
	// create separate cache entries for each variant (new behavior).
	// Otherwise, use the previous behavior where variants overwrite each other (default).
	varyHeaders := headerAllCommaSepValues(resp.Header, "vary")
	if t.EnableVarySeparation && len(varyHeaders) > 0 {
		// Keep original base key so we can also persist a manifest/last-variant there
		baseKey := cacheKey
		// Use vary-specific cache key for this variant
		varyKey := cacheKeyWithVary(req, varyHeaders)

		if req.Method == methodGET {
			// Store the full response under both the variant key and the base key so
			// that RoundTrip can read the base entry (to discover Vary) and then
			// re-lookup the variant-specific entry. This preserves backward compatibility
			// with existing lookup behaviour while providing separate entries per variant.
			t.setupCachingBodyMultiple(resp, []string{varyKey, baseKey})
			return
		}

		// Non-GET responses: store under both keys immediately
		t.storeCachedResponse(resp, varyKey)
		// Also store a copy under base key
		respCopy := *resp
		t.storeCachedResponse(&respCopy, baseKey)
		return
	}

	if req.Method == methodGET {
		t.setupCachingBody(resp, cacheKey)
	} else {
		t.storeCachedResponse(resp, cacheKey)
	}
}

// RoundTrip takes a Request and returns a Response
//
// If there is a fresh Response already in cache, then it will be returned without connecting to
// the server.
//
// If there is a stale Response, then any validators it contains will be set on the new request
// to give the server a chance to respond with NotModified. If this happens, then the cached Response
// will be returned.
func (t *Transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	cacheKey := cacheKeyWithHeaders(req, t.CacheKeyHeaders)
	cacheable := (req.Method == methodGET || req.Method == methodHEAD) && req.Header.Get("range") == ""

	var cachedResp *http.Response
	if cacheable {
		// Try to get cached response
		cachedResp, err = cachedResponseWithKey(t.Cache, req, cacheKey)

		// RFC 9111 Vary Separation: If EnableVarySeparation is true and cached response has Vary headers,
		// recalculate cache key with vary values and try again for the correct variant.
		// This only applies when the new vary separation behavior is enabled.
		if t.EnableVarySeparation && cachedResp != nil && err == nil {
			varyHeaders := headerAllCommaSepValues(cachedResp.Header, "vary")
			if len(varyHeaders) > 0 {
				// Recalculate key with vary headers for proper variant lookup
				varyCacheKey := cacheKeyWithVary(req, varyHeaders)
				if varyCacheKey != cacheKey {
					// Try with vary-specific key
					varyCachedResp, varyErr := cachedResponseWithKey(t.Cache, req, varyCacheKey)
					if varyErr == nil && varyCachedResp != nil {
						cachedResp = varyCachedResp
						cacheKey = varyCacheKey
					}
				}
			}
		}
	} else {
		// RFC 7234 Section 4.4: Invalidate cache on unsafe methods
		// Delete the request URI immediately for unsafe methods
		t.Cache.Delete(cacheKey)
	}

	transport := t.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	// Handle cached vs uncached response
	if cacheable && cachedResp != nil && err == nil {
		resp, err = t.processCachedResponse(cachedResp, req, transport, cacheKey)
	} else {
		resp, err = processUncachedRequest(transport, req)
	}

	if err != nil {
		return nil, err
	}

	// RFC 7234 Section 4.4: Invalidate cache for unsafe methods
	// After successful response, invalidate related URIs
	if isUnsafeMethod(req.Method) {
		t.invalidateCache(req, resp)
	}

	// Store response in cache if applicable
	t.storeResponseInCache(resp, req, cacheKey, cacheable)

	return resp, nil
}

// isUnsafeMethod returns true if the HTTP method is considered unsafe
// RFC 7234 Section 4.4: POST, PUT, DELETE, PATCH are unsafe methods
func isUnsafeMethod(method string) bool {
	return method == methodPOST || method == methodPUT || method == methodDELETE || method == methodPATCH
}

// invalidateCache invalidates cache entries per RFC 9111 Section 4.4
// When receiving a non-error response to an unsafe method, invalidate:
// 1. The effective Request-URI
// 2. URIs in Location and Content-Location response headers (if present and same-origin)
//
// RFC 9111 restricts invalidation to same-origin URIs for security.
func (t *Transport) invalidateCache(req *http.Request, resp *http.Response) {
	// RFC 9111 Section 4.4: Only invalidate on non-error responses
	if resp.StatusCode >= 400 {
		if logger := GetLogger(); logger != nil {
			logger.Debug("skipping cache invalidation for error response",
				"status", resp.StatusCode,
				"url", req.URL.String())
		}
		return
	}

	// Always invalidate the Request-URI
	t.invalidateURI(req.URL, "request-uri")

	// Invalidate Location header URI (RFC 9111 Section 4.4)
	if location := resp.Header.Get(headerLocation); location != "" {
		if err := t.invalidateHeaderURI(req.URL, location, "Location"); err != nil {
			if logger := GetLogger(); logger != nil {
				logger.Debug("failed to invalidate Location URI",
					"location", location,
					"error", err.Error())
			}
		}
	}

	// Invalidate Content-Location header URI (RFC 9111 Section 4.4)
	if contentLocation := resp.Header.Get(headerContentLocation); contentLocation != "" {
		if err := t.invalidateHeaderURI(req.URL, contentLocation, "Content-Location"); err != nil {
			if logger := GetLogger(); logger != nil {
				logger.Debug("failed to invalidate Content-Location URI",
					"content-location", contentLocation,
					"error", err.Error())
			}
		}
	}
}

// invalidateHeaderURI parses and invalidates a URI from a response header.
// It ensures same-origin policy compliance per RFC 9111.
// Returns an error if the URI cannot be parsed.
func (t *Transport) invalidateHeaderURI(requestURL *url.URL, headerValue string, headerName string) error {
	// Parse the header value as a URI (may be relative or absolute)
	targetURL, err := requestURL.Parse(headerValue)
	if err != nil {
		return err
	}

	// RFC 9111 Section 4.4: Only invalidate same-origin URIs
	// Origin = scheme + host (host includes port if present)
	if !isSameOrigin(requestURL, targetURL) {
		if logger := GetLogger(); logger != nil {
			logger.Debug("skipping cross-origin invalidation",
				"header", headerName,
				"request-origin", getOrigin(requestURL),
				"target-origin", getOrigin(targetURL))
		}
		return nil
	}

	t.invalidateURI(targetURL, headerName)
	return nil
}

// invalidateURI removes cache entries for the given URI.
// It invalidates both GET and HEAD requests for the URI.
func (t *Transport) invalidateURI(targetURL *url.URL, source string) {
	// Invalidate GET request for this URL
	getReq := &http.Request{
		Method: methodGET,
		URL:    targetURL,
	}
	getKey := cacheKey(getReq)
	t.Cache.Delete(getKey)

	if logger := GetLogger(); logger != nil {
		logger.Debug("invalidated cache entry",
			"key", getKey,
			"source", source,
			"url", targetURL.String())
	}

	// Also invalidate HEAD request if different key
	headReq := &http.Request{
		Method: methodHEAD,
		URL:    targetURL,
	}
	headKey := cacheKey(headReq)
	if headKey != getKey {
		t.Cache.Delete(headKey)
		if logger := GetLogger(); logger != nil {
			logger.Debug("invalidated HEAD cache entry",
				"key", headKey,
				"source", source)
		}
	}
}

// isSameOrigin checks if two URLs have the same origin.
// Per RFC 9111, origin is defined as scheme + host (including port).
func isSameOrigin(url1, url2 *url.URL) bool {
	return url1.Scheme == url2.Scheme && url1.Host == url2.Host
}

// getOrigin returns the origin string for a URL (scheme://host).
// Used for debugging and logging purposes.
func getOrigin(u *url.URL) string {
	return u.Scheme + "://" + u.Host
}

// ErrNoDateHeader indicates that the HTTP headers contained no Date header.
var ErrNoDateHeader = errors.New("no Date header")

// Date parses and returns the value of the Date header.
func Date(respHeaders http.Header) (date time.Time, err error) {
	dateHeader := respHeaders.Get("date")
	if dateHeader == "" {
		err = ErrNoDateHeader
		return
	}

	return time.Parse(time.RFC1123, dateHeader)
}

// calculateAge calculates the Age header value according to RFC 7234 Section 4.2.3.
// The Age value is the sum of:
//   - age_value: Age header from the response (if present)
//   - date_value: Time since the Date header
//   - resident_time: Time the response has been cached (from X-Cached-Time)
//
// parseAgeHeader parses the Age header according to RFC 9111 Section 5.1.
// Returns the age duration and a boolean indicating if the header is valid.
//
// RFC 9111 requirements:
// - If multiple Age headers exist, use the first value and discard others
// - If the value is invalid (negative, non-numeric), ignore it completely
// - Age header value must be a non-negative integer representing seconds
func parseAgeHeader(headers http.Header) (age time.Duration, valid bool) {
	ageValues := headers.Values(headerAge)

	if len(ageValues) == 0 {
		return 0, false
	}

	// RFC 9111: use the first value, discard others
	ageStr := strings.TrimSpace(ageValues[0])

	if len(ageValues) > 1 {
		GetLogger().Warn("multiple Age headers detected, using first value",
			"count", len(ageValues),
			"first", ageStr,
			"all", ageValues)
	}

	// Validate that it's a non-negative integer
	ageInt, err := strconv.ParseInt(ageStr, 10, 64)
	if err != nil {
		GetLogger().Warn("invalid Age header value, ignoring",
			"value", ageStr,
			"error", err)
		return 0, false
	}

	if ageInt < 0 {
		GetLogger().Warn("negative Age header value, ignoring",
			"value", ageInt)
		return 0, false
	}

	return time.Duration(ageInt) * time.Second, true
}

// calculateAge implements the Age calculation algorithm from RFC 9111 Section 4.2.3.
//
// RFC 9111 formula:
//
//	apparent_age = max(0, response_time - date_value)
//	response_delay = response_time - request_time
//	corrected_age_value = age_value + response_delay
//	corrected_initial_age = max(apparent_age, corrected_age_value)
//	resident_time = now - response_time
//	current_age = corrected_initial_age + resident_time
//
// For cached responses:
//   - request_time is stored in X-Request-Time header
//   - response_time is stored in X-Response-Time header (falls back to X-Cached-Time for compatibility)
//   - date_value comes from Date header
//   - age_value comes from Age header (if present)
func calculateAge(respHeaders http.Header) (age time.Duration, err error) {
	// Get the Date header (required)
	dateValue, err := Date(respHeaders)
	if err != nil {
		return 0, err
	}

	// Get response_time (when we received the response)
	// Try X-Response-Time first, fall back to X-Cached-Time for backward compatibility
	responseTimeStr := respHeaders.Get(XResponseTime)
	if responseTimeStr == "" {
		responseTimeStr = respHeaders.Get(XCachedTime)
	}

	if responseTimeStr == "" {
		// If no cached time, use simplified calculation
		age = clock.since(dateValue)

		// Add any existing Age header
		if ageValue, valid := parseAgeHeader(respHeaders); valid {
			age += ageValue
		}

		return age, nil
	}

	// Parse response_time
	responseTime, parseErr := time.Parse(time.RFC3339, responseTimeStr)
	if parseErr != nil {
		GetLogger().Warn("failed to parse response time header",
			"header", responseTimeStr,
			"error", parseErr)

		// Fallback to simplified calculation
		age = clock.since(dateValue)
		if ageValue, valid := parseAgeHeader(respHeaders); valid {
			age += ageValue
		}
		return age, nil
	}

	// RFC 9111 Section 4.2.3: apparent_age = max(0, response_time - date_value)
	apparentAge := time.Duration(0)
	if responseTime.After(dateValue) {
		apparentAge = responseTime.Sub(dateValue)
	}

	// Parse age_value from Age header (if present)
	ageValue, _ := parseAgeHeader(respHeaders)

	// Get request_time (when we started the request)
	requestTimeStr := respHeaders.Get(XRequestTime)
	responseDelay := time.Duration(0)

	if requestTimeStr != "" {
		requestTime, parseErr := time.Parse(time.RFC3339, requestTimeStr)
		if parseErr == nil && responseTime.After(requestTime) {
			// RFC 9111: response_delay = response_time - request_time
			responseDelay = responseTime.Sub(requestTime)
		} else if parseErr != nil {
			GetLogger().Warn("failed to parse request time header",
				"header", requestTimeStr,
				"error", parseErr)
		}
	}

	// RFC 9111: corrected_age_value = age_value + response_delay
	correctedAgeValue := ageValue + responseDelay

	// RFC 9111: corrected_initial_age = max(apparent_age, corrected_age_value)
	correctedInitialAge := apparentAge
	if correctedAgeValue > correctedInitialAge {
		correctedInitialAge = correctedAgeValue
	}

	// RFC 9111: resident_time = now - response_time
	residentTime := clock.since(responseTime)

	// RFC 9111: current_age = corrected_initial_age + resident_time
	currentAge := correctedInitialAge + residentTime

	return currentAge, nil
}

// formatAge formats a duration as an Age header value (seconds)
func formatAge(age time.Duration) string {
	seconds := int64(age.Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return strconv.FormatInt(seconds, 10)
}

// addWarningHeader adds a Warning header to the response per RFC 7234 Section 5.5
// Warning headers can be stacked, so we use Add instead of Set
func addWarningHeader(resp *http.Response, warningCode string) {
	resp.Header.Add(headerWarning, warningCode)
}

// addStaleWarning adds "110 Response is Stale" warning header
func addStaleWarning(resp *http.Response) {
	addWarningHeader(resp, warningResponseIsStale)
}

// addRevalidationFailedWarning adds "111 Revalidation Failed" warning header
func addRevalidationFailedWarning(resp *http.Response) {
	addWarningHeader(resp, warningRevalidationFailed)
}

// isActuallyStale checks if a response is actually stale (ignoring client's max-stale tolerance)
func isActuallyStale(respHeaders http.Header) bool {
	respCacheControl := parseCacheControl(respHeaders)

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

type realClock struct{}

func (c *realClock) since(d time.Time) time.Duration {
	return time.Since(d)
}

type timer interface {
	since(d time.Time) time.Duration
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
func getFreshness(respHeaders, reqHeaders http.Header) (freshness int) {
	respCacheControl := parseCacheControl(respHeaders)
	reqCacheControl := parseCacheControl(reqHeaders)

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

// Returns true if either the request or the response includes the stale-if-error
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

// cache control extension: https://tools.ietf.org/html/rfc5861
func canStaleOnError(respHeaders, reqHeaders http.Header) bool {
	respCacheControl := parseCacheControl(respHeaders)
	reqCacheControl := parseCacheControl(reqHeaders)

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

func getEndToEndHeaders(respHeaders http.Header) []string {
	// These headers are always hop-by-hop
	hopByHopHeaders := map[string]struct{}{
		"Connection":          {},
		"Keep-Alive":          {},
		"Proxy-Authenticate":  {},
		"Proxy-Authorization": {},
		"Te":                  {},
		"Trailers":            {},
		"Transfer-Encoding":   {},
		"Upgrade":             {},
	}

	for _, extra := range strings.Split(respHeaders.Get("connection"), ",") {
		// any header listed in connection, if present, is also considered hop-by-hop
		if strings.Trim(extra, " ") != "" {
			hopByHopHeaders[http.CanonicalHeaderKey(extra)] = struct{}{}
		}
	}
	endToEndHeaders := []string{}
	for respHeader := range respHeaders {
		if _, ok := hopByHopHeaders[respHeader]; !ok {
			endToEndHeaders = append(endToEndHeaders, respHeader)
		}
	}
	return endToEndHeaders
}

// canStore determines if a response can be stored in the cache based on Cache-Control directives.
// isPublicCache: true if this is a shared/public cache, false for private cache (default)
// canStore returns whether the response can be stored in the cache.
// RFC 9111 Section 3: Storing Responses in Caches
// RFC 9111 Section 5.2.2.3: must-understand directive
// RFC 9111 Section 3.5: Storing Responses to Authenticated Requests
func canStore(req *http.Request, reqCacheControl, respCacheControl cacheControl, isPublicCache bool, statusCode int) (canStore bool) {
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
			GetLogger().Debug("refusing to cache Authorization request in shared cache",
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

func newGatewayTimeoutResponse(req *http.Request) *http.Response {
	var braw bytes.Buffer
	braw.WriteString("HTTP/1.1 504 Gateway Timeout\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(&braw), req)
	if err != nil {
		panic(err)
	}
	return resp
}

// cloneRequest returns a clone of the provided *http.Request.
// The clone is a shallow copy of the struct and its Header map.
// (This function copyright goauth2 authors: https://code.google.com/p/goauth2)
func cloneRequest(r *http.Request) *http.Request {
	// shallow copy of the struct
	r2 := new(http.Request)
	*r2 = *r
	// deep copy of the Header
	r2.Header = make(http.Header)
	for k, s := range r.Header {
		r2.Header[k] = s
	}
	return r2
}

type cacheControl map[string]string

// parseCacheControl parses the Cache-Control header and returns a map of directives.
// Implements RFC 9111 Section 4.2.1 validation:
// - Duplicate directives: uses the first occurrence, logs warning
// - Conflicting directives: applies the most restrictive, logs warning
// - Invalid values: logs warning but continues processing
func parseCacheControl(headers http.Header) cacheControl {
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
			GetLogger().Warn("duplicate Cache-Control directive detected, using first value",
				"directive", directive,
				"ignored_value", value)
			continue
		}

		seen[directive] = true
		cc[directive] = value
	}

	// RFC 9111: Detect conflicting directives and apply most restrictive
	detectConflictingDirectives(cc)

	return cc
}

// detectConflictingDirectives checks for conflicting Cache-Control directives
// and applies the most restrictive according to RFC 9111 Section 4.2.1
func detectConflictingDirectives(cc cacheControl) {
	// Conflict: no-cache + max-age
	// no-cache is more restrictive (requires revalidation)
	if _, hasNoCache := cc[cacheControlNoCache]; hasNoCache {
		if maxAge, hasMaxAge := cc[cacheControlMaxAge]; hasMaxAge && maxAge != "" {
			GetLogger().Warn(logConflictingDirectives,
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
			GetLogger().Warn(logConflictingDirectives,
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
			GetLogger().Warn(logConflictingDirectives,
				"conflict", "no-store + max-age",
				"resolution", "no-store takes precedence (prevents caching)")
			// Keep both, but no-store will prevent caching in canStore()
		}
	}

	// Conflict: no-store + must-revalidate
	// no-store is more restrictive (no caching vs stale serving)
	if _, hasNoStore := cc[cacheControlNoStore]; hasNoStore {
		if _, hasMustRevalidate := cc[cacheControlMustRevalidate]; hasMustRevalidate {
			GetLogger().Warn(logConflictingDirectives,
				"conflict", "no-store + must-revalidate",
				"resolution", "no-store takes precedence (prevents caching)")
			// must-revalidate is irrelevant if we're not caching
		}
	}

	// Validate max-age and s-maxage values
	validateMaxAgeDirective(cc, cacheControlMaxAge, "max-age")
	validateMaxAgeDirective(cc, cacheControlSMaxAge, "s-maxage")
}

// validateMaxAgeDirective validates max-age or s-maxage directive values
func validateMaxAgeDirective(cc cacheControl, directiveKey, directiveName string) {
	if value, hasDirective := cc[directiveKey]; hasDirective && value != "" {
		// Check if value contains decimal point (float)
		if strings.Contains(value, ".") {
			GetLogger().Warn("invalid Cache-Control value (float not allowed)",
				"directive", directiveName,
				"value", value,
				"resolution", "ignoring directive")
			delete(cc, directiveKey)
			return
		}

		if duration, err := time.ParseDuration(value + "s"); err == nil {
			if duration < 0 {
				GetLogger().Warn("invalid Cache-Control value (negative)",
					"directive", directiveName,
					"value", value,
					"resolution", "treating as 0")
				cc[directiveKey] = "0"
			}
		} else {
			GetLogger().Warn("invalid Cache-Control value (non-numeric)",
				"directive", directiveName,
				"value", value,
				"resolution", "ignoring directive")
			delete(cc, directiveKey)
		}
	}
}

// headerAllCommaSepValues returns all comma-separated values (each
// with whitespace trimmed) for header name in headers. According to
// Section 4.2 of the HTTP/1.1 spec
// (http://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2),
// values from multiple occurrences of a header should be concatenated, if
// the header's value is a comma-separated list.
func headerAllCommaSepValues(headers http.Header, name string) []string {
	var vals []string
	for _, val := range headers[http.CanonicalHeaderKey(name)] {
		fields := strings.Split(val, ",")
		for i, f := range fields {
			fields[i] = strings.TrimSpace(f)
		}
		vals = append(vals, fields...)
	}
	return vals
}

// cachingReadCloser is a wrapper around ReadCloser R that calls OnEOF
// handler with a full copy of the content read from R when EOF is
// reached.
type cachingReadCloser struct {
	// Underlying ReadCloser.
	R io.ReadCloser
	// OnEOF is called with a copy of the content of R when EOF is reached.
	OnEOF func(io.Reader)

	buf bytes.Buffer // buf stores a copy of the content of R.
}

// Read reads the next len(p) bytes from R or until R is drained. The
// return value n is the number of bytes read. If R has no data to
// return, err is io.EOF and OnEOF is called with a full copy of what
// has been read so far.
func (r *cachingReadCloser) Read(p []byte) (n int, err error) {
	n, err = r.R.Read(p)
	r.buf.Write(p[:n])
	if err == io.EOF || n < len(p) {
		r.OnEOF(bytes.NewReader(r.buf.Bytes()))
	}
	return n, err
}

func (r *cachingReadCloser) Close() error {
	return r.R.Close()
}

// NewMemoryCacheTransport returns a new Transport using the in-memory cache implementation
func NewMemoryCacheTransport() *Transport {
	c := NewMemoryCache()
	t := NewTransport(c)
	return t
}

const bodyDrainSize = 1 << 15 // 32KB, arbitrary limit for draining

// drainDiscardedBody reads and discards up to drainSize bytes from the body to allow connection reuse.
// It's used when we're discarding a response (e.g., returning stale cache instead of a 500 error).
func drainDiscardedBody(body io.ReadCloser) error {
	if body == nil {
		return nil
	}

	// Drain the body to allow connection reuse
	if _, err := io.Copy(io.Discard, io.LimitReader(body, bodyDrainSize)); err != nil {
		GetLogger().Warn("failed to drain response body", "error", err)
	}

	// Close the body
	if err := body.Close(); err != nil {
		GetLogger().Warn("failed to close response body", "error", err)
		return err
	}

	return nil
}
