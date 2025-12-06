package httpcache

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"strconv"
	"testing"
	"time"
)

var s struct {
	server    *httptest.Server
	client    http.Client
	transport *Transport
	done      chan struct{} // Closed to unlock infinite handlers.
}

type fakeClock struct {
	elapsed time.Duration
}

func (c *fakeClock) since(t time.Time) time.Duration {
	return c.elapsed
}

func TestMain(m *testing.M) {
	flag.Parse()
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}

func setup() {
	tp := newMockCacheTransport()
	client := http.Client{Transport: tp}
	s.transport = tp
	s.client = client
	s.done = make(chan struct{})

	mux := http.NewServeMux()
	s.server = httptest.NewServer(mux)

	mux.HandleFunc("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
	}))

	mux.HandleFunc("/method", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write([]byte(r.Method))
	}))

	mux.HandleFunc("/range", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lm := "Fri, 14 Dec 2010 01:01:50 GMT"
		if r.Header.Get("if-modified-since") == lm {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("last-modified", lm)
		if r.Header.Get("range") == "bytes=4-9" {
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte(" text "))
			return
		}
		_, _ = w.Write([]byte("Some text content"))
	}))

	mux.HandleFunc("/nostore", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
	}))

	staleWhileRevalidateCounter := 0
	mux.HandleFunc("/stale-while-revalidate", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		staleWhileRevalidateCounter++
		w.Header().Set("X-Counter", strconv.Itoa(staleWhileRevalidateCounter))
		w.Header().Set("Cache-Control", "max-age=100, stale-while-revalidate=100")
	}))

	mux.HandleFunc("/etag", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		etag := "124567"
		if r.Header.Get("if-none-match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("etag", etag)
	}))

	mux.HandleFunc("/lastmodified", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lm := "Fri, 14 Dec 2010 01:01:50 GMT"
		if r.Header.Get("if-modified-since") == lm {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("last-modified", lm)
	}))

	mux.HandleFunc("/varyaccept", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Vary", "Accept")
		w.Write([]byte("Some text content"))
	}))

	mux.HandleFunc("/doublevary", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Vary", "Accept, Accept-Language")
		w.Write([]byte("Some text content"))
	}))
	mux.HandleFunc("/2varyheaders", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Add("Vary", "Accept")
		w.Header().Add("Vary", "Accept-Language")
		w.Write([]byte("Some text content"))
	}))
	mux.HandleFunc("/varyunused", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Vary", "X-Madeup-Header")
		w.Write([]byte("Some text content"))
	}))

	mux.HandleFunc("/cachederror", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		etag := "abc"
		if r.Header.Get("if-none-match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("etag", etag)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not found"))
	}))

	mux.HandleFunc("/notfound", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not found"))
	}))

	mux.HandleFunc("/redirect", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Location", "http://example.com/target")
		w.WriteHeader(http.StatusMovedPermanently)
	}))

	mux.HandleFunc("/badrequest", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	}))

	updateFieldsCounter := 0
	mux.HandleFunc("/updatefields", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Counter", strconv.Itoa(updateFieldsCounter))
		w.Header().Set("Etag", `"e"`)
		updateFieldsCounter++
		if r.Header.Get("if-none-match") != "" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Write([]byte("Some text content"))
	}))

	// Take 3 seconds to return 200 OK (for testing client timeouts).
	mux.HandleFunc("/3seconds", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
	}))

	mux.HandleFunc("/infinite", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for {
			select {
			case <-s.done:
				return
			default:
				w.Write([]byte{0})
			}
		}
	}))

	mux.HandleFunc("/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "application/json")
		// This will force using bufio.Read() instead of chunkedReader.Read()
		// to miss the EOF.
		w.Header().Set("Transfer-encoding", "identity")
		json.NewEncoder(w).Encode(map[string]string{"k": "v"})
	}))

	serverErrorCounter := 0
	mux.HandleFunc("/servererror", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverErrorCounter++
		if serverErrorCounter == 1 {
			// First request: return 200 OK with cache headers
			w.Header().Set("Cache-Control", "max-age=3600")
			w.Header().Set("Etag", "error-etag")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK response"))
		} else {
			// Subsequent requests: return 500 error
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Server error"))
		}
	}))
}

func teardown() {
	close(s.done)
	s.server.Close()
}

func resetTest() {
	s.transport.Cache = newMockCache()
	clock = &realClock{}
}

