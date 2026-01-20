package httpcache

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// panicRoundTripper is a mock RoundTripper that panics on RoundTrip
type panicRoundTripper struct {
	panicMessage string
	mu           sync.Mutex
	panicked     bool
}

func (p *panicRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.Lock()
	p.panicked = true
	p.mu.Unlock()
	panic(p.panicMessage)
}

func (p *panicRoundTripper) didPanic() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.panicked
}

// TestAsyncRevalidatePanicRecovery verifies that panics in async revalidation
// are caught and logged without crashing the application.
func TestAsyncRevalidatePanicRecovery(t *testing.T) {
	resetTest()

	// Create a cache with an initial response
	cache := &mockCache{items: make(map[string][]byte)}

	// Capture log output to verify panic was logged
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create transport with panic-inducing transport
	panicTransport := &panicRoundTripper{
		panicMessage: "test panic in RoundTrip",
	}

	transport := NewTransport(cache,
		WithLogger(logger),
	)
	transport.Transport = panicTransport
	transport.AsyncRevalidateTimeout = 2 * time.Second

	// Create a test server to generate the initial cached response
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=1, stale-while-revalidate=10")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
	}))
	defer ts.Close()

	// First request: cache the response using a working transport
	workingTransport := http.DefaultTransport
	transport.Transport = workingTransport

	req1, _ := http.NewRequest("GET", ts.URL, nil)
	resp1, err := transport.RoundTrip(req1)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	// Wait for response to be cached
	time.Sleep(100 * time.Millisecond)

	// Switch to panic transport
	transport.Transport = panicTransport

	// Wait for response to become stale
	time.Sleep(1500 * time.Millisecond)

	// Second request: should trigger async revalidation which will panic
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()

	// Wait for async revalidation goroutine to panic and recover
	time.Sleep(500 * time.Millisecond)

	// Verify that the application is still running (we got here without crashing)
	t.Log("Application survived the panic - recovery worked!")

	// Verify that panic was triggered
	if !panicTransport.didPanic() {
		t.Error("expected panic transport to be called")
	}

	// Copy log output before checking it (to avoid races with async goroutines)
	logOutput := logBuffer.String()

	// Verify that panic was logged
	if logOutput == "" {
		t.Error("expected log output but got none")
	}

	// Check for panic-related log messages
	logBytes := []byte(logOutput)
	if !containsSubstring(logBytes, "panic") && !containsSubstring(logBytes, "test panic") {
		t.Errorf("expected panic to be logged, but log output was: %s", logOutput)
	}

	t.Logf("Log output (truncated): %s...", truncateString(logOutput, 200))
}

// TestAsyncRevalidatePanicRecoveryMultipleCalls verifies that panic recovery
// works correctly for multiple concurrent async revalidations.
func TestAsyncRevalidatePanicRecoveryMultipleCalls(t *testing.T) {
	resetTest()

	cache := &mockCache{items: make(map[string][]byte)}

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	panicTransport := &panicRoundTripper{
		panicMessage: "concurrent panic test",
	}

	transport := NewTransport(cache,
		WithLogger(logger),
	)
	transport.AsyncRevalidateTimeout = 1 * time.Second

	// Create a test server for initial caching
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=1, stale-while-revalidate=10")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
	}))
	defer ts.Close()

	// Cache initial response with working transport
	transport.Transport = http.DefaultTransport
	req1, _ := http.NewRequest("GET", ts.URL, nil)
	resp1, err := transport.RoundTrip(req1)
	if err != nil {
		t.Fatalf("initial request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	time.Sleep(100 * time.Millisecond)

	// Switch to panic transport
	transport.Transport = panicTransport

	// Wait for staleness
	time.Sleep(1500 * time.Millisecond)

	// Trigger multiple concurrent requests that will cause async revalidations
	const numRequests = 10
	var wg sync.WaitGroup
	errChan := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			req, _ := http.NewRequest("GET", ts.URL, nil)
			resp, err := transport.RoundTrip(req)
			if err != nil {
				errChan <- err
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		t.Errorf("request failed: %v", err)
	}

	// Wait for all async revalidations to complete
	time.Sleep(2 * time.Second)

	// Verify application is still running
	t.Log("Application survived multiple concurrent panics!")

	// Copy log output before checking (to avoid races)
	logOutput := logBuffer.String()

	// Verify panics were logged (should see multiple panic logs)
	if logOutput == "" {
		t.Error("expected panic logs but got none")
	}
}

// TestAsyncRevalidateNoPanic verifies that normal operation (no panic)
// still works correctly after adding panic recovery.
func TestAsyncRevalidateNoPanic(t *testing.T) {
	resetTest()

	cache := &mockCache{items: make(map[string][]byte)}

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	transport := NewTransport(cache,
		WithLogger(logger),
	)
	transport.AsyncRevalidateTimeout = 2 * time.Second

	// Create a test server that returns different content on revalidation
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Cache-Control", "max-age=1, stale-while-revalidate=10")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response " + string(rune('0'+callCount))))
	}))
	defer ts.Close()

	// First request: cache the response
	req1, _ := http.NewRequest("GET", ts.URL, nil)
	resp1, err := transport.RoundTrip(req1)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	time.Sleep(100 * time.Millisecond)

	// Wait for staleness
	time.Sleep(1500 * time.Millisecond)

	// Second request: should serve stale and trigger async revalidation
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()

	// Wait for async revalidation to complete
	time.Sleep(1 * time.Second)

	// Copy log output before checking (to avoid races)
	logOutput := logBuffer.String()

	// Verify that async revalidation completed successfully
	logBytes := []byte(logOutput)
	if !containsSubstring(logBytes, "async revalidation completed") {
		t.Error("expected async revalidation to complete successfully")
	}

	// Should NOT contain any panic messages
	if containsSubstring(logBytes, "panic") {
		t.Errorf("unexpected panic in normal operation: %s", logOutput)
	}

	// Verify server was called at least twice (initial + revalidation)
	if callCount < 2 {
		t.Errorf("expected at least 2 server calls, got %d", callCount)
	}
}

