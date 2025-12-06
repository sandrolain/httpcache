package prewarmer

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/freecache"
)

// newTestServer creates a test HTTP server that returns cacheable responses.
func newTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Set cache headers
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "text/plain")

		switch path {
		case "/error":
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "error")
		case "/slow":
			time.Sleep(100 * time.Millisecond)
			fmt.Fprint(w, "slow response")
		default:
			fmt.Fprintf(w, "response for %s", path)
		}
	}))
}

// newSitemapServer creates a test server that serves a sitemap.
func newSitemapServer(urls []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			sitemap := Sitemap{
				XMLName: xml.Name{Local: "urlset"},
				URLs:    make([]SitemapURL, len(urls)),
			}
			for i, u := range urls {
				sitemap.URLs[i] = SitemapURL{Loc: u}
			}

			w.Header().Set("Content-Type", "application/xml")
			data, _ := xml.Marshal(sitemap)
			w.Write([]byte(xml.Header))
			w.Write(data)
			return
		}

		// Default response for other paths
		w.Header().Set("Cache-Control", "max-age=3600")
		fmt.Fprintf(w, "response for %s", r.URL.Path)
	}))
}

func TestNew(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cache := freecache.New(10 * 1024 * 1024)
		transport := httpcache.NewTransport(cache)
		client := transport.Client()

		pw, err := New(Config{Client: client})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if pw == nil {
			t.Fatal("expected prewarmer, got nil")
		}
	})

	t.Run("nil client", func(t *testing.T) {
		_, err := New(Config{})
		if err == nil {
			t.Fatal("expected error for nil client")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cache := freecache.New(10 * 1024 * 1024)
		transport := httpcache.NewTransport(cache)
		client := transport.Client()

		pw, err := New(Config{
			Client:       client,
			UserAgent:    "custom-agent",
			Timeout:      5 * time.Second,
			ForceRefresh: true,
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if pw.userAgent != "custom-agent" {
			t.Errorf("expected custom-agent, got %s", pw.userAgent)
		}
		if pw.timeout != 5*time.Second {
			t.Errorf("expected 5s timeout, got %v", pw.timeout)
		}
		if !pw.forceRefresh {
			t.Error("expected forceRefresh to be true")
		}
	})
}

func TestPrewarm(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cache := freecache.New(10 * 1024 * 1024)
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	pw, err := New(Config{Client: client})
	if err != nil {
		t.Fatalf("failed to create prewarmer: %v", err)
	}

	urls := []string{
		server.URL + "/page1",
		server.URL + "/page2",
		server.URL + "/page3",
	}

	stats, err := pw.Prewarm(context.Background(), urls)
	if err != nil {
		t.Fatalf("prewarm failed: %v", err)
	}

	if stats.Total != 3 {
		t.Errorf("expected total 3, got %d", stats.Total)
	}
	if stats.Successful != 3 {
		t.Errorf("expected successful 3, got %d", stats.Successful)
	}
	if stats.Failed != 0 {
		t.Errorf("expected failed 0, got %d", stats.Failed)
	}
	if stats.TotalBytes == 0 {
		t.Error("expected TotalBytes > 0")
	}
	if stats.TotalDuration == 0 {
		t.Error("expected TotalDuration > 0")
	}
}

func TestPrewarmWithErrors(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cache := freecache.New(10 * 1024 * 1024)
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	pw, err := New(Config{Client: client})
	if err != nil {
		t.Fatalf("failed to create prewarmer: %v", err)
	}

	urls := []string{
		server.URL + "/page1",
		server.URL + "/error",
		server.URL + "/page2",
	}

	stats, err := pw.Prewarm(context.Background(), urls)
	if err != nil {
		t.Fatalf("prewarm failed: %v", err)
	}

	if stats.Total != 3 {
		t.Errorf("expected total 3, got %d", stats.Total)
	}
	if stats.Successful != 2 {
		t.Errorf("expected successful 2, got %d", stats.Successful)
	}
	if stats.Failed != 1 {
		t.Errorf("expected failed 1, got %d", stats.Failed)
	}
	if len(stats.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(stats.Errors))
	}
}

func TestPrewarmWithCallback(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cache := freecache.New(10 * 1024 * 1024)
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	pw, err := New(Config{Client: client})
	if err != nil {
		t.Fatalf("failed to create prewarmer: %v", err)
	}

	urls := []string{
		server.URL + "/page1",
		server.URL + "/page2",
	}

	var callbackCalls int
	callback := func(result *Result, completed, total int) {
		callbackCalls++
		if result.URL == "" {
			t.Error("expected URL in result")
		}
		if completed > total {
			t.Errorf("completed (%d) > total (%d)", completed, total)
		}
	}

	_, err = pw.PrewarmWithCallback(context.Background(), urls, callback)
	if err != nil {
		t.Fatalf("prewarm failed: %v", err)
	}

	if callbackCalls != 2 {
		t.Errorf("expected 2 callback calls, got %d", callbackCalls)
	}
}

func TestPrewarmConcurrent(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cache := freecache.New(10 * 1024 * 1024)
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	pw, err := New(Config{Client: client})
	if err != nil {
		t.Fatalf("failed to create prewarmer: %v", err)
	}

	urls := make([]string, 10)
	for i := 0; i < 10; i++ {
		urls[i] = fmt.Sprintf("%s/page%d", server.URL, i)
	}

	stats, err := pw.PrewarmConcurrent(context.Background(), urls, 5)
	if err != nil {
		t.Fatalf("prewarm failed: %v", err)
	}

	if stats.Total != 10 {
		t.Errorf("expected total 10, got %d", stats.Total)
	}
	if stats.Successful != 10 {
		t.Errorf("expected successful 10, got %d", stats.Successful)
	}
}

func TestPrewarmConcurrentWithCallback(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cache := freecache.New(10 * 1024 * 1024)
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	pw, err := New(Config{Client: client})
	if err != nil {
		t.Fatalf("failed to create prewarmer: %v", err)
	}

	urls := make([]string, 5)
	for i := 0; i < 5; i++ {
		urls[i] = fmt.Sprintf("%s/page%d", server.URL, i)
	}

	var callbackCount int32
	callback := func(result *Result, completed, total int) {
		atomic.AddInt32(&callbackCount, 1)
	}

	_, err = pw.PrewarmConcurrentWithCallback(context.Background(), urls, 3, callback)
	if err != nil {
		t.Fatalf("prewarm failed: %v", err)
	}

	if atomic.LoadInt32(&callbackCount) != 5 {
		t.Errorf("expected 5 callback calls, got %d", callbackCount)
	}
}

func TestPrewarmContextCancellation(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cache := freecache.New(10 * 1024 * 1024)
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	pw, err := New(Config{Client: client})
	if err != nil {
		t.Fatalf("failed to create prewarmer: %v", err)
	}

	urls := make([]string, 100)
	for i := 0; i < 100; i++ {
		urls[i] = fmt.Sprintf("%s/slow", server.URL)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	stats, err := pw.Prewarm(ctx, urls)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}

	// Should have processed fewer than all URLs
	if stats.Total == stats.Successful+stats.Failed && stats.Total == 100 {
		t.Error("expected cancellation to stop early")
	}
}

func TestPrewarmFromSitemap(t *testing.T) {
	// Create content server
	contentServer := newTestServer()
	defer contentServer.Close()

	// Create sitemap with references to content server
	urls := []string{
		contentServer.URL + "/page1",
		contentServer.URL + "/page2",
		contentServer.URL + "/page3",
	}
	sitemapServer := newSitemapServer(urls)
	defer sitemapServer.Close()

	cache := freecache.New(10 * 1024 * 1024)
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	pw, err := New(Config{Client: client})
	if err != nil {
		t.Fatalf("failed to create prewarmer: %v", err)
	}

	stats, err := pw.PrewarmFromSitemap(context.Background(), sitemapServer.URL+"/sitemap.xml")
	if err != nil {
		t.Fatalf("prewarm from sitemap failed: %v", err)
	}

	if stats.Total != 3 {
		t.Errorf("expected total 3, got %d", stats.Total)
	}
	if stats.Successful != 3 {
		t.Errorf("expected successful 3, got %d", stats.Successful)
	}
}

func TestPrewarmFromSitemapConcurrent(t *testing.T) {
	contentServer := newTestServer()
	defer contentServer.Close()

	urls := make([]string, 10)
	for i := 0; i < 10; i++ {
		urls[i] = fmt.Sprintf("%s/page%d", contentServer.URL, i)
	}
	sitemapServer := newSitemapServer(urls)
	defer sitemapServer.Close()

	cache := freecache.New(10 * 1024 * 1024)
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	pw, err := New(Config{Client: client})
	if err != nil {
		t.Fatalf("failed to create prewarmer: %v", err)
	}

	stats, err := pw.PrewarmFromSitemapConcurrent(context.Background(), sitemapServer.URL+"/sitemap.xml", 5)
	if err != nil {
		t.Fatalf("prewarm from sitemap failed: %v", err)
	}

	if stats.Total != 10 {
		t.Errorf("expected total 10, got %d", stats.Total)
	}
	if stats.Successful != 10 {
		t.Errorf("expected successful 10, got %d", stats.Successful)
	}
}

func TestPrewarmCachePopulation(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cache := freecache.New(10 * 1024 * 1024)
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	pw, err := New(Config{Client: client})
	if err != nil {
		t.Fatalf("failed to create prewarmer: %v", err)
	}

	urls := []string{
		server.URL + "/cacheable",
	}

	// First prewarm - should hit origin
	stats1, err := pw.Prewarm(context.Background(), urls)
	if err != nil {
		t.Fatalf("prewarm failed: %v", err)
	}
	if stats1.FromCache != 0 {
		t.Errorf("first request should not be from cache")
	}

	// Second prewarm - should be from cache
	stats2, err := pw.Prewarm(context.Background(), urls)
	if err != nil {
		t.Fatalf("prewarm failed: %v", err)
	}
	if stats2.FromCache != 1 {
		t.Errorf("second request should be from cache, got FromCache=%d", stats2.FromCache)
	}
}

func TestPrewarmForceRefresh(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	cache := freecache.New(10 * 1024 * 1024)
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	// First, populate cache
	pw1, _ := New(Config{Client: client})
	urls := []string{server.URL + "/page1"}
	_, _ = pw1.Prewarm(context.Background(), urls)

	// With forceRefresh, should bypass cache
	pw2, err := New(Config{
		Client:       client,
		ForceRefresh: true,
	})
	if err != nil {
		t.Fatalf("failed to create prewarmer: %v", err)
	}

	stats, err := pw2.Prewarm(context.Background(), urls)
	if err != nil {
		t.Fatalf("prewarm failed: %v", err)
	}

	// With force refresh, response should not come from cache
	if stats.FromCache != 0 {
		t.Errorf("with ForceRefresh, expected FromCache=0, got %d", stats.FromCache)
	}
}

func TestPrewarmEmptyURLs(t *testing.T) {
	cache := freecache.New(10 * 1024 * 1024)
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	pw, _ := New(Config{Client: client})

	stats, err := pw.Prewarm(context.Background(), []string{})
	if err != nil {
		t.Fatalf("prewarm failed: %v", err)
	}

	if stats.Total != 0 {
		t.Errorf("expected total 0, got %d", stats.Total)
	}
}

func TestPrewarmInvalidURL(t *testing.T) {
	cache := freecache.New(10 * 1024 * 1024)
	transport := httpcache.NewTransport(cache)
	client := transport.Client()

	pw, _ := New(Config{
		Client:  client,
		Timeout: 1 * time.Second, // Short timeout for invalid URLs
	})

	urls := []string{
		"not-a-valid-url",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stats, err := pw.Prewarm(ctx, urls)
	if err != nil {
		t.Fatalf("prewarm should not return error for invalid URLs: %v", err)
	}

	if stats.Failed != 1 {
		t.Errorf("expected 1 failure, got %d", stats.Failed)
	}
	if len(stats.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(stats.Errors))
	}
}

func TestResult(t *testing.T) {
	result := &Result{
		URL:        "http://example.com",
		Success:    true,
		StatusCode: 200,
		Duration:   100 * time.Millisecond,
		Size:       1024,
		FromCache:  true,
	}

	if result.URL != "http://example.com" {
		t.Errorf("unexpected URL: %s", result.URL)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if result.StatusCode != 200 {
		t.Errorf("unexpected status code: %d", result.StatusCode)
	}
	if result.Duration != 100*time.Millisecond {
		t.Errorf("unexpected duration: %v", result.Duration)
	}
	if result.Size != 1024 {
		t.Errorf("unexpected size: %d", result.Size)
	}
	if !result.FromCache {
		t.Error("expected from cache")
	}
}