// TestCacheableMethod ensures that uncacheable method does not get stored
// in cache and get incorrectly used for a following cacheable method request.
func TestCacheableMethod(t *testing.T) {
	resetTest()
	{
		req, err := http.NewRequest("POST", s.server.URL+"/method", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		_, err = io.Copy(&buf, resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		err = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := buf.String(), "POST"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("response status code isn't 200 OK: %v", resp.StatusCode)
		}
	}
	{
		req, err := http.NewRequest(methodGET, s.server.URL+"/method", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		_, err = io.Copy(&buf, resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		err = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := buf.String(), methodGET; got != want {
			t.Errorf("got wrong body %q, want %q", got, want)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("response status code isn't 200 OK: %v", resp.StatusCode)
		}
		if resp.Header.Get(XFromCache) != "" {
			t.Errorf("XFromCache header isn't blank")
		}
	}
}

func TestDontServeHeadResponseToGetRequest(t *testing.T) {
	resetTest()
	url := s.server.URL + "/"
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	req, err = http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Header.Get(XFromCache) != "" {
		t.Errorf("Cache should not match")
	}
}

func TestDontStorePartialRangeInCache(t *testing.T) {
	resetTest()
	{
		req, err := http.NewRequest(methodGET, s.server.URL+"/range", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("range", "bytes=4-9")
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		_, err = io.Copy(&buf, resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		err = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := buf.String(), " text "; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if resp.StatusCode != http.StatusPartialContent {
			t.Errorf("response status code isn't 206 Partial Content: %v", resp.StatusCode)
		}
	}
	{
		req, err := http.NewRequest(methodGET, s.server.URL+"/range", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		_, err = io.Copy(&buf, resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		err = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := buf.String(), "Some text content"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("response status code isn't 200 OK: %v", resp.StatusCode)
		}
		if resp.Header.Get(XFromCache) != "" {
			t.Error("XFromCache header isn't blank")
		}
	}
	{
		req, err := http.NewRequest(methodGET, s.server.URL+"/range", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		_, err = io.Copy(&buf, resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		err = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := buf.String(), "Some text content"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("response status code isn't 200 OK: %v", resp.StatusCode)
		}
		if resp.Header.Get(XFromCache) != "1" {
			t.Errorf(`XFromCache header isn't "1": %v`, resp.Header.Get(XFromCache))
		}
	}
	{
		req, err := http.NewRequest(methodGET, s.server.URL+"/range", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("range", "bytes=4-9")
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		_, err = io.Copy(&buf, resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		err = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := buf.String(), " text "; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if resp.StatusCode != http.StatusPartialContent {
			t.Errorf("response status code isn't 206 Partial Content: %v", resp.StatusCode)
		}
	}
}

func TestCacheOnlyIfBodyRead(t *testing.T) {
	resetTest()
	{
		req, err := http.NewRequest(methodGET, s.server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
		// We do not read the body
		resp.Body.Close()
	}
	{
		req, err := http.NewRequest(methodGET, s.server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatalf("XFromCache header isn't blank")
		}
	}
}

func TestOnlyReadBodyOnDemand(t *testing.T) {
	resetTest()

	req, err := http.NewRequest(methodGET, s.server.URL+"/infinite", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.client.Do(req) // This shouldn't hang forever.
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 10) // Only partially read the body.
	_, err = resp.Body.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

func TestGetOnlyIfCachedHit(t *testing.T) {
	resetTest()
	{
		req, err := http.NewRequest(methodGET, s.server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
		_, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
	}
	{
		req, err := http.NewRequest(methodGET, s.server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Add("cache-control", "only-if-cached")
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "1" {
			t.Fatalf(`XFromCache header isn't "1": %v`, resp.Header.Get(XFromCache))
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("response status code isn't 200 OK: %v", resp.StatusCode)
		}
	}
}

func TestGetOnlyIfCachedMiss(t *testing.T) {
	resetTest()
	req, err := http.NewRequest(methodGET, s.server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add("cache-control", "only-if-cached")
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Header.Get(XFromCache) != "" {
		t.Fatal("XFromCache header isn't blank")
	}
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("response status code isn't 504 GatewayTimeout: %v", resp.StatusCode)
	}
}

func TestGetNoStoreRequest(t *testing.T) {
	resetTest()
	req, err := http.NewRequest(methodGET, s.server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add("Cache-Control", "no-store")
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
	}
}

func TestGetNoStoreResponse(t *testing.T) {
	resetTest()
	req, err := http.NewRequest(methodGET, s.server.URL+"/nostore", nil)
	if err != nil {
		t.Fatal(err)
	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
	}
}

func TestGetWithEtag(t *testing.T) {
	resetTest()
	req, err := http.NewRequest(methodGET, s.server.URL+"/etag", nil)
	if err != nil {
		t.Fatal(err)
	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
		_, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "1" {
			t.Fatalf(`XFromCache header isn't "1": %v`, resp.Header.Get(XFromCache))
		}
		if resp.Header.Get(XRevalidated) != "1" {
			t.Fatalf(`XRevalidated header isn't "1": %v`, resp.Header.Get(XRevalidated))
		}
		// additional assertions to verify that 304 response is converted properly
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("response status code isn't 200 OK: %v", resp.StatusCode)
		}
		if _, ok := resp.Header["Connection"]; ok {
			t.Fatalf("Connection header isn't absent")
		}
	}
}

func TestCachingJSONWithoutContentLength(t *testing.T) {
	resetTest()
	req, err := http.NewRequest(methodGET, s.server.URL+"/json", nil)
	if err != nil {
		t.Fatal(err)
	}

	// First request - should not be cached
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.Header.Get(XFromCache) != "" {
		t.Fatal("XFromCache header isn't blank on first request")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	var data map[string]string
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatal(err)
	}

	if data["k"] != "v" {
		t.Fatalf("unexpected JSON response: %v", data)
	}

	// Second request - should be served from cache
	resp2, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp2.Body.Close() }()

	if resp2.Header.Get(XFromCache) != "1" {
		t.Fatal("XFromCache header isn't '1' on second request - caching failed")
	}

	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatal(err)
	}

	if err := json.Unmarshal(body2, &data); err != nil {
		t.Fatal(err)
	}

	if data["k"] != "v" {
		t.Fatalf("unexpected JSON response from cache: %v", data)
	}
}

func TestSkipServerErrorsFromCache(t *testing.T) {
	resetTest()

	// Test 1: Default behavior (SkipServerErrorsFromCache = false)
	// Manually create and cache a 500 error response
	req, err := http.NewRequest(methodGET, "http://example.com/error", nil)
	if err != nil {
		t.Fatal(err)
	}

	errorResp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header: http.Header{
			"Cache-Control": []string{"max-age=3600"},
			"Date":          []string{time.Now().UTC().Format(time.RFC1123)},
			"Etag":          []string{"error-etag"},
		},
		Body:    io.NopCloser(bytes.NewBufferString("Server error")),
		Request: req,
	}

	// Dump and store in cache
	respBytes, _ := httputil.DumpResponse(errorResp, true)
	cacheKey := req.URL.String()
	_ = s.transport.Cache.Set(req.Context(), cacheKey, respBytes)

	// Retrieve from cache - default behavior should serve the 500 from cache
	cachedResp, err := CachedResponse(s.transport.Cache, req)
	if err != nil {
		t.Fatal(err)
	}

	if cachedResp == nil {
		t.Fatal("expected cached response")
	}

	// With default settings (SkipServerErrorsFromCache = false), handleCachedResponse should allow it
	_, useCache := s.transport.handleCachedResponse(cachedResp, req)
	if !useCache {
		t.Fatal("Expected to use cached 500 with default settings")
	}

	// Test 2: With SkipServerErrorsFromCache = true
	s.transport.SkipServerErrorsFromCache = true

	// Now handleCachedResponse should NOT allow using the cached 500
	cachedResp2, _ := CachedResponse(s.transport.Cache, req)
	_, useCache2 := s.transport.handleCachedResponse(cachedResp2, req)
	if useCache2 {
		t.Fatal("Should NOT use cached 500 when SkipServerErrorsFromCache is true")
	}
}

func TestGetWithLastModified(t *testing.T) {
	resetTest()
	req, err := http.NewRequest(methodGET, s.server.URL+"/lastmodified", nil)
	if err != nil {
		t.Fatal(err)
	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
		_, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "1" {
			t.Fatalf(`XFromCache header isn't "1": %v`, resp.Header.Get(XFromCache))
		}
	}
}

func TestGetWithVary(t *testing.T) {
	resetTest()
	s.transport.EnableVarySeparation = true // Enable vary separation for this test
	req, err := http.NewRequest(methodGET, s.server.URL+"/varyaccept", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/plain")
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get("Vary") != "Accept" {
			t.Fatalf(`Vary header isn't "Accept": %v`, resp.Header.Get("Vary"))
		}
		_, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "1" {
			t.Fatalf(`XFromCache header isn't "1": %v`, resp.Header.Get(XFromCache))
		}
	}
	req.Header.Set("Accept", "text/html")
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
	}
	req.Header.Set("Accept", "")
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
	}
}

func TestGetWithDoubleVary(t *testing.T) {
	resetTest()
	s.transport.EnableVarySeparation = true // Enable vary separation for this test
	req, err := http.NewRequest(methodGET, s.server.URL+"/doublevary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("Accept-Language", "da, en-gb;q=0.8, en;q=0.7")
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get("Vary") == "" {
			t.Fatalf(`Vary header is blank`)
		}
		_, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "1" {
			t.Fatalf(`XFromCache header isn't "1": %v`, resp.Header.Get(XFromCache))
		}
	}
	req.Header.Set("Accept-Language", "")
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
	}
	req.Header.Set("Accept-Language", "da")
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
	}
}

func TestGetWith2VaryHeaders(t *testing.T) {
	resetTest()
	s.transport.EnableVarySeparation = true // Enable vary separation for this test
	// Tests that multiple Vary headers' comma-separated lists are
	// merged. See https://github.com/sandrolain/httpcache/issues/27.
	const (
		accept         = "text/plain"
		acceptLanguage = "da, en-gb;q=0.8, en;q=0.7"
	)
	req, err := http.NewRequest(methodGET, s.server.URL+"/2varyheaders", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", acceptLanguage)
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get("Vary") == "" {
			t.Fatalf(`Vary header is blank`)
		}
		_, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "1" {
			t.Fatalf(`XFromCache header isn't "1": %v`, resp.Header.Get(XFromCache))
		}
	}
	req.Header.Set("Accept-Language", "")
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
	}
	req.Header.Set("Accept-Language", "da")
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
	}
	req.Header.Set("Accept-Language", acceptLanguage)
	req.Header.Set("Accept", "")
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
	}
	req.Header.Set("Accept", "image/png")
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "" {
			t.Fatal("XFromCache header isn't blank")
		}
		_, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "1" {
			t.Fatalf(`XFromCache header isn't "1": %v`, resp.Header.Get(XFromCache))
		}
	}
}

func TestGetVaryUnused(t *testing.T) {
	resetTest()
	s.transport.EnableVarySeparation = true // Enable vary separation for this test
	req, err := http.NewRequest(methodGET, s.server.URL+"/varyunused", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/plain")
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get("Vary") == "" {
			t.Fatalf(`Vary header is blank`)
		}
		_, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "1" {
			t.Fatalf(`XFromCache header isn't "1": %v`, resp.Header.Get(XFromCache))
		}
	}
}

func TestUpdateFields(t *testing.T) {
	resetTest()
	req, err := http.NewRequest(methodGET, s.server.URL+"/updatefields", nil)
	if err != nil {
		t.Fatal(err)
	}
	var counter, counter2 string
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		counter = resp.Header.Get("x-counter")
		_, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.Header.Get(XFromCache) != "1" {
			t.Fatalf(`XFromCache header isn't "1": %v`, resp.Header.Get(XFromCache))
		}
		counter2 = resp.Header.Get("x-counter")
	}
	if counter == counter2 {
		t.Fatalf(`both "x-counter" values are equal: %v %v`, counter, counter2)
	}
}

