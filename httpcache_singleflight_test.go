package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDuplicateRequests verifies that pending identical requests are deduplicated
func TestDuplicateRequests(t *testing.T) {
	// server that counts requests and blocks until allowed to respond
	counter := atomic.Int32{}
	respondToRequest := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/slowendpoint", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter.Add(1)
		<-respondToRequest

		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write([]byte("information"))
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	cache := newMockCache()
	transport := NewTransport(cache)
	client := http.Client{Transport: transport}

	// Create multiple identical requests in parallel
	const numRequests = 5
	var wg sync.WaitGroup
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			defer wg.Done()
			req, reqErr := http.NewRequest(methodGET, server.URL+"/slowendpoint", nil)
			require.NoError(t, reqErr)

			resp, err := client.Do(req)
			require.NoError(t, err)
			require.NotNil(t, resp)

			body, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)
			assert.Equal(t, []byte("information"), body)
			resp.Body.Close()
		}(i)
	}

	// Allow all requests to proceed after a short delay
	time.Sleep(50 * time.Millisecond)
	close(respondToRequest)

	// Wait for all requests to complete
	wg.Wait()

	// Verify that only one request was made to the server
	assert.Equal(t, int32(1), counter.Load(), "Expected only 1 request to the server, got %d", counter.Load())
}

// TestDuplicateRequestsWithError verifies that errors are properly shared
func TestDuplicateRequestsWithError(t *testing.T) {
	counter := atomic.Int32{}
	respondToRequest := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/error", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter.Add(1)
		<-respondToRequest
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	cache := newMockCache()
	transport := NewTransport(cache)
	client := http.Client{Transport: transport}

	const numRequests = 3
	var wg sync.WaitGroup
	wg.Add(numRequests)
	errorCount := atomic.Int32{}

	for i := 0; i < numRequests; i++ {
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(methodGET, server.URL+"/error", nil)
			resp, err := client.Do(req)
			// Should succeed (no network error) but get 500 status
			require.NoError(t, err)
			if resp.StatusCode == http.StatusInternalServerError {
				errorCount.Add(1)
			}
			resp.Body.Close()
		}()
	}

	time.Sleep(50 * time.Millisecond)
	close(respondToRequest)
	wg.Wait()

	// All requests should get the error response
	assert.Equal(t, int32(1), counter.Load(), "Expected only 1 request to the server")
	assert.Equal(t, int32(numRequests), errorCount.Load(), "All requests should receive the error response")
}

// TestDuplicateRequestsNonCacheable verifies that non-cacheable requests are NOT deduplicated
func TestDuplicateRequestsNonCacheable(t *testing.T) {
	counter := atomic.Int32{}

	mux := http.NewServeMux()
	mux.HandleFunc("/post", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	cache := newMockCache()
	transport := NewTransport(cache)
	client := http.Client{Transport: transport}

	const numRequests = 3
	var wg sync.WaitGroup
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			defer wg.Done()
			// POST requests are not cacheable
			req, _ := http.NewRequest(methodPOST, server.URL+"/post", nil)
			resp, err := client.Do(req)
			require.NoError(t, err)
			resp.Body.Close()
		}()
	}

	wg.Wait()

	// POST requests should NOT be deduplicated
	assert.Equal(t, int32(numRequests), counter.Load(), "POST requests should not be deduplicated")
}

// TestDuplicateRequestsBodyReading verifies that each goroutine can read the body independently
func TestDuplicateRequestsBodyReading(t *testing.T) {
	respondToRequest := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/data", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-respondToRequest
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write([]byte("test data content"))
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	cache := newMockCache()
	transport := NewTransport(cache)
	client := http.Client{Transport: transport}

	const numRequests = 4
	var wg sync.WaitGroup
	wg.Add(numRequests)
	results := make([]string, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(idx int) {
			defer wg.Done()
			req, _ := http.NewRequest(methodGET, server.URL+"/data", nil)
			resp, err := client.Do(req)
			require.NoError(t, err)

			// Each goroutine should be able to read the body
			body, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)
			results[idx] = string(body)
			resp.Body.Close()
		}(i)
	}

	time.Sleep(50 * time.Millisecond)
	close(respondToRequest)
	wg.Wait()

	// Verify all goroutines got the same content
	for i, result := range results {
		assert.Equal(t, "test data content", result, "Goroutine %d should read the full body", i)
	}
}
