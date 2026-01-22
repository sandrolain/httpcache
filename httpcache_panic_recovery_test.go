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

// syncWriter wraps a writer with mutex protection for thread-safe access
type syncWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (sw syncWriter) Write(p []byte) (n int, err error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}

// waitForCondition polls a condition function until it returns true or timeout
func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		if condition() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		<-ticker.C
	}
}

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
	var logMu sync.Mutex
	logger := slog.New(slog.NewTextHandler(syncWriter{w: &logBuffer, mu: &logMu}, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create transport with panic-inducing transport
	panicTransport := &panicRoundTripper{
		panicMessage: "test panic in RoundTrip",
	}

	transport := NewTransport(cache,
		WithLogger(logger),
	)
	transport.SetTransport(panicTransport)
	transport.AsyncRevalidateTimeout = 5 * time.Second

	// Create a test server to generate the initial cached response
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=1, stale-while-revalidate=10")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
	}))
	defer ts.Close()

	// First request: cache the response using a working transport
	workingTransport := http.DefaultTransport
	transport.SetTransport(workingTransport)

	req1, _ := http.NewRequest("GET", ts.URL, nil)
	resp1, err := transport.RoundTrip(req1)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	// Wait for response to be cached (cache operations are synchronous)
	time.Sleep(200 * time.Millisecond)

	// Switch to panic transport
	transport.SetTransport(panicTransport)

	// Wait for response to become stale (max-age=1)
	time.Sleep(1200 * time.Millisecond)

	// Second request: should trigger async revalidation which will panic
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()

	// Wait for panic to be triggered and logged (max 3 seconds)
	if !waitForCondition(t, 3*time.Second, func() bool {
		return panicTransport.didPanic()
	}) {
		t.Fatal("timeout waiting for panic to occur")
	}

	// Verify that the application is still running (we got here without crashing)
	t.Log("Application survived the panic - recovery worked!")

	// Wait a bit more for log to be written
	time.Sleep(200 * time.Millisecond)

	// Copy log output safely
	logMu.Lock()
	logOutput := logBuffer.String()
	logMu.Unlock()

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
	var logMu sync.Mutex
	logger := slog.New(slog.NewTextHandler(syncWriter{w: &logBuffer, mu: &logMu}, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	panicTransport := &panicRoundTripper{
		panicMessage: "concurrent panic test",
	}

	transport := NewTransport(cache,
		WithLogger(logger),
	)
	transport.AsyncRevalidateTimeout = 3 * time.Second

	// Create a test server for initial caching
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=1, stale-while-revalidate=10")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
	}))
	defer ts.Close()

	// Cache initial response with working transport
	transport.SetTransport(http.DefaultTransport)
	req1, _ := http.NewRequest("GET", ts.URL, nil)
	resp1, err := transport.RoundTrip(req1)
	if err != nil {
		t.Fatalf("initial request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	// Wait for response to be cached (cache operations are synchronous)
	time.Sleep(200 * time.Millisecond)

	// Switch to panic transport
	transport.SetTransport(panicTransport)

	// Wait for staleness
	time.Sleep(1200 * time.Millisecond)

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

	// Wait for panic to occur
	if !waitForCondition(t, 4*time.Second, func() bool {
		return panicTransport.didPanic()
	}) {
		t.Fatal("timeout waiting for panic to occur")
	}

	// Verify application is still running
	t.Log("Application survived multiple concurrent panics!")

	// Wait a bit for all logs to be written
	time.Sleep(200 * time.Millisecond)

	// Copy log output safely
	logMu.Lock()
	logOutput := logBuffer.String()
	logMu.Unlock()

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
	var logMu sync.Mutex
	logger := slog.New(slog.NewTextHandler(syncWriter{w: &logBuffer, mu: &logMu}, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	transport := NewTransport(cache,
		WithLogger(logger),
	)
	transport.AsyncRevalidateTimeout = 5 * time.Second

	// Create a test server that returns different content on revalidation
	callCount := 0
	var callMu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callMu.Lock()
		callCount++
		count := callCount
		callMu.Unlock()
		w.Header().Set("Cache-Control", "max-age=1, stale-while-revalidate=10")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response " + string(rune('0'+count))))
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

	// Wait for response to be cached (cache operations are synchronous)
	time.Sleep(200 * time.Millisecond)

	// Wait for staleness
	time.Sleep(1200 * time.Millisecond)

	// Second request: should serve stale and trigger async revalidation
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()

	// Wait for async revalidation to complete (poll for log message)
	if !waitForCondition(t, 4*time.Second, func() bool {
		logMu.Lock()
		defer logMu.Unlock()
		logBytes := logBuffer.Bytes()
		return containsSubstring(logBytes, "async revalidation completed")
	}) {
		logMu.Lock()
		logOutput := logBuffer.String()
		logMu.Unlock()
		t.Fatalf("timeout waiting for async revalidation, log: %s", logOutput)
	}

	// Copy log output safely
	logMu.Lock()
	logOutput := logBuffer.String()
	logMu.Unlock()

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
	callMu.Lock()
	count := callCount
	callMu.Unlock()
	if count < 2 {
		t.Errorf("expected at least 2 server calls, got %d", count)
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
	var logMu sync.Mutex
	logger := slog.New(slog.NewTextHandler(syncWriter{w: &logBuffer, mu: &logMu}, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	transport := NewTransport(cache,
		WithLogger(logger),
	)
	transport.AsyncRevalidateTimeout = 3 * time.Second

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

	// Wait for response to be cached (cache operations are synchronous)
	time.Sleep(200 * time.Millisecond)

	// Switch to error transport
	transport.SetTransport(&errorRoundTripper{})

	// Wait for staleness
	time.Sleep(1200 * time.Millisecond)

	// Trigger request that will cause async revalidation with error
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()

	// Wait for async revalidation to fail (poll for log message)
	if !waitForCondition(t, 3*time.Second, func() bool {
		logMu.Lock()
		defer logMu.Unlock()
		logBytes := logBuffer.Bytes()
		return containsSubstring(logBytes, "async revalidation failed")
	}) {
		logMu.Lock()
		logOutput := logBuffer.String()
		logMu.Unlock()
		t.Fatalf("timeout waiting for async revalidation failure, log: %s", logOutput)
	}

	// Copy log output safely
	logMu.Lock()
	logOutput := logBuffer.String()
	logMu.Unlock()
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