// This tests the fix for https://github.com/sandrolain/httpcache/issues/74.
// Previously, after validating a cached response, its StatusCode
// was incorrectly being replaced.
func TestCachedErrorsKeepStatus(t *testing.T) {
	resetTest()
	req, err := http.NewRequest(methodGET, s.server.URL+"/cachederror", nil)
	if err != nil {
		t.Fatal(err)
	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	{
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("Status code isn't 404: %d", resp.StatusCode)
		}
	}
}

func TestParseCacheControl(t *testing.T) {
	resetTest()
	h := http.Header{}
	for range parseCacheControl(h, slog.Default()) {
		t.Fatal("cacheControl should be empty")
	}

	h.Set("cache-control", "no-cache")
	{
		cc := parseCacheControl(h, slog.Default())
		if _, ok := cc["foo"]; ok {
			t.Error(`Value "foo" shouldn't exist`)
		}
		noCache, ok := cc["no-cache"]
		if !ok {
			t.Fatalf(`"no-cache" value isn't set`)
		}
		if noCache != "" {
			t.Fatalf(`"no-cache" value isn't blank: %v`, noCache)
		}
	}
	h.Set("cache-control", "no-cache, max-age=3600")
	{
		cc := parseCacheControl(h, slog.Default())
		noCache, ok := cc["no-cache"]
		if !ok {
			t.Fatalf(`"no-cache" value isn't set`)
		}
		if noCache != "" {
			t.Fatalf(`"no-cache" value isn't blank: %v`, noCache)
		}
		if cc["max-age"] != "3600" {
			t.Fatalf(`"max-age" value isn't "3600": %v`, cc["max-age"])
		}
	}
}

