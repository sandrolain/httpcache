package httpcache

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIsSameOriginNormalizesDefaultPorts(t *testing.T) {
	tests := []struct {
		left, right string
		want        bool
	}{
		{"http://example.test/a", "http://example.test:80/b", true},
		{"https://example.test/a", "https://example.test:443/b", true},
		{"http://example.test/a", "http://example.test:8080/b", false},
		{"http://example.test/a", "https://example.test/a", false},
		{"http://EXAMPLE.test/a", "http://example.test/b", true},
	}
	for _, tt := range tests {
		left, _ := url.Parse(tt.left)
		right, _ := url.Parse(tt.right)
		if got := isSameOrigin(left, right); got != tt.want {
			t.Errorf("isSameOrigin(%q, %q) = %v, want %v", tt.left, tt.right, got, tt.want)
		}
	}
}

func TestExplicitFreshnessAnd308ResponsesAreStorable(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		cacheControl string
		expires      bool
	}{
		{name: "308 default", status: http.StatusPermanentRedirect},
		{name: "max-age", status: http.StatusTeapot, cacheControl: "max-age=60"},
		{name: "public", status: http.StatusTeapot, cacheControl: "public"},
		{name: "private", status: http.StatusTeapot, cacheControl: "private"},
		{name: "Expires", status: http.StatusTeapot, expires: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := newMockCacheTransport()
			responseHeader := make(http.Header)
			if tt.cacheControl != "" {
				responseHeader.Set("Cache-Control", tt.cacheControl)
			}
			if tt.expires {
				responseHeader.Set("Date", time.Now().UTC().Format(http.TimeFormat))
				responseHeader.Set("Expires", time.Now().UTC().Add(time.Hour).Format(http.TimeFormat))
			}
			transport.SetTransport(transportMock{response: &http.Response{
				StatusCode: tt.status,
				Header:     responseHeader,
				Body:       io.NopCloser(strings.NewReader("response")),
			}})
			req, _ := http.NewRequest(http.MethodGet, "http://example.test/explicit", nil)
			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if _, exists, err := transport.cacheGet(req.Context(), cacheKey(req)); err != nil || !exists {
				t.Fatalf("response was not stored: exists=%v err=%v", exists, err)
			}
		})
	}
}

func TestOnlyIfCachedSynthetic504IsNeverStored(t *testing.T) {
	transport := newMockCacheTransport()
	shouldCacheCalls := 0
	transport.ShouldCache = func(*http.Response) bool {
		shouldCacheCalls++
		return true
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example.test/missing", nil)
	req.Header.Set("Cache-Control", "only-if-cached")
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", resp.StatusCode)
	}
	_ = resp.Body.Close()
	if shouldCacheCalls != 0 {
		t.Fatalf("ShouldCache called %d times for synthetic 504", shouldCacheCalls)
	}
	if _, exists, err := transport.cacheGet(req.Context(), cacheKey(req)); err != nil || exists {
		t.Fatalf("synthetic 504 was stored: exists=%v err=%v", exists, err)
	}
}

func TestStaleIfErrorAllowsHEAD(t *testing.T) {
	response := &http.Response{Header: http.Header{
		"Cache-Control": {"max-age=0, stale-if-error=60"},
		"Date":          {time.Now().UTC().Format(http.TimeFormat)},
	}}
	request, _ := http.NewRequest(http.MethodHead, "http://example.test/", nil)
	if !shouldReturnStaleOnError(nil, &http.Response{StatusCode: http.StatusServiceUnavailable}, response, request, &realClock{}, slog.Default(), false) {
		t.Fatal("HEAD request did not use eligible stale-if-error response")
	}
}

func TestVaryCombinesMultipleRequestFieldLines(t *testing.T) {
	response := &http.Response{Header: make(http.Header)}
	response.Header.Set("Vary", "Accept-Language")
	storedRequest, _ := http.NewRequest(http.MethodGet, "http://example.test/", nil)
	storedRequest.Header.Add("Accept-Language", "EN")
	storedRequest.Header.Add("Accept-Language", "us")
	storeVaryHeaders(response, storedRequest)

	lookup, _ := http.NewRequest(http.MethodGet, "http://example.test/", nil)
	lookup.Header.Set("Accept-Language", "en, US")
	if !varyMatches(response, lookup) {
		t.Fatal("semantically equivalent multi-line Vary request did not match")
	}
}

func Test304MergeExcludesProxyAuthenticationInfo(t *testing.T) {
	if headers := getEndToEndHeaders(http.Header{"Proxy-Authentication-Info": {"secret"}}); len(headers) != 0 {
		t.Fatalf("Proxy-Authentication-Info included in cache metadata update: %v", headers)
	}
}
