package httpcache

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDeduplicationKeyIncludesAllRequestHeaders(t *testing.T) {
	first, _ := http.NewRequest(http.MethodGet, "https://example.test/items", nil)
	second, _ := http.NewRequest(http.MethodGet, "https://example.test/items", nil)
	first.Header.Set("Accept-Language", "en")
	second.Header.Set("Accept-Language", "fr")
	if deduplicationKey("url-key", first) == deduplicationKey("url-key", second) {
		t.Fatal("requests with distinct headers must have different deduplication keys")
	}
}

func TestDeduplicationKeyLengthPrefixesValues(t *testing.T) {
	first, _ := http.NewRequest(http.MethodGet, "https://example.test/items", nil)
	second, _ := http.NewRequest(http.MethodGet, "https://example.test/items", nil)
	first.Header["X-A"] = []string{"a", "bc"}
	second.Header["X-A"] = []string{"ab", "c"}
	if deduplicationKey("key", first) == deduplicationKey("key", second) {
		t.Fatal("different header value sequences must not collide")
	}
}

func TestDeduplicationKeyIncludesNoncanonicalHeaderMapEntries(t *testing.T) {
	first, _ := http.NewRequest(http.MethodGet, "https://example.test/items", nil)
	second, _ := http.NewRequest(http.MethodGet, "https://example.test/items", nil)
	first.Header["x-custom"] = []string{"first"}
	second.Header["x-custom"] = []string{"second"}
	if deduplicationKey("key", first) == deduplicationKey("key", second) {
		t.Fatal("different values in a noncanonical header map entry must not collide")
	}
}

func TestProbeDedupResponseBoundsBufferAndRestoresPrefix(t *testing.T) {
	resp := &http.Response{Header: make(http.Header), Body: io.NopCloser(strings.NewReader("0123456789")), ContentLength: -1}
	resp, _, oversized, noShare, err := probeDedupResponse(resp, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !oversized || noShare {
		t.Fatalf("probe flags = oversized %v, noShare %v; want true, false", oversized, noShare)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got, want := string(body), "0123456789"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestProbeDedupResponseDoesNotShareVaryStar(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Vary": {"Accept-Language, *"}}, Body: io.NopCloser(strings.NewReader("small")), ContentLength: 5}
	resp, _, oversized, noShare, err := probeDedupResponse(resp, 10)
	if err != nil {
		t.Fatal(err)
	}
	if oversized || !noShare {
		t.Fatalf("probe flags = oversized %v, noShare %v; want false, true", oversized, noShare)
	}
	_ = resp.Body.Close()
}

func TestDedupWaiterCanCancelIndependently(t *testing.T) {
	var group dedupGroup
	started := make(chan struct{})
	finish := make(chan struct{})
	originCtx := make(chan context.Context, 1)
	fn := func(ctx context.Context, _ *http.Request) (*http.Response, error) {
		originCtx <- ctx
		close(started)
		<-finish
		return dedupTestResponse("ok"), nil
	}

	leaderReq, _ := http.NewRequest(http.MethodGet, "https://example.test/items", nil)
	leaderDone := make(chan error, 1)
	go func() {
		resp, _, err := group.do(leaderReq, "key", 10, fn)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		leaderDone <- err
	}()
	<-started
	operationCtx := <-originCtx

	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	waiterReq, _ := http.NewRequestWithContext(waiterCtx, http.MethodGet, "https://example.test/items", nil)
	waiterDone := make(chan error, 1)
	go func() {
		_, _, err := group.do(waiterReq, "key", 10, fn)
		waiterDone <- err
	}()
	waitForDedupRefs(t, &group, "key", 2)
	cancelWaiter()
	select {
	case err := <-waiterDone:
		if err != context.Canceled {
			t.Fatalf("waiter error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter did not return promptly")
	}
	select {
	case <-operationCtx.Done():
		t.Fatal("origin canceled while leader remained active")
	default:
	}
	close(finish)
	if err := <-leaderDone; err != nil {
		t.Fatalf("leader error: %v", err)
	}
}

func TestDedupCreatorCancellationDoesNotCancelActiveWaiter(t *testing.T) {
	var group dedupGroup
	started := make(chan struct{})
	finish := make(chan struct{})
	originCtx := make(chan context.Context, 1)
	fn := func(ctx context.Context, _ *http.Request) (*http.Response, error) {
		originCtx <- ctx
		close(started)
		<-finish
		return dedupTestResponse("ok"), nil
	}

	creatorCtx, cancelCreator := context.WithCancel(context.Background())
	creatorReq, _ := http.NewRequestWithContext(creatorCtx, http.MethodGet, "https://example.test/items", nil)
	creatorDone := make(chan error, 1)
	go func() {
		_, _, err := group.do(creatorReq, "key", 10, fn)
		creatorDone <- err
	}()
	<-started
	operationCtx := <-originCtx

	waiterReq, _ := http.NewRequest(http.MethodGet, "https://example.test/items", nil)
	waiterDone := make(chan error, 1)
	go func() {
		resp, _, err := group.do(waiterReq, "key", 10, fn)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		waiterDone <- err
	}()
	waitForDedupRefs(t, &group, "key", 2)
	cancelCreator()
	if err := <-creatorDone; err != context.Canceled {
		t.Fatalf("creator error = %v, want context.Canceled", err)
	}
	select {
	case <-operationCtx.Done():
		t.Fatal("origin canceled while another participant remained active")
	default:
	}
	close(finish)
	if err := <-waiterDone; err != nil {
		t.Fatalf("waiter error: %v", err)
	}
}

func TestDedupClosesResponseReturnedAfterAllCallersCancel(t *testing.T) {
	var group dedupGroup
	started := make(chan struct{})
	releaseOrigin := make(chan struct{})
	originCtxCh := make(chan context.Context, 1)
	var closeCount atomic.Int32
	fn := func(ctx context.Context, _ *http.Request) (*http.Response, error) {
		originCtxCh <- ctx
		close(started)
		<-releaseOrigin
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       &countingCloseBody{closed: &closeCount},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test/items", nil)
	result := make(chan error, 1)
	go func() {
		_, _, err := group.do(req, "key", 10, fn)
		result <- err
	}()
	<-started
	originCtx := <-originCtxCh
	cancel()
	if err := <-result; err != context.Canceled {
		t.Fatalf("request error = %v, want context.Canceled", err)
	}
	select {
	case <-originCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("origin context was not canceled after the last caller left")
	}
	close(releaseOrigin)
	deadline := time.Now().Add(time.Second)
	for closeCount.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := closeCount.Load(); got != 1 {
		t.Fatalf("response body close count = %d, want exactly 1", got)
	}
}

type countingCloseBody struct{ closed *atomic.Int32 }

func (*countingCloseBody) Read([]byte) (int, error) { return 0, io.EOF }
func (b *countingCloseBody) Close() error {
	b.closed.Add(1)
	return nil
}

func TestTransportDedupSeparatesRequestsWithDifferentHeaders(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Vary", "Accept-Language")
		_, _ = io.WriteString(w, r.Header.Get("Accept-Language"))
	}))
	defer server.Close()

	transport := NewTransport(newMockCache())
	transport.EnableDeduplication = true
	results := make([]string, 2)
	langs := []string{"en", "fr"}
	var wg sync.WaitGroup
	for i, lang := range langs {
		wg.Add(1)
		go func(i int, lang string) {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
			req.Header.Set("Accept-Language", lang)
			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Errorf("RoundTrip(%s): %v", lang, err)
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("read body(%s): %v", lang, err)
				return
			}
			results[i] = string(body)
		}(i, lang)
	}
	wg.Wait()
	if results[0] != "en" || results[1] != "fr" {
		t.Fatalf("responses = %q, want [en fr]", results)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("origin calls = %d, want 2", got)
	}
}

func TestTransportDedupRetriesWaiterForOversizedResponse(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call := calls.Add(1)
		if call == 1 {
			<-release
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", "10")
		_, _ = io.WriteString(w, "0123456789")
	}))
	defer server.Close()

	transport := NewTransport(newMockCache())
	transport.EnableDeduplication = true
	transport.MaxCacheableResponseSize = 4
	results := make([]string, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Errorf("RoundTrip: %v", err)
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				return
			}
			results[i] = string(body)
		}(i)
	}
	waitForDedupRefs(t, &transport.deduplication, deduplicationKey(cacheKeyForTest(server.URL), mustRequest(t, server.URL)), 2)
	close(release)
	wg.Wait()
	for i, got := range results {
		if got != "0123456789" {
			t.Errorf("response %d = %q, want complete body", i, got)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("origin calls = %d, want one shared request plus one independent retry", got)
	}
}