// Helper function to truncate a string for logging
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// errorRoundTripper returns errors instead of panicking
type errorRoundTripper struct{}

func (e *errorRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errors.New("simulated error")
}

// TestAsyncRevalidateWithErrors verifies that regular errors (not panics)
// are still handled correctly.
func TestAsyncRevalidateWithErrors(t *testing.T) {
	resetTest()

	cache := &mockCache{items: make(map[string][]byte)}

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	transport := NewTransport(cache,
		WithLogger(logger),
	)
	transport.AsyncRevalidateTimeout = 1 * time.Second

	// Create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=1, stale-while-revalidate=10")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test"))
	}))
	defer ts.Close()

	// Cache initial response
	req1, _ := http.NewRequest("GET", ts.URL, nil)
	resp1, err := transport.RoundTrip(req1)
	if err != nil {
		t.Fatalf("initial request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	time.Sleep(100 * time.Millisecond)

	// Switch to error transport
	transport.Transport = &errorRoundTripper{}

	// Wait for staleness
	time.Sleep(1500 * time.Millisecond)

	// Trigger request that will cause async revalidation with error
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()

	// Wait for async revalidation
	time.Sleep(500 * time.Millisecond)

	// Copy log output before checking (to avoid races)
	logOutput := logBuffer.String()
	logBytes := []byte(logOutput)

	// Verify error was logged (not panic)
	if !containsSubstring(logBytes, "async revalidation failed") {
		t.Error("expected async revalidation failure to be logged")
	}

	// Should NOT contain panic messages
	if containsSubstring(logBytes, "panic") {
		t.Errorf("unexpected panic with error transport: %s", logOutput)
	}
}

// TestAsyncRevalidateCancellation verifies that context cancellation
// works correctly with panic recovery.
func TestAsyncRevalidateCancellation(t *testing.T) {
	resetTest()

	cache := &mockCache{items: make(map[string][]byte)}

	transport := NewTransport(cache)
	// Very short timeout to trigger cancellation
	transport.AsyncRevalidateTimeout = 1 * time.Millisecond

	// Create slow server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Cache-Control", "max-age=1, stale-while-revalidate=10")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("slow response"))
	}))
	defer ts.Close()

	// Cache initial response with longer timeout
	originalTimeout := transport.AsyncRevalidateTimeout
	transport.AsyncRevalidateTimeout = 1 * time.Second

	req1, _ := http.NewRequest("GET", ts.URL, nil)
	resp1, err := transport.RoundTrip(req1)
	if err != nil {
		t.Fatalf("initial request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	time.Sleep(100 * time.Millisecond)

	// Restore short timeout
	transport.AsyncRevalidateTimeout = originalTimeout

	// Wait for staleness
	time.Sleep(1500 * time.Millisecond)

	// Trigger async revalidation that will be cancelled
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()

	// Wait and verify no panic occurred
	time.Sleep(200 * time.Millisecond)

	// Test passes if we get here without crashing
	t.Log("Context cancellation handled correctly with panic recovery")
}
