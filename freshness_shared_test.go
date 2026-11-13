package httpcache

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSharedCacheSMaxAgeOverridesMaxAge(t *testing.T) {
	date := time.Now().UTC().Truncate(time.Second)
	response := http.Header{"Cache-Control": {"max-age=86400, s-maxage=60"}, "Date": {date.Format(time.RFC1123)}}
	clock := &fakeClock{elapsed: 90 * time.Second}
	if got := getFreshness(response, http.Header{}, clock, slog.Default(), true); got != stale {
		t.Fatalf("shared freshness = %v, want stale", got)
	}
	if got := getFreshness(response, http.Header{}, clock, slog.Default(), false); got != fresh {
		t.Fatalf("private freshness = %v, want fresh", got)
	}
}

func TestSharedCacheDirectivesProhibitStaleReuse(t *testing.T) {
	date := time.Now().UTC().Truncate(time.Second)
	for _, directive := range []string{"proxy-revalidate", "s-maxage=10"} {
		t.Run(directive, func(t *testing.T) {
			h := http.Header{"Cache-Control": {"max-age=10, stale-while-revalidate=100, stale-if-error=100, " + directive}, "Date": {date.Format(time.RFC1123)}}
			clock := &fakeClock{elapsed: 15 * time.Second}
			if got := getFreshness(h, http.Header{}, clock, slog.Default(), true); got != stale {
				t.Errorf("freshness = %v, want stale", got)
			}
			if canStaleOnError(h, http.Header{}, clock, slog.Default(), true) {
				t.Error("canStaleOnError = true, want false")
			}
		})
	}
}

func TestMustRevalidateProhibitsStaleReuse(t *testing.T) {
	date := time.Now().UTC().Truncate(time.Second)
	h := http.Header{"Cache-Control": {"max-age=10, must-revalidate, stale-while-revalidate=100, stale-if-error=100"}, "Date": {date.Format(time.RFC1123)}}
	clock := &fakeClock{elapsed: 15 * time.Second}
	if got := getFreshness(h, http.Header{}, clock, slog.Default(), false); got != stale {
		t.Errorf("freshness = %v, want stale", got)
	}
	if canStaleOnError(h, http.Header{}, clock, slog.Default(), false) {
		t.Error("canStaleOnError = true, want false")
	}
	request := http.Header{"Cache-Control": {"stale-if-error=100"}}
	if canStaleOnError(http.Header{"Cache-Control": {"max-age=10, must-revalidate"}, "Date": {date.Format(time.RFC1123)}}, request, clock, slog.Default(), false) {
		t.Error("request stale-if-error bypassed must-revalidate")
	}
}

func TestSharedCacheSMaxAgeRevalidatesEndToEnd(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Cache-Control", "max-age=3600, s-maxage=1")
		_, _ = io.WriteString(w, "body")
	}))
	defer server.Close()

	clock := &fakeClock{}
	transport := newMockCacheTransportWithClock(clock)
	transport.IsPublicCache = true
	client := &http.Client{Transport: transport}
	request := func() *http.Response {
		t.Helper()
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp
	}

	request()
	clock.elapsed = 2 * time.Second
	request()
	if got := requests.Load(); got != 2 {
		t.Fatalf("origin requests = %d, want 2 after shared s-maxage expires", got)
	}
}

func TestMustRevalidateOnlyIfCachedReturnsGatewayTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=1, must-revalidate")
		_, _ = io.WriteString(w, "body")
	}))
	defer server.Close()

	clock := &fakeClock{}
	transport := newMockCacheTransportWithClock(clock)
	client := &http.Client{Transport: transport}
	first, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, first.Body)
	_ = first.Body.Close()

	clock.elapsed = 2 * time.Second
	request, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cache-Control", "only-if-cached")
	resp, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 when stale must-revalidate entry cannot be used", resp.StatusCode)
	}
}

func TestMustRevalidateDoesNotServeStaleOnOriginError(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Cache-Control", "max-age=1, must-revalidate, stale-if-error=100")
			_, _ = io.WriteString(w, "cached body")
			return
		}
		http.Error(w, "origin unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	clock := &fakeClock{}
	transport := newMockCacheTransportWithClock(clock)
	client := &http.Client{Transport: transport}
	first, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, first.Body)
	_ = first.Body.Close()

	clock.elapsed = 2 * time.Second
	second, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Body.Close()
	if second.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want origin 503 instead of stale cached response", second.StatusCode)
	}
	body, err := io.ReadAll(second.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) == "cached body" {
		t.Fatal("served stale cached body despite must-revalidate")
	}
}