func TestNoCacheRequestExpiration(t *testing.T) {
	resetTest()
	respHeaders := http.Header{}
	respHeaders.Set("Cache-Control", "max-age=7200")

	reqHeaders := http.Header{}
	reqHeaders.Set("Cache-Control", "no-cache")
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != transparent {
		t.Fatal("freshness isn't transparent")
	}
}

func TestNoCacheResponseExpiration(t *testing.T) {
	resetTest()
	respHeaders := http.Header{}
	respHeaders.Set("Cache-Control", "no-cache")
	respHeaders.Set("Expires", "Wed, 19 Apr 3000 11:43:00 GMT")

	reqHeaders := http.Header{}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != stale {
		t.Fatal("freshness isn't stale")
	}
}

func TestReqMustRevalidate(t *testing.T) {
	resetTest()
	// not paying attention to request setting max-stale means never returning stale
	// responses, so always acting as if must-revalidate is set
	respHeaders := http.Header{}

	reqHeaders := http.Header{}
	reqHeaders.Set("Cache-Control", "must-revalidate")
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != stale {
		t.Fatal("freshness isn't stale")
	}
}

func TestRespMustRevalidate(t *testing.T) {
	resetTest()
	respHeaders := http.Header{}
	respHeaders.Set("Cache-Control", "must-revalidate")

	reqHeaders := http.Header{}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != stale {
		t.Fatal("freshness isn't stale")
	}
}

