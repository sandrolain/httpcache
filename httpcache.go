// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC-compliant cache for http responses.
//
// It is only suitable for use as a 'private' cache (i.e. for a web-browser or an API-client
// and not for a shared proxy).
package httpcache

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"
)

const (
	stale = iota
	fresh
	transparent
	// XFromCache is the header added to responses that are returned from the cache
	XFromCache = "X-From-Cache"
	// XRevalidated is the header added to responses that got revalidated
	XRevalidated = "X-Revalidated"
	// XStale is the header added to responses that are stale
	XStale = "X-Stale"

	methodGET  = "GET"
	methodHEAD = "HEAD"

	headerXVariedPrefix = "X-Varied-"
	headerLastModified  = "last-modified"
	headerETag          = "etag"

	cacheControlOnlyIfCached = "only-if-cached"
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

// MemoryCache is an implemtation of Cache that stores responses in an in-memory map.
type MemoryCache struct {
	mu    sync.RWMutex
	items map[string][]byte
}

// Get returns the []byte representation of the response and true if present, false if not
func (c *MemoryCache) Get(key string) (resp []byte, ok bool) {
	c.mu.RLock()
	resp, ok = c.items[key]
	c.mu.RUnlock()
	return resp, ok
}

// Set saves response resp to the cache with key
func (c *MemoryCache) Set(key string, resp []byte) {
	c.mu.Lock()
	c.items[key] = resp
	c.mu.Unlock()
}

// Delete removes key from the cache
func (c *MemoryCache) Delete(key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// NewMemoryCache returns a new Cache that will store items in an in-memory map
func NewMemoryCache() *MemoryCache {
	c := &MemoryCache{items: map[string][]byte{}}
	return c
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
	if freshness == fresh {
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
			respBytes, err := httputil.DumpResponse(&resp, true)
			if err == nil {
				t.Cache.Set(cacheKey, respBytes)
			}
		},
	}
}

// storeCachedResponse caches the response immediately
func (t *Transport) storeCachedResponse(resp *http.Response, cacheKey string) {
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
				GetLogger().Debug("error draining 304 response body", "error", drainErr)
			}
		}
		return handleNotModifiedResponse(cachedResp, resp, t.MarkCachedResponses), nil
	}

	if shouldReturnStaleOnError(err, resp, cachedResp, req) {
		// Drain and close the error response body since we're using the cached response
		if resp != nil {
			if drainErr := drainDiscardedBody(resp.Body); drainErr != nil {
				GetLogger().Debug("error draining stale response body", "error", drainErr)
			}
		}
		if t.MarkCachedResponses {
			cachedResp.Header.Set(XStale, "1")
		}
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

	// Store response in cache if applicable
	t.storeResponseInCache(resp, req, cacheKey, cacheable)

	return resp, nil
} // ErrNoDateHeader indicates that the HTTP headers contained no Date header.
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

// checkCacheControl checks for no-cache directives and only-if-cached
func checkCacheControl(respCacheControl, reqCacheControl cacheControl) (int, bool) {
	if _, ok := reqCacheControl["no-cache"]; ok {
		return transparent, true
	}
	if _, ok := respCacheControl["no-cache"]; ok {
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
	if maxAge, ok := respCacheControl["max-age"]; ok {
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
func adjustAgeForRequestControls(reqCacheControl cacheControl, currentAge time.Duration, lifetime time.Duration) (time.Duration, time.Duration, bool) {
	if maxAge, ok := reqCacheControl["max-age"]; ok {
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

	// Check cache control directives
	if result, done := checkCacheControl(respCacheControl, reqCacheControl); done {
		return result
	}

	date, err := Date(respHeaders)
	if err != nil {
		return stale
	}
	currentAge := clock.since(date)

	// Calculate response lifetime
	lifetime := calculateLifetime(respCacheControl, respHeaders, date)

	// Adjust age based on request controls
	var returnFresh bool
	currentAge, lifetime, returnFresh = adjustAgeForRequestControls(reqCacheControl, currentAge, lifetime)
	if returnFresh {
		return fresh
	}

	if lifetime > currentAge {
		return fresh
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
