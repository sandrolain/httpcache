package httpcache

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCacheKeyScopesRequestHostOverride(t *testing.T) {
	baseReq, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	implicit, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	equalOverride, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	equalOverride.Host = "EXAMPLE.com:80"
	otherHost, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	otherHost.Host = "tenant.example.net"

	baseKey := cacheKey(baseReq)
	if got := cacheKey(implicit); got != baseKey {
		t.Fatalf("implicit host key = %q, want %q", got, baseKey)
	}
	if got := cacheKey(equalOverride); got != baseKey {
		t.Fatalf("equivalent Host override changed key: got %q, want %q", got, baseKey)
	}
	if got := cacheKey(otherHost); got == baseKey {
		t.Fatal("different Host override must produce a distinct cache key")
	}

	transport := NewTransport(newMockCache(), WithCacheKeyHeaders([]string{"Host"}))
	key, _ := transport.getCacheKey(otherHost)
	if key == baseKey {
		t.Fatal("CacheKeyHeaders Host must include the effective Request.Host")
	}
}

func TestRoundTripDoesNotReuseAcrossHostOverrides(t *testing.T) {
	var calls int
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = io.WriteString(w, r.Host)
	}))
	defer origin.Close()

	transport := NewTransport(newMockCache())
	request := func(host string) string {
		req, err := http.NewRequest(http.MethodGet, origin.URL+"/item", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = host
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}

	if got := request("tenant-a.example"); got != "tenant-a.example" {
		t.Fatalf("first response = %q", got)
	}
	if got := request("tenant-b.example"); got != "tenant-b.example" {
		t.Fatalf("second response = %q; host variant may have been served from cache", got)
	}
	if calls != 2 {
		t.Fatalf("origin calls = %d, want 2", calls)
	}
}

func TestUnsafeRequestInvalidatesHostScopedEntries(t *testing.T) {
	cache := newMockCache()
	transport := NewTransport(cache, WithVarySeparation(true))
	url := "http://example.com/item"

	get, _ := http.NewRequest(http.MethodGet, url, nil)
	get.Host = "tenant.example"
	head, _ := http.NewRequest(http.MethodHead, url, nil)
	head.Host = "tenant.example"
	getKey, headKey := cacheKey(get), cacheKey(head)
	cache.items[hashKey(getKey)] = []byte("get")
	cache.items[hashKey(headKey)] = []byte("head")
	defaultHost, _ := http.NewRequest(http.MethodGet, url, nil)
	defaultKey := cacheKey(defaultHost)
	cache.items[hashKey(defaultKey)] = []byte("default host")

	unsafe, _ := http.NewRequest(http.MethodPut, url, nil)
	unsafe.Host = "tenant.example"
	transport.invalidateCache(unsafe, &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header)})

	if _, ok := cache.items[hashKey(getKey)]; ok {
		t.Fatal("host-scoped GET entry was not invalidated")
	}
	if _, ok := cache.items[hashKey(headKey)]; ok {
		t.Fatal("host-scoped HEAD entry was not invalidated")
	}
	if _, ok := cache.items[hashKey(defaultKey)]; !ok {
		t.Fatal("invalidating one Host override deleted the default-host entry")
	}
	if _, ok := cache.items[hashKey(invalidationMarkerKey(get.URL))]; !ok {
		t.Fatal("URI invalidation marker for host-scoped variants was not stored")
	}
	if !transport.invalidatedSinceRequest(context.Background(), get.URL, http.Header{
		XCachedTime: []string{time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)},
	}, get.Host) {
		t.Fatal("host-scoped invalidation marker was not consulted")
	}
}

func TestHostOverrideScopeIsNotAddedForEquivalentAuthority(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://EXAMPLE.com/item", nil)
	req.Host = "example.com:443"
	if got := hostOverrideKey(req); got != "" {
		t.Fatalf("equivalent authority scope = %q, want empty", got)
	}
	if strings.Contains(cacheKey(req), "|host:") {
		t.Fatal("equivalent authority should not add host key scope")
	}
}

func TestHostOverrideScopePreservesIPv6ZoneCase(t *testing.T) {
	newRequest := func(host string) *http.Request {
		req, err := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = host
		return req
	}

	lowerZone := newRequest("[fe80::1%en0]")
	upperZone := newRequest("[fe80::1%EN0]")
	upperAddress := newRequest("[FE80::1%en0]")
	if cacheKey(lowerZone) == cacheKey(upperZone) {
		t.Fatal("case-distinct IPv6 zone identifiers must have distinct cache scopes")
	}
	if cacheKey(lowerZone) != cacheKey(upperAddress) {
		t.Fatal("IPv6 address hex case should not change the cache scope")
	}
}

func TestCacheKeyHeadersDistinguishEmptyAndFieldLineValues(t *testing.T) {
	cacheHeaders := []string{"X-User-ID"}
	absent, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	empty, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	empty.Header["X-User-Id"] = []string{""}
	multiple, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	multiple.Header["X-User-Id"] = []string{"a", "b"}
	combined, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	combined.Header.Set("X-User-ID", "a, b")
	differentLines, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	differentLines.Header["X-User-Id"] = []string{"ab", "c"}

	if cacheKeyWithHeaders(absent, cacheHeaders) == cacheKeyWithHeaders(empty, cacheHeaders) {
		t.Fatal("an explicit empty CacheKeyHeaders value must differ from absence")
	}
	if cacheKeyWithHeaders(multiple, cacheHeaders) == cacheKeyWithHeaders(combined, cacheHeaders) {
		t.Fatal("multiple field lines must differ from one comma-containing value")
	}
	if cacheKeyWithHeaders(multiple, cacheHeaders) == cacheKeyWithHeaders(differentLines, cacheHeaders) {
		t.Fatal("different CacheKeyHeaders field-line sequences must have distinct keys")
	}

	ordinary, _ := http.NewRequest(http.MethodGet, "http://example.com/item", nil)
	ordinary.Header.Set("X-User-ID", "user-1")
	canonical := http.CanonicalHeaderKey("X-User-ID")
	want := cacheKey(ordinary) + "|" + canonical + ":" + url.QueryEscape("user-1")
	if got := cacheKeyWithHeaders(ordinary, cacheHeaders); got != want {
		t.Fatalf("ordinary single-value key changed: got %q, want %q", got, want)
	}
}
