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

	headerPragma  = "Pragma"
	pragmaNoCache = "no-cache"

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
	// ShouldCache allows configuring non-standard caching behaviour based on the response.
	// If set, this function is called to determine whether a non-200 response should be cached.
	// This enables caching of responses like 404 Not Found, 301 Moved Permanently, etc.
	// If nil, only 200 OK responses are cached (standard behavior).
	// The function receives the http.Response and should return true to cache it.
	// Note: This only bypasses the status code check; Cache-Control headers are still respected.
	ShouldCache func(*http.Response) bool
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
	for _, header := range headerAllCommaSepValues(cachedResp.Header, "vary") {
		header = http.CanonicalHeaderKey(header)
		if header != "" && req.Header.Get(header) != cachedResp.Header.Get(headerXVariedPrefix+header) {
			return false
		}
	}
	return true
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
	return transport.RoundTrip(req)
}

// storeVaryHeaders stores the Vary header values in the response for future cache validation
func storeVaryHeaders(resp *http.Response, req *http.Request) {
	for _, varyKey := range headerAllCommaSepValues(resp.Header, "vary") {
		varyKey = http.CanonicalHeaderKey(varyKey)
		reqValue := req.Header.Get(varyKey)
		if reqValue != "" {
			fakeHeader := headerXVariedPrefix + varyKey
			resp.Header.Set(fakeHeader, reqValue)
		}
	}
}

// setupCachingBody wraps the response body to cache it when fully read
func (t *Transport) setupCachingBody(resp *http.Response, cacheKey string) {
	resp.Body = &cachingReadCloser{
		R: resp.Body,
		OnEOF: func(r io.Reader) {
			resp := *resp
			resp.Body = io.NopCloser(r)
			// Add timestamp when caching
			resp.Header.Set(XCachedTime, time.Now().UTC().Format(time.RFC3339))
			respBytes, err := httputil.DumpResponse(&resp, true)
			if err == nil {
				t.Cache.Set(cacheKey, respBytes)
			}
		},
	}
}