func TestFreshExpiration(t *testing.T) {
	resetTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("expires", now.Add(time.Duration(2)*time.Second).Format(time.RFC1123))

	reqHeaders := http.Header{}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != fresh {
		t.Fatal("freshness isn't fresh")
	}

	clock = &fakeClock{elapsed: 3 * time.Second}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != stale {
		t.Fatal("freshness isn't stale")
	}
}

func TestMaxAge(t *testing.T) {
	resetTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=2")

	reqHeaders := http.Header{}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != fresh {
		t.Fatal("freshness isn't fresh")
	}

	clock = &fakeClock{elapsed: 3 * time.Second}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != stale {
		t.Fatal("freshness isn't stale")
	}
}

func TestMaxAgeZero(t *testing.T) {
	resetTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=0")

	reqHeaders := http.Header{}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != stale {
		t.Fatal("freshness isn't stale")
	}
}

func TestBothMaxAge(t *testing.T) {
	resetTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=2")

	reqHeaders := http.Header{}
	reqHeaders.Set("cache-control", "max-age=0")
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != stale {
		t.Fatal("freshness isn't stale")
	}
}

func TestMinFreshWithExpires(t *testing.T) {
	resetTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("expires", now.Add(time.Duration(2)*time.Second).Format(time.RFC1123))

	reqHeaders := http.Header{}
	reqHeaders.Set("cache-control", "min-fresh=1")
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != fresh {
		t.Fatal("freshness isn't fresh")
	}

	reqHeaders = http.Header{}
	reqHeaders.Set("cache-control", "min-fresh=2")
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != stale {
		t.Fatal("freshness isn't stale")
	}
}

func TestEmptyMaxStale(t *testing.T) {
	resetTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=20")

	reqHeaders := http.Header{}
	reqHeaders.Set("cache-control", "max-stale")
	clock = &fakeClock{elapsed: 10 * time.Second}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != fresh {
		t.Fatal("freshness isn't fresh")
	}

	clock = &fakeClock{elapsed: 60 * time.Second}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != fresh {
		t.Fatal("freshness isn't fresh")
	}
}

func TestMaxStaleValue(t *testing.T) {
	resetTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=10")

	reqHeaders := http.Header{}
	reqHeaders.Set("cache-control", "max-stale=20")
	clock = &fakeClock{elapsed: 5 * time.Second}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != fresh {
		t.Fatal("freshness isn't fresh")
	}

	clock = &fakeClock{elapsed: 15 * time.Second}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != fresh {
		t.Fatal("freshness isn't fresh")
	}

	clock = &fakeClock{elapsed: 30 * time.Second}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != stale {
		t.Fatal("freshness isn't stale")
	}
}

func containsHeader(headers []string, header string) bool {
	for _, v := range headers {
		if http.CanonicalHeaderKey(v) == http.CanonicalHeaderKey(header) {
			return true
		}
	}
	return false
}

