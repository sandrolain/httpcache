// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"net/http"
)

// addWarningHeader adds a Warning header to the response per RFC 7234 Section 5.5
// Warning headers can be stacked, so we use Add instead of Set
// Note: RFC 9111 has obsoleted the Warning header field.
func addWarningHeader(resp *http.Response, warningCode string) {
	resp.Header.Add(headerWarning, warningCode)
}

// addStaleWarning adds "110 Response is Stale" warning header
// Note: RFC 9111 has obsoleted the Warning header field.
func addStaleWarning(resp *http.Response) {
	addWarningHeader(resp, warningResponseIsStale)
}

// addRevalidationFailedWarning adds "111 Revalidation Failed" warning header
// Note: RFC 9111 has obsoleted the Warning header field.
func addRevalidationFailedWarning(resp *http.Response) {
	addWarningHeader(resp, warningRevalidationFailed)
}
