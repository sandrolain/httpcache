// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
//
// By default, it operates as a 'private' cache (suitable for web browsers or API clients).
// It can also be configured as a 'shared/public' cache by setting IsPublicCache to true,
// which enforces stricter caching rules for multi-user scenarios (e.g., CDNs, reverse proxies).
//
// RFC 9111 (HTTP Caching) obsoletes RFC 7234 and is the current HTTP caching standard.
package httpcache

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httputil"
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
// v2.0 BREAKING CHANGE: Methods now accept context.Context for timeout/cancellation
// support and return errors for proper error propagation from cache backends.
type Cache interface {
	// Get returns the []byte representation of a cached response.
	// Returns (nil, false, nil) if the key doesn't exist.
	// Returns (nil, false, err) if there was an error retrieving the value.
	Get(ctx context.Context, key string) (responseBytes []byte, ok bool, err error)
	// Set stores the []byte representation of a response against a key.
	// Returns an error if the operation fails.
	Set(ctx context.Context, key string, responseBytes []byte) error
	// Delete removes the value associated with the key.
	// Returns an error if the operation fails.
	Delete(ctx context.Context, key string) error
}

// CachedResponse returns the cached http.Response for req if present, and nil
// otherwise. Uses the request context for cache operations.
// Note: This function does not apply key hashing or decryption as it uses the cache directly.
// For full security features (hashing, encryption), use Transport methods instead.
func CachedResponse(c Cache, req *http.Request) (resp *http.Response, err error) {
	cachedVal, ok, err := c.Get(req.Context(), cacheKey(req))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
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
	// DisableWarningHeader disables the deprecated Warning header (RFC 7234) in responses.
	// RFC 9111 has obsoleted the Warning header field, making it no longer part of the standard.
	// When true, Warning headers (110, 111, etc.) will not be added to cached responses.
	// Default is false (Warning headers are enabled for backward compatibility).
	// Set to true to comply with RFC 9111 and avoid deprecated headers.
	DisableWarningHeader bool

	// security holds the security configuration for key hashing and optional encryption.
	// This is configured via WithEncryption option.
	security *securityConfig
}

// NewTransport returns a new Transport with the
// provided Cache implementation and MarkCachedResponses set to true.
// Options can be provided to customize the Transport behavior.
// All cache keys are automatically hashed with SHA-256 before being passed to the backend.
// Use WithEncryption to enable AES-256-GCM encryption of cached data.
func NewTransport(c Cache, opts ...TransportOption) *Transport {
	t := &Transport{Cache: c, MarkCachedResponses: true}
	for _, opt := range opts {
		if err := opt(t); err != nil {
			GetLogger().Error("failed to apply transport option", "error", err)
		}
	}
	return t
}

// Client returns an *http.Client that caches responses.
func (t *Transport) Client() *http.Client {
	return &http.Client{Transport: t}
}

// cacheGet retrieves data from the cache, applying key hashing and optional decryption.
func (t *Transport) cacheGet(ctx context.Context, key string) ([]byte, bool, error) {
	hashedKey := hashKey(key)
	data, ok, err := t.Cache.Get(ctx, hashedKey)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	// Decrypt if encryption is enabled
	if t.security != nil && t.security.gcm != nil {
		plaintext, decryptErr := decrypt(t.security.gcm, data)
		if decryptErr != nil {
			GetLogger().Warn("failed to decrypt cached data", "key", hashedKey, "error", decryptErr)
			return nil, false, decryptErr
		}
		return plaintext, true, nil
	}

	return data, true, nil
}

// cacheSet stores data in the cache, applying key hashing and optional encryption.
func (t *Transport) cacheSet(ctx context.Context, key string, data []byte) error {
	hashedKey := hashKey(key)

	// Encrypt if encryption is enabled
	var toStore []byte
	if t.security != nil && t.security.gcm != nil {
		encrypted, encryptErr := encrypt(t.security.gcm, data)
		if encryptErr != nil {
			GetLogger().Warn("failed to encrypt data", "key", hashedKey, "error", encryptErr)
			return encryptErr
		}
		toStore = encrypted
	} else {
		toStore = data
	}

	return t.Cache.Set(ctx, hashedKey, toStore)
}

// cacheDelete removes data from the cache, applying key hashing.
func (t *Transport) cacheDelete(ctx context.Context, key string) error {
	hashedKey := hashKey(key)
	return t.Cache.Delete(ctx, hashedKey)
}