func TestGetEndToEndHeaders(t *testing.T) {
	resetTest()
	var (
		headers http.Header
		end2end []string
	)

	headers = http.Header{}
	headers.Set("content-type", "text/html")
	headers.Set("te", "deflate")

	end2end = getEndToEndHeaders(headers)
	if !containsHeader(end2end, "content-type") {
		t.Fatal(`doesn't contain "content-type" header`)
	}
	if containsHeader(end2end, "te") {
		t.Fatal(`doesn't contain "te" header`)
	}

	headers = http.Header{}
	headers.Set("connection", "content-type")
	headers.Set("content-type", "text/csv")
	headers.Set("te", "deflate")
	end2end = getEndToEndHeaders(headers)
	if containsHeader(end2end, "connection") {
		t.Fatal(`doesn't contain "connection" header`)
	}
	if containsHeader(end2end, "content-type") {
		t.Fatal(`doesn't contain "content-type" header`)
	}
	if containsHeader(end2end, "te") {
		t.Fatal(`doesn't contain "te" header`)
	}

	headers = http.Header{}
	end2end = getEndToEndHeaders(headers)
	if len(end2end) != 0 {
		t.Fatal(`non-zero end2end headers`)
	}

	headers = http.Header{}
	headers.Set("connection", "content-type")
	end2end = getEndToEndHeaders(headers)
	if len(end2end) != 0 {
		t.Fatal(`non-zero end2end headers`)
	}
}

type transportMock struct {
	response *http.Response
	err      error
}

func (t transportMock) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	return t.response, t.err
}

func TestStaleIfErrorRequest(t *testing.T) {
	resetTest()
	now := time.Now()
	tmock := transportMock{
		response: &http.Response{
			Status:     http.StatusText(http.StatusOK),
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Date":          []string{now.Format(time.RFC1123)},
				"Cache-Control": []string{"no-cache"},
			},
			Body: io.NopCloser(bytes.NewBuffer([]byte("some data"))),
		},
		err: nil,
	}
	tp := newMockCacheTransport()
	tp.Transport = &tmock

	// First time, response is cached on success
	r, _ := http.NewRequest(methodGET, "http://somewhere.com/", nil)
	r.Header.Set("Cache-Control", "stale-if-error")
	resp, err := tp.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("resp is nil")
	}
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// On failure, response is returned from the cache
	tmock.response = nil
	tmock.err = errors.New("some error")
	resp, err = tp.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("resp is nil")
	}
	if resp.Header.Get(XStale) != "1" {
		t.Fatalf(`XStale header isn't "1": %v`, resp.Header.Get(XStale))
	}
}

func TestStaleIfErrorRequestLifetime(t *testing.T) {
	resetTest()
	now := time.Now()
	tmock := transportMock{
		response: &http.Response{
			Status:     http.StatusText(http.StatusOK),
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Date":          []string{now.Format(time.RFC1123)},
				"Cache-Control": []string{"no-cache"},
			},
			Body: io.NopCloser(bytes.NewBuffer([]byte("some data"))),
		},
		err: nil,
	}
	tp := newMockCacheTransport()
	tp.Transport = &tmock

	// First time, response is cached on success
	r, _ := http.NewRequest(methodGET, "http://somewhere.com/", nil)
	r.Header.Set("Cache-Control", "stale-if-error=100")
	resp, err := tp.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("resp is nil")
	}
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// On failure, response is returned from the cache
	tmock.response = nil
	tmock.err = errors.New("some error")
	resp, err = tp.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("resp is nil")
	}
	if resp.Header.Get(XStale) != "1" {
		t.Fatalf(`XStale header isn't "1": %v`, resp.Header.Get(XStale))
	}

	// Same for http errors
	tmock.response = &http.Response{StatusCode: http.StatusInternalServerError}
	tmock.err = nil
	resp, err = tp.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("resp is nil")
	}
	if resp.Header.Get(XStale) != "1" {
		t.Fatalf(`XStale header isn't "1": %v`, resp.Header.Get(XStale))
	}

	// If failure last more than max stale, error is returned
	clock = &fakeClock{elapsed: 200 * time.Second}
	_, err = tp.RoundTrip(r)
	if err != tmock.err {
		t.Fatalf("got err %v, want %v", err, tmock.err)
	}
}

func TestStaleIfErrorResponse(t *testing.T) {
	resetTest()
	now := time.Now()
	tmock := transportMock{
		response: &http.Response{
			Status:     http.StatusText(http.StatusOK),
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Date":          []string{now.Format(time.RFC1123)},
				"Cache-Control": []string{"no-cache, stale-if-error"},
			},
			Body: io.NopCloser(bytes.NewBuffer([]byte("some data"))),
		},
		err: nil,
	}
	tp := newMockCacheTransport()
	tp.Transport = &tmock

	// First time, response is cached on success
	r, _ := http.NewRequest(methodGET, "http://somewhere.com/", nil)
	resp, err := tp.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("resp is nil")
	}
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// On failure, response is returned from the cache
	tmock.response = nil
	tmock.err = errors.New("some error")
	resp, err = tp.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("resp is nil")
	}
	if resp.Header.Get(XStale) != "1" {
		t.Fatalf(`XStale header isn't "1": %v`, resp.Header.Get(XStale))
	}
}

