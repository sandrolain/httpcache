// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// isUnsafeMethod returns true if the HTTP method is considered unsafe
// RFC 9110 Section 9.2.1: all methods other than GET, HEAD, OPTIONS, and TRACE
// are unsafe, including methods whose safety is unknown.
func isUnsafeMethod(method string) bool {
	switch method {
	case methodGET, methodHEAD, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

// invalidateCache invalidates cache entries per RFC 9111 Section 4.4
// When receiving a non-error response to an unsafe method, invalidate:
// 1. The effective Request-URI
// 2. URIs in Location and Content-Location response headers (if present and same-origin)
//
// RFC 9111 restricts invalidation to same-origin URIs for security.
func (t *Transport) invalidateCache(req *http.Request, resp *http.Response) {
	ctx := req.Context()

	// RFC 9111 Section 4.4: Only invalidate on non-error responses
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		t.log().Debug("skipping cache invalidation for error response",
			"status", resp.StatusCode,
			"url", req.URL.String())
		return
	}

	// Always invalidate the Request-URI
	t.invalidateURI(ctx, req.URL, "request-uri")

	// Invalidate Location header URI (RFC 9111 Section 4.4)
	if location := resp.Header.Get(headerLocation); location != "" {
		if err := t.invalidateHeaderURI(ctx, req.URL, location, "Location"); err != nil {
			t.log().Debug("failed to invalidate Location URI",
				"location", location,
				"error", err.Error())
		}
	}

	// Invalidate Content-Location header URI (RFC 9111 Section 4.4)
	if contentLocation := resp.Header.Get(headerContentLocation); contentLocation != "" {
		if err := t.invalidateHeaderURI(ctx, req.URL, contentLocation, "Content-Location"); err != nil {
			t.log().Debug("failed to invalidate Content-Location URI",
				"content-location", contentLocation,
				"error", err.Error())
		}
	}
}

// invalidateHeaderURI parses and invalidates a URI from a response header.
// It ensures same-origin policy compliance per RFC 9111.
// Returns an error if the URI cannot be parsed.
func (t *Transport) invalidateHeaderURI(ctx context.Context, requestURL *url.URL, headerValue string, headerName string) error {
	// Parse the header value as a URI (may be relative or absolute)
	targetURL, err := requestURL.Parse(headerValue)
	if err != nil {
		return err
	}

	// RFC 9111 Section 4.4: Only invalidate same-origin URIs
	// Origin = scheme + host (host includes port if present)
	if !isSameOrigin(requestURL, targetURL) {
		t.log().Debug("skipping cross-origin invalidation",
			"header", headerName,
			"request-origin", getOrigin(requestURL),
			"target-origin", getOrigin(targetURL))
		return nil
	}

	t.invalidateURI(ctx, targetURL, headerName)
	return nil
}

// invalidateURI removes cache entries for the given URI.
// It invalidates both GET and HEAD requests for the URI.
func (t *Transport) invalidateURI(ctx context.Context, targetURL *url.URL, source string) {
	// CacheKeyHeaders and Vary separation append data to cache keys, so deleting
	// the base GET/HEAD keys cannot remove every stored representation. Persist a
	// URI-level request-time marker that forces older variants through validation.
	if len(t.CacheKeyHeaders) > 0 || t.EnableVarySeparation {
		key := invalidationMarkerKey(targetURL)
		marker := []byte(time.Now().UTC().Format(time.RFC3339Nano))
		if err := t.cacheSet(ctx, key, marker); err != nil {
			t.log().Warn("failed to store URI invalidation marker", "key", key, "error", err)
		}
	}

	// Invalidate GET request for this URL
	getReq := &http.Request{
		Method: methodGET,
		URL:    targetURL,
	}
	getKey := cacheKey(getReq)
	if err := t.cacheDelete(ctx, getKey); err != nil {
		t.log().Warn("failed to invalidate cache entry", "key", getKey, "error", err)
	} else {
		t.log().Debug("invalidated cache entry",
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
		if err := t.cacheDelete(ctx, headKey); err != nil {
			t.log().Warn("failed to invalidate HEAD cache entry", "key", headKey, "error", err)
		} else {
			t.log().Debug("invalidated HEAD cache entry",
				"key", headKey,
				"source", source)
		}
	}
}

const invalidationMarkerPrefix = "httpcache-invalidation:"

func invalidationMarkerKey(targetURL *url.URL) string {
	return invalidationMarkerPrefix + targetURL.String()
}

// invalidatedSinceRequest reports whether a stored response predates the last
// successful unsafe request for its URI. Responses without request-time
// metadata are conservatively treated as invalidated.
func (t *Transport) invalidatedSinceRequest(ctx context.Context, targetURL *url.URL, headers http.Header) bool {
	if len(t.CacheKeyHeaders) == 0 && !t.EnableVarySeparation {
		return false
	}
	data, ok, err := t.cacheGet(ctx, invalidationMarkerKey(targetURL))
	if err != nil {
		t.log().Warn("failed to read URI invalidation marker", "url", targetURL.String(), "error", err)
		return true
	}
	if !ok {
		return false
	}
	marker, err := time.Parse(time.RFC3339Nano, string(data))
	if err != nil {
		t.log().Warn("invalid URI invalidation marker", "url", targetURL.String(), "error", err)
		return true
	}
	requestTime, err := time.Parse(time.RFC3339Nano, headers.Get(XRequestTime))
	if err != nil {
		requestTime, err = time.Parse(time.RFC3339Nano, headers.Get(XCachedTime))
	}
	if err != nil {
		return true
	}
	return !requestTime.After(marker)
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
