package httpcache

import (
	"log/slog"
	"net/http"
	"testing"
)

func Test304MergePreservesContentLengthAndDropsOneXXWarnings(t *testing.T) {
	cached := &http.Response{Header: http.Header{
		"Content-Length": {"128"},
		"Warning":        {`110 cache "stale, cached response", 299 cache "retained warning"`},
	}}
	revalidated := &http.Response{StatusCode: http.StatusNotModified, Header: http.Header{
		"Content-Length": {"8"},
		"Warning":        {`111 cache "revalidation failed", 214 cache "transformation applied"`},
	}}
	got := handleNotModifiedResponse(cached, revalidated, false, &realClock{}, slog.Default())
	if got.Header.Get("Content-Length") != "128" {
		t.Fatalf("304 Content-Length replaced the stored representation length: got %q", got.Header.Get("Content-Length"))
	}
	warnings := got.Header.Values("Warning")
	if len(warnings) != 1 || warnings[0] != `214 cache "transformation applied"` {
		t.Fatalf("Warning values after validation = %#v, want only the 2xx warning", warnings)
	}
}