func TestStaleIfErrorResponseLifetime(t *testing.T) {
	resetTest()
	now := time.Now()
	tmock := transportMock{
		response: &http.Response{
			Status:     http.StatusText(http.StatusOK),
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Date":          []string{now.Format(time.RFC1123)},
				"Cache-Control": []string{"no-cache, stale-if-error=100"},
			},
			Body: io.NopCloser(bytes.NewBuffer([]byte("some data"))),
		},
		err: nil,
	}
	tp := newMockCacheTransport()
	tp.Transport = &tmock

	// First time, response is cached on success
	r, _ := http.NewRequest(methodGET, "http://somewhere.com/", nil)
	resp, err := tp.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("resp is nil")
	}
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// On failure, response is returned from the cache
	tmock.response = nil
	tmock.err = errors.New("some error")
	resp, err = tp.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("resp is nil")
	}
	if resp.Header.Get(XStale) != "1" {
		t.Fatalf(`XStale header isn't "1": %v`, resp.Header.Get(XStale))
	}

	// If failure last more than max stale, error is returned
	clock = &fakeClock{elapsed: 200 * time.Second}
	_, err = tp.RoundTrip(r)
	if err != tmock.err {
		t.Fatalf("got err %v, want %v", err, tmock.err)
	}
}

// This tests the fix for https://github.com/sandrolain/httpcache/issues/74.
// Previously, after a stale response was used after encountering an error,
// its StatusCode was being incorrectly replaced.
func TestStaleIfErrorKeepsStatus(t *testing.T) {
	resetTest()
	now := time.Now()
	tmock := transportMock{
		response: &http.Response{
			Status:     http.StatusText(http.StatusNotFound),
			StatusCode: http.StatusNotFound,
			Header: http.Header{
				"Date":          []string{now.Format(time.RFC1123)},
				"Cache-Control": []string{"no-cache"},
			},
			Body: io.NopCloser(bytes.NewBuffer([]byte("some data"))),
		},
		err: nil,
	}
	tp := newMockCacheTransport()
	tp.Transport = &tmock

	// First time, response is cached on success
	r, _ := http.NewRequest(methodGET, "http://somewhere.com/", nil)
	r.Header.Set("Cache-Control", "stale-if-error")
	resp, err := tp.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("resp is nil")
	}
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// On failure, response is returned from the cache
	tmock.response = nil
	tmock.err = errors.New("some error")
	resp, err = tp.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("resp is nil")
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Status wasn't 404: %d", resp.StatusCode)
	}
	if resp.Header.Get(XStale) != "1" {
		t.Fatalf(`XStale header isn't "1": %v`, resp.Header.Get(XStale))
	}
}

// Test that http.Client.Timeout is respected when cache transport is used.
// That is so as long as request cancellation is propagated correctly.
// In the past, that required CancelRequest to be implemented correctly,
// but modern http.Client uses Request.Cancel (or request context) instead,
// so we don't have to do anything.
func TestClientTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in short mode") // Because it takes at least 3 seconds to run.
	}
	resetTest()
	client := &http.Client{
		Transport: newMockCacheTransport(),
		Timeout:   time.Second,
	}
	started := time.Now()
	resp, err := client.Get(s.server.URL + "/3seconds")
	taken := time.Since(started)
	if err == nil {
		t.Error("got nil error, want timeout error")
	}
	if resp != nil {
		t.Error("got non-nil resp, want nil resp")
	}
	if taken >= 2*time.Second {
		t.Error("client.Do took 2+ seconds, want < 2 seconds")
	}
}

func TestFreshnessStaleWhileRevalidate(t *testing.T) {
	resetTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("Cache-Control", "max-age=100, stale-while-revalidate=100")

	reqHeaders := http.Header{}

	clock = &fakeClock{elapsed: 50 * time.Second}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != fresh {
		t.Fatal("freshness isn't fresh")
	}

	clock = &fakeClock{elapsed: 150 * time.Second}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != staleWhileRevalidate {
		t.Fatal("freshness isn't staleWhileRevalidate")
	}

	clock = &fakeClock{elapsed: 250 * time.Second}
	if getFreshness(respHeaders, reqHeaders, slog.Default()) != stale {
		t.Fatal("freshness isn't stale")
	}
}

