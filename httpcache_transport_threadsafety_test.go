package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// mockRoundTripper implements http.RoundTripper for testing
type mockRoundTripper struct {
	id int
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(http.NoBody),
		Header:     make(http.Header),
	}, nil
}

// TestTransportConcurrentModification verifies that modifying the Transport field
// concurrently with active requests does not cause data races.
// This test addresses the race condition identified in bug-analysis-2026-01-21.md
func TestTransportConcurrentModification(t *testing.T) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache)

	// Create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=1")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	// Use WaitGroup to synchronize goroutines
	var wg sync.WaitGroup
	done := make(chan bool)

	// Goroutine 1: Continuously modify Transport
	wg.Add(1)
	go func() {
		defer wg.Done()
		id := 0
		for {
			select {
			case <-done:
				return
			default:
				transport.SetTransport(&mockRoundTripper{id: id})
				id++
				time.Sleep(time.Microsecond)
			}
		}
	}()

	// Goroutine 2: Continuously make requests
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			req, _ := http.NewRequest("GET", ts.URL, nil)
			resp, err := transport.RoundTrip(req)
			if err == nil && resp != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			time.Sleep(time.Microsecond)
		}
	}()

	// Goroutine 3: Continuously read Transport
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = transport.GetTransport()
			time.Sleep(time.Microsecond)
		}
	}()

	// Wait for request goroutines to complete
	time.Sleep(200 * time.Millisecond)
	close(done)
	wg.Wait()

	// If we reach here without race detector errors, the test passes
}

// TestTransportConcurrentSetAndGet verifies concurrent SetTransport and GetTransport calls
func TestTransportConcurrentSetAndGet(t *testing.T) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache)

	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup

	// Multiple goroutines setting Transport
	for i := 0; i < goroutines/2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				transport.SetTransport(&mockRoundTripper{id: id})
			}
		}(i)
	}

	// Multiple goroutines getting Transport
	for i := 0; i < goroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				rt := transport.GetTransport()
				if rt == nil {
					t.Error("GetTransport returned nil")
				}
			}
		}()
	}

	wg.Wait()
}

// TestTransportDefaultTransportBehavior verifies that GetTransport returns
// http.DefaultTransport when no transport has been set
func TestTransportDefaultTransportBehavior(t *testing.T) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache)

	// Should return DefaultTransport when not set
	rt := transport.GetTransport()
	if rt != http.DefaultTransport {
		t.Errorf("expected DefaultTransport, got %v", rt)
	}

	// Set a custom transport
	custom := &mockRoundTripper{id: 1}
	transport.SetTransport(custom)

	// Should return custom transport
	rt = transport.GetTransport()
	if rt != custom {
		t.Errorf("expected custom transport, got %v", rt)
	}

	// Set nil transport
	transport.SetTransport(nil)

	// Should return DefaultTransport again
	rt = transport.GetTransport()
	if rt != http.DefaultTransport {
		t.Errorf("expected DefaultTransport after setting nil, got %v", rt)
	}
}

// TestTransportThreadSafetyWithAsyncRevalidation tests that Transport modification
// is safe during async revalidation operations
func TestTransportThreadSafetyWithAsyncRevalidation(t *testing.T) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache)
	transport.AsyncRevalidateTimeout = 2 * time.Second

	// Create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=1, stale-while-revalidate=10")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response"))
	}))
	defer ts.Close()

	// First request to populate cache
	req1, _ := http.NewRequest("GET", ts.URL, nil)
	resp1, err := transport.RoundTrip(req1)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	// Wait for response to become stale
	time.Sleep(1200 * time.Millisecond)

	var wg sync.WaitGroup

	// Goroutine 1: Make request that triggers async revalidation
	wg.Add(1)
	go func() {
		defer wg.Done()
		req2, _ := http.NewRequest("GET", ts.URL, nil)
		resp2, err := transport.RoundTrip(req2)
		if err == nil && resp2 != nil {
			_, _ = io.Copy(io.Discard, resp2.Body)
			resp2.Body.Close()
		}
	}()

	// Goroutine 2: Modify Transport while async revalidation may be running
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			transport.SetTransport(&mockRoundTripper{id: i})
			time.Sleep(50 * time.Millisecond)
		}
	}()

	wg.Wait()

	// If we reach here without race detector errors, the test passes
}
