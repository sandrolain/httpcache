package httpcache

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRequestMaxAgeCapsResponseFreshness(t *testing.T) {
	clock := &fakeClock{elapsed: 90 * time.Second}
	date := time.Now().UTC().Format(http.TimeFormat)
	resp := http.Header{"Cache-Control": {"max-age=60"}, "Date": {date}}

	if got := getFreshness(resp, http.Header{"Cache-Control": {"max-age=120"}}, clock, slog.Default(), false); got != stale {
		t.Fatalf("request max-age extended response freshness: got %v, want stale", got)
	}
	resp.Set("Cache-Control", "max-age=3600, stale-while-revalidate=300")
	if got := getFreshness(resp, http.Header{"Cache-Control": {"max-age=60"}}, clock, slog.Default(), false); got != stale {
		t.Fatalf("request max-age cap was bypassed by stale-while-revalidate: got %v, want stale", got)
	}
}

func TestRequestNoStorePreservesRevalidatedEntry(t *testing.T) {
	resetTest()
	upstream := &transportMock{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": {"max-age=0"}, "Date": {time.Now().UTC().Format(http.TimeFormat)}, "ETag": {`"entry"`}},
		Body:       io.NopCloser(strings.NewReader("cached body")),
	}}
	transport := newMockCacheTransport()
	transport.SetTransport(upstream)
	req, _ := http.NewRequest(http.MethodGet, "http://example.test/no-store", nil)
	first, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(first.Body); err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	key := cacheKeyWithHeaders(req, nil)
	if _, exists, err := transport.cacheGet(req.Context(), key); err != nil || !exists {
		t.Fatalf("initial response was not cached: exists=%v err=%v", exists, err)
	}

	upstream.response = &http.Response{
		StatusCode: http.StatusNotModified,
		Header:     http.Header{"Cache-Control": {"max-age=3600"}, "ETag": {`"entry"`}},
		Body:       io.NopCloser(strings.NewReader("")),
	}
	secondReq := req.Clone(req.Context())
	secondReq.Header.Set("Cache-Control", "no-store")
	second, err := transport.RoundTrip(secondReq)
	if err != nil {
		t.Fatal(err)
	}
	second.Body.Close()

	if _, exists, err := transport.cacheGet(req.Context(), key); err != nil || !exists {
		t.Fatalf("request no-store deleted the existing cache entry: exists=%v err=%v", exists, err)
	}
}

func TestStaleIfErrorWindowStartsAtExpiration(t *testing.T) {
	clock := &fakeClock{elapsed: 15 * time.Second}
	resp := http.Header{"Cache-Control": {"max-age=10, stale-if-error=10"}, "Date": {time.Now().UTC().Format(http.TimeFormat)}}
	if !canStaleOnError(resp, nil, clock, slog.Default(), false) {
		t.Fatal("expected response to be eligible 5 seconds into the stale-if-error window")
	}
	clock.elapsed = 21 * time.Second
	if canStaleOnError(resp, nil, clock, slog.Default(), false) {
		t.Fatal("response outside stale-if-error window was accepted")
	}
}

func TestStaleIfErrorLimitsStatusesAndVary(t *testing.T) {
	date := time.Now().UTC().Add(-time.Hour).UTC().Format(http.TimeFormat)
	cached := &http.Response{Header: http.Header{
		"Cache-Control":            {"max-age=0, stale-if-error=3700"},
		"Date":                     {date},
		"Vary":                     {"Accept-Language"},
		"X-Varied-Accept-Language": {"en"},
	}}
	req, _ := http.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.Header.Set("Accept-Language", "fr")
	if shouldReturnStaleOnError(nil, &http.Response{StatusCode: http.StatusNotImplemented}, cached, req, &realClock{}, slog.Default(), false) {
		t.Fatal("stale-if-error accepted a 501 response")
	}
	if shouldReturnStaleOnError(errors.New("origin unavailable"), nil, cached, req, &realClock{}, slog.Default(), false) {
		t.Fatal("stale-if-error served a mismatched Vary variant")
	}
	req.Header.Set("Accept-Language", "en")
	if !shouldReturnStaleOnError(nil, &http.Response{StatusCode: http.StatusBadGateway}, cached, req, &realClock{}, slog.Default(), false) {
		t.Fatal("stale-if-error rejected an eligible 502 response")
	}
}
