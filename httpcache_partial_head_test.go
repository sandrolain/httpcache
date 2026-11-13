package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPartialContentIsNeverStored(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Range", "bytes 0-1/4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "ab")
	}))
	defer server.Close()

	transport := newMockCacheTransport()
	transport.ShouldCache = func(*http.Response) bool { return true }
	client := &http.Client{Transport: transport}
	for i := 0; i < 2; i++ {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusPartialContent {
			t.Fatalf("status = %d, want 206", resp.StatusCode)
		}
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("origin requests = %d, want 2 because 206 is not cacheable", got)
	}
}

func TestHead304ReusesStoredHeadResponse(t *testing.T) {
	var clock fakeClock
	var requests atomic.Int32
	const etag = `"head-v1"`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Cache-Control", "max-age=1")
		w.Header().Set("Content-Length", "8")
	}))
	defer server.Close()

	transport := newMockCacheTransportWithClock(&clock)
	transport.MarkCachedResponses = true
	client := &http.Client{Transport: transport}
	request, err := http.NewRequest(http.MethodHead, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Body.Close()
	clock.elapsed = 2 * time.Second

	second, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Body.Close()
	if second.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want synthesized 200 response", second.StatusCode)
	}
	if second.Header.Get(XRevalidated) != "1" {
		t.Fatal("HEAD response was not marked revalidated")
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("origin requests = %d, want 2", got)
	}
}