func TestTransportDedupDoesNotShareVaryStarResponse(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		<-release
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Vary", "*")
		_, _ = io.WriteString(w, "response")
	}))
	defer server.Close()

	transport := NewTransport(newMockCache())
	transport.EnableDeduplication = true
	results := make([]string, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Errorf("RoundTrip: %v", err)
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				return
			}
			results[i] = string(body)
		}(i)
	}
	waitForDedupRefs(t, &transport.deduplication, deduplicationKey(cacheKeyForTest(server.URL), mustRequest(t, server.URL)), 2)
	close(release)
	wg.Wait()
	for i, got := range results {
		if got != "response" {
			t.Errorf("response %d = %q, want %q", i, got, "response")
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("origin calls = %d, want 2 because Vary:* responses are not shared", got)
	}
}

func TestTransportDedupPreservesOversizedResponseTrailers(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call := calls.Add(1)
		if call == 1 {
			<-release
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Add("Trailer", "X-Checksum")
		_, _ = io.WriteString(w, "0123456789")
		w.Header().Set("X-Checksum", "complete")
	}))
	defer server.Close()

	transport := NewTransport(newMockCache())
	transport.EnableDeduplication = true
	transport.MaxCacheableResponseSize = 4
	req := mustRequest(t, server.URL)
	respCh := make(chan *http.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := transport.RoundTrip(req)
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()
	waitForDedupRefs(t, &transport.deduplication, deduplicationKey(cacheKeyForTest(server.URL), req), 1)
	close(release)
	select {
	case err := <-errCh:
		t.Fatal(err)
	case resp := <-respCh:
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "0123456789" {
			t.Fatalf("body = %q, want complete body", body)
		}
		if got := resp.Trailer.Get("X-Checksum"); got != "complete" {
			t.Fatalf("trailer = %q, want complete", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response")
	}
}

func dedupTestResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
}

func mustRequest(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func cacheKeyForTest(url string) string { return url }

func waitForDedupRefs(t *testing.T, group *dedupGroup, key string, refs int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		group.mu.Lock()
		flight := group.flights[key]
		got := 0
		if flight != nil {
			got = flight.refs
		}
		group.mu.Unlock()
		if got == refs {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("flight %q did not reach %d references", key, refs)
}