// storeCachedResponse caches the response immediately
func (t *Transport) storeCachedResponse(resp *http.Response, cacheKey string) {
	// Add timestamp when caching
	resp.Header.Set(XCachedTime, time.Now().UTC().Format(time.RFC3339))
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
	if !cacheable || !canStore(parseCacheControl(req.Header), parseCacheControl(resp.Header)) {
		t.Cache.Delete(cacheKey)
		return
	}

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
		resp.StatusCode == http.StatusNotImplemented // 501

	// Allow custom override via ShouldCache hook
	if !shouldCache && t.ShouldCache != nil {
		shouldCache = t.ShouldCache(resp)
	}

	if !shouldCache {
		t.Cache.Delete(cacheKey)
		return
	}

	storeVaryHeaders(resp, req)

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
	cacheKey := cacheKey(req)
	cacheable := (req.Method == methodGET || req.Method == methodHEAD) && req.Header.Get("range") == ""

	var cachedResp *http.Response
	if cacheable {
		cachedResp, err = CachedResponse(t.Cache, req)
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

// invalidateCache invalidates cache entries per RFC 7234 Section 4.4
// When receiving a non-error response to an unsafe method, invalidate:
// 1. The effective Request-URI
// 2. URIs in Location and Content-Location response headers (if present)
func (t *Transport) invalidateCache(req *http.Request, resp *http.Response) {
	// RFC 7234 Section 4.4: Only invalidate on non-error responses
	if resp.StatusCode >= 400 {
		return
	}

	// Invalidate the request URI
	// Need to invalidate the GET version since cacheKey for GET uses only URL
	getReq := &http.Request{
		Method: methodGET,
		URL:    req.URL,
	}
	getKey := cacheKey(getReq)
	t.Cache.Delete(getKey)

	// Also invalidate HEAD if different
	headReq := &http.Request{
		Method: methodHEAD,
		URL:    req.URL,
	}
	headKey := cacheKey(headReq)
	if headKey != getKey {
		t.Cache.Delete(headKey)
	}

	// Invalidate Location header URI if present
	if locationHeader := resp.Header.Get(headerLocation); locationHeader != "" {
		if locationURL, err := req.URL.Parse(locationHeader); err == nil {
			// Invalidate GET and HEAD for location URL
			locationGetReq := &http.Request{
				Method: methodGET,
				URL:    locationURL,
			}
			t.Cache.Delete(cacheKey(locationGetReq))

			locationHeadReq := &http.Request{
				Method: methodHEAD,
				URL:    locationURL,
			}
			locationHeadKey := cacheKey(locationHeadReq)
			if locationHeadKey != cacheKey(locationGetReq) {
				t.Cache.Delete(locationHeadKey)
			}
		}
	}

	// Invalidate Content-Location header URI if present
	if contentLocationHeader := resp.Header.Get(headerContentLocation); contentLocationHeader != "" {
		if contentLocationURL, err := req.URL.Parse(contentLocationHeader); err == nil {
			// Invalidate GET and HEAD for content-location URL
			contentLocationGetReq := &http.Request{
				Method: methodGET,
				URL:    contentLocationURL,
			}
			t.Cache.Delete(cacheKey(contentLocationGetReq))

			contentLocationHeadReq := &http.Request{
				Method: methodHEAD,
				URL:    contentLocationURL,
			}
			contentLocationHeadKey := cacheKey(contentLocationHeadReq)
			if contentLocationHeadKey != cacheKey(contentLocationGetReq) {
				t.Cache.Delete(contentLocationHeadKey)
			}
		}
	}
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
func calculateAge(respHeaders http.Header) (age time.Duration, err error) {
	// Get the Date header (required)
	date, err := Date(respHeaders)
	if err != nil {
		return 0, err
	}

	// apparent_age = max(0, response_time - date_value)
	// For cached responses, response_time is when we received it (X-Cached-Time)
	apparentAge := time.Duration(0)

	cachedTimeStr := respHeaders.Get(XCachedTime)
	if cachedTimeStr != "" {
		// Parse the cached time
		cachedTime, parseErr := time.Parse(time.RFC3339, cachedTimeStr)
		if parseErr == nil {
			// apparent_age = max(0, cached_time - date_value)
			if cachedTime.After(date) {
				apparentAge = cachedTime.Sub(date)
			}

			// resident_time = now - cached_time
			residentTime := clock.since(cachedTime)

			// Parse any Age header that was already present
			ageValue := time.Duration(0)
			if ageHeader := respHeaders.Get(headerAge); ageHeader != "" {
				if seconds, parseErr := time.ParseDuration(ageHeader + "s"); parseErr == nil {
					ageValue = seconds
				}
			}

			// corrected_age_value = age_value + resident_time
			correctedAgeValue := ageValue + residentTime

			// age = max(apparent_age, corrected_age_value)
			age = apparentAge
			if correctedAgeValue > age {
				age = correctedAgeValue
			}

			return age, nil
		}
	}

	// If no cached time, just use time since Date header
	age = clock.since(date)

	// Add any existing Age header
	if ageHeader := respHeaders.Get(headerAge); ageHeader != "" {
		if seconds, parseErr := time.ParseDuration(ageHeader + "s"); parseErr == nil {
			age += seconds
		}
	}

	return age, nil
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
	if _, mustRevalidate := respCacheControl["must-revalidate"]; mustRevalidate {
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
// Because this is only a private cache, 'public' and 'private' in cache-control aren't
// signficant. Similarly, smax-age isn't used.
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

func canStore(reqCacheControl, respCacheControl cacheControl) (canStore bool) {
	if _, ok := respCacheControl["no-store"]; ok {
		return false
	}
	if _, ok := reqCacheControl["no-store"]; ok {
		return false
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

func parseCacheControl(headers http.Header) cacheControl {
	cc := cacheControl{}
	ccHeader := headers.Get("Cache-Control")
	for _, part := range strings.Split(ccHeader, ",") {
		part = strings.Trim(part, " ")
		if part == "" {
			continue
		}
		if strings.ContainsRune(part, '=') {
			keyval := strings.Split(part, "=")
			cc[strings.Trim(keyval[0], " ")] = strings.Trim(keyval[1], ",")
		} else {
			cc[part] = ""
		}
	}
	return cc
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