// cachedResponseWithKeySecure returns the cached http.Response for the given cache key if present.
// Applies key hashing and optional decryption.
func (t *Transport) cachedResponseWithKeySecure(req *http.Request, key string) (resp *http.Response, err error) {
	cachedVal, ok, err := t.cacheGet(req.Context(), key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	b := bytes.NewBuffer(cachedVal)
	return http.ReadResponse(bufio.NewReader(b), req)
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
		if !t.DisableWarningHeader && isActuallyStale(cachedResp.Header) {
			// RFC 7234 Section 5.5: Add Warning 110 (Response is Stale)
			addStaleWarning(cachedResp)
		}
		return req, true
	}

	if freshness == staleWhileRevalidate {
		// RFC 7234 Section 5.5: Add Warning 110 (Response is Stale)
		if !t.DisableWarningHeader {
			addStaleWarning(cachedResp)
		}
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

// setupCachingBody wraps the response body to cache it when fully read.
// Uses context.Background() for the cache operation since the body may be read
// after the original request context has been cancelled.
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
				if cacheErr := t.cacheSet(context.Background(), cacheKey, respBytes); cacheErr != nil {
					GetLogger().Warn("failed to cache response", "key", cacheKey, "error", cacheErr)
				}
			}
		},
	}
}

// setupCachingBodyMultiple stores the cached response under multiple cache keys when the
// response body is fully read. This is used for Vary separation where we also keep
// a manifest or pointer under the base key to allow discovery of variant keys.
// Uses context.Background() for the cache operation since the body may be read
// after the original request context has been cancelled.
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
					if cacheErr := t.cacheSet(context.Background(), k, respBytes); cacheErr != nil {
						GetLogger().Warn("failed to cache response", "key", k, "error", cacheErr)
					}
				}
			}
		},
	}
}

// storeCachedResponse caches the response immediately using the provided context.
func (t *Transport) storeCachedResponse(ctx context.Context, resp *http.Response, cacheKey string) {
	// Add cached timestamp (backward compatibility with X-Cached-Time)
	// X-Request-Time and X-Response-Time are already set by performRequest
	resp.Header.Set(XCachedTime, resp.Header.Get(XResponseTime))
	respBytes, err := httputil.DumpResponse(resp, true)
	if err == nil {
		if cacheErr := t.cacheSet(ctx, cacheKey, respBytes); cacheErr != nil {
			GetLogger().Warn("failed to cache response", "key", cacheKey, "error", cacheErr)
		}
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
		if !t.DisableWarningHeader {
			addRevalidationFailedWarning(cachedResp)
		}
		return cachedResp, nil
	}

	if err != nil || resp.StatusCode != http.StatusOK {
		if delErr := t.cacheDelete(req.Context(), cacheKey); delErr != nil {
			GetLogger().Warn("failed to delete cache entry", "key", cacheKey, "error", delErr)
		}
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

// storeResponseInCache stores the response in cache if applicable.
// Uses the request context for cache operations.
func (t *Transport) storeResponseInCache(resp *http.Response, req *http.Request, cacheKey string, cacheable bool) {
	ctx := req.Context()
	respCacheControl := parseCacheControl(resp.Header)
	reqCacheControl := parseCacheControl(req.Header)

	if !cacheable || !canStore(req, reqCacheControl, respCacheControl, t.IsPublicCache, resp.StatusCode) {
		if err := t.cacheDelete(ctx, cacheKey); err != nil {
			GetLogger().Warn("failed to delete cache entry", "key", cacheKey, "error", err)
		}
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
		if err := t.cacheDelete(ctx, cacheKey); err != nil {
			GetLogger().Warn("failed to delete cache entry", "key", cacheKey, "error", err)
		}
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
		t.storeCachedResponse(ctx, resp, varyKey)
		// Also store a copy under base key
		respCopy := *resp
		t.storeCachedResponse(ctx, &respCopy, baseKey)
		return
	}

	if req.Method == methodGET {
		t.setupCachingBody(resp, cacheKey)
	} else {
		t.storeCachedResponse(ctx, resp, cacheKey)
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
		cachedResp, err = t.cachedResponseWithKeySecure(req, cacheKey)

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
					varyCachedResp, varyErr := t.cachedResponseWithKeySecure(req, varyCacheKey)
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
		if err := t.cacheDelete(req.Context(), cacheKey); err != nil {
			GetLogger().Warn("failed to delete cache entry", "key", cacheKey, "error", err)
		}
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
