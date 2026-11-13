package httpcache

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStaleWhileRevalidateIsConditionalAndDeduplicated(t *testing.T) {
	resetTest()
	var requests atomic.Int32
	validatorSeen := make(chan struct{}, 10)
	release := make(chan struct{})
	releaseOrigin := func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Cache-Control", "max-age=1, stale-while-revalidate=60")
			w.Header().Set("ETag", `"cached"`)
			_, _ = io.WriteString(w, "representation")
			return
		}
		if r.Header.Get("If-None-Match") != `"cached"` {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		validatorSeen <- struct{}{}
		<-release
		w.Header().Set("Cache-Control", "max-age=60, stale-while-revalidate=60")
		w.Header().Set("ETag", `"cached"`)
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()
	defer releaseOrigin()

	clock := &fakeClock{}
	transport := newMockCacheTransportWithClock(clock)
	transport.SetTransport(http.DefaultTransport)
	request, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	first, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(first.Body); err != nil {
		t.Fatal(err)
	}
	first.Body.Close()

	clock.elapsed = 2 * time.Second
	const clients = 8
	var wg sync.WaitGroup
	errCh := make(chan error, clients)
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := transport.RoundTrip(request.Clone(request.Context()))
			if err != nil {
				errCh <- err
				return
			}
			if resp.Header.Get(XFromCache) != "1" {
				errCh <- errUnexpectedCacheMiss
			}
			if resp.Body != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	select {
	case <-validatorSeen:
	case <-time.After(3 * time.Second):
		t.Fatal("background revalidation did not send the cached validator")
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("got %d origin requests, want one initial fetch and one deduplicated revalidation", got)
	}
	releaseOrigin()
	asyncRevalidateWG.Wait()
}

var errUnexpectedCacheMiss = errors.New("stale-while-revalidate request missed the cache")
