package httpcache

import (
	"net/http"
	"testing"
	"time"
)

// TestFreshnessFutureDateClockSkew verifies a future Date header (clock skew) on a response
// with no freshness info is stale: RFC 9111 §4.2.3 clamps apparent_age to max(0, ...).
func TestFreshnessFutureDateClockSkew(t *testing.T) {
	resetTest()

	respHeaders := http.Header{}
	// Date 2s in the future, no Cache-Control and no Expires (lifetime 0).
	respHeaders.Set("Date", time.Now().Add(2*time.Second).UTC().Format(time.RFC1123))

	if got := getFreshness(respHeaders, http.Header{}); got == fresh {
		t.Fatalf("future Date with no freshness info treated as fresh; want stale (RFC 9111 §4.2.3); got %s", freshnessString(got))
	}
}
