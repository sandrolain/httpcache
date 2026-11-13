// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"net/http"
	"strconv"
	"strings"
)

// removeObsoleteWarnings removes Warning values with a 1xx warn-code after
// successful validation, as required by RFC 9111 §4.3.4.
func removeObsoleteWarnings(header http.Header) {
	var retained []string
	for _, fieldValue := range header.Values(headerWarning) {
		for _, value := range splitWarningValues(fieldValue) {
			value = strings.TrimSpace(value)
			code, _, found := strings.Cut(value, " ")
			if found && len(code) == 3 {
				if n, err := strconv.Atoi(code); err == nil && n >= 100 && n < 200 {
					continue
				}
			}
			retained = append(retained, value)
		}
	}
	if len(retained) == 0 {
		header.Del(headerWarning)
		return
	}
	header[http.CanonicalHeaderKey(headerWarning)] = retained
}

func splitWarningValues(value string) []string {
	var values []string
	start := 0
	quoted := false
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '"':
			if i == 0 || value[i-1] != '\\' {
				quoted = !quoted
			}
		case ',':
			if !quoted {
				values = append(values, value[start:i])
				start = i + 1
			}
		}
	}
	return append(values, value[start:])
}

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