func TestStaleWhileRevalidate(t *testing.T) {
	resetTest()
	req, err := http.NewRequest("GET", s.server.URL+"/stale-while-revalidate", nil)
	if err != nil {
		t.Fatal(err)
	}
	var counter1 string
	{
		// 1st request: Not cached
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.Header.Get(XFromCache) != "" {
			t.Fatalf(`XFromCache header isn't absent: %v`, resp.Header.Get(XFromCache))
		}
		if resp.Header.Get(XFreshness) != "" {
			t.Fatalf(`X-Cache-Freshness header isn't absent: %v`, resp.Header.Get(XFreshness))
		}

		counter1 = resp.Header.Get("x-counter")

		_, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
	}
	{
		// 2nd request: Fresh
		clock = &fakeClock{elapsed: 50 * time.Second}
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.Header.Get(XFromCache) != "1" {
			t.Fatalf(`XFromCache header isn't "1": %v`, resp.Header.Get(XFromCache))
		}
		if resp.Header.Get(XFreshness) != "fresh" {
			t.Fatalf(`X-Cache-Freshness header isn't "fresh": %v`, resp.Header.Get(XFreshness))
		}

		counter := resp.Header.Get("x-counter")
		if counter1 != counter {
			t.Fatalf(`"x-counter" values are different: %v %v`, counter1, counter)
		}

		_, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
	}
	{
		// 3rd request: Stale-While-Revalidate
		clock = &fakeClock{elapsed: 150 * time.Second}
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.Header.Get(XFromCache) != "1" {
			t.Fatalf(`XFromCache header isn't "1": %v`, resp.Header.Get(XFromCache))
		}
		if resp.Header.Get(XFreshness) != "stale-while-revalidate" {
			t.Fatalf(`X-Cache-Freshness header isn't "stale-while-revalidate": %v`, resp.Header.Get(XFreshness))
		}

		counter := resp.Header.Get("x-counter")
		if counter1 != counter {
			t.Fatalf(`"x-counter" values are different: %v %v`, counter1, counter)
		}

		_, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		// Revalidate is asynchronous, make sure it completes executing
		time.Sleep(1 * time.Second)
	}
	{
		// 4th request: Return the response cached just now
		clock = &fakeClock{elapsed: 50 * time.Second}
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.Header.Get(XFromCache) != "1" {
			t.Fatalf(`XFromCache header isn't "1": %v`, resp.Header.Get(XFromCache))
		}
		if resp.Header.Get(XFreshness) != "fresh" {
			t.Fatalf(`X-Cache-Freshness header isn't "fresh": %v`, resp.Header.Get(XFreshness))
		}

		counter := resp.Header.Get("x-counter")
		if counter1 == counter {
			t.Fatalf(`"x-counter" values are equal: %v %v`, counter1, counter)
		}

		_, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestShouldCacheNotFound(t *testing.T) {
	resetTest()

	// Configure transport to cache 404 responses
	s.transport.ShouldCache = func(resp *http.Response) bool {
		return resp.StatusCode == http.StatusNotFound
	}

	req, err := http.NewRequest("GET", s.server.URL+"/notfound", nil)
	if err != nil {
		t.Fatal(err)
	}

	// First request: should cache the 404
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	if resp.Header.Get(XFromCache) != "" {
		t.Error("first request should not be from cache")
	}

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// Second request: should be served from cache
	resp, err = s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	if resp.Header.Get(XFromCache) != "1" {
		t.Error("second request should be from cache")
	}

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
}

func TestShouldCacheRedirect(t *testing.T) {
	resetTest()

	// Configure transport to cache redirect responses
	s.transport.ShouldCache = func(resp *http.Response) bool {
		return resp.StatusCode >= 300 && resp.StatusCode < 400
	}

	// Disable following redirects for this test
	oldClient := s.client
	s.client = http.Client{
		Transport: s.transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer func() { s.client = oldClient }()

	req, err := http.NewRequest("GET", s.server.URL+"/redirect", nil)
	if err != nil {
		t.Fatal(err)
	}

	// First request: should cache the redirect
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", resp.StatusCode)
	}
	if resp.Header.Get(XFromCache) != "" {
		t.Error("first request should not be from cache")
	}

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// Second request: should be served from cache
	resp, err = s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", resp.StatusCode)
	}
	if resp.Header.Get(XFromCache) != "1" {
		t.Error("second request should be from cache")
	}
	if location := resp.Header.Get("Location"); location != "http://example.com/target" {
		t.Errorf("expected location header, got %q", location)
	}

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
}

func TestShouldCacheNilDoesNotCache(t *testing.T) {
	resetTest()

	// Ensure ShouldCache is explicitly nil
	s.transport.ShouldCache = nil

	req, err := http.NewRequest("GET", s.server.URL+"/badrequest", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Clear cache to ensure clean state
	s.transport.Cache = newMockCache()

	// First request
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	if resp.Header.Get(XFromCache) != "" {
		t.Error("first request should not be from cache")
	}

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// Second request: should NOT be from cache (400 is not cacheable by default per RFC 7231)
	resp, err = s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get(XFromCache); got != "" {
		t.Errorf("400 should not be cached when ShouldCache is nil, got XFromCache=%q", got)
	}

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
}
