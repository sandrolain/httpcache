// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithLogger(t *testing.T) {
	cache := newMockCacheInternal()

	// Create a buffer to capture log output
	var buf bytes.Buffer
	testLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	transport := NewTransport(cache, WithLogger(testLogger))

	if transport.logger != testLogger {
		t.Error("WithLogger should set the logger on the transport")
	}
}

func TestTransportLogMethod(t *testing.T) {
	cache := newMockCacheInternal()

	// Create a transport with a custom logger
	var buf bytes.Buffer
	testLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	transport := NewTransport(cache, WithLogger(testLogger))

	// Verify log() returns the custom logger
	if transport.log() != testLogger {
		t.Error("log() should return the custom logger when set")
	}

	// Create a transport without a custom logger
	transport2 := NewTransport(cache)

	// Verify log() returns the global logger
	if transport2.log() == nil {
		t.Error("log() should return the global logger when no custom logger is set")
	}
}

func TestLoggerIntegration(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
	}))
	defer server.Close()

	// Create a buffer to capture log output
	var buf bytes.Buffer
	testLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	cache := newMockCacheInternal()
	transport := NewTransport(cache, WithLogger(testLogger))

	client := transport.Client()

	// Make a request
	resp, err := client.Get(server.URL + "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	// Read and discard body to trigger caching
	body := make([]byte, 1024)
	_, _ = resp.Body.Read(body)

	// Verify that logs were written
	logOutput := buf.String()
	if !strings.Contains(logOutput, "RoundTrip started") {
		t.Error("expected 'RoundTrip started' log message")
	}
	if !strings.Contains(logOutput, "cache miss") {
		t.Error("expected 'cache miss' log message")
	}
	if !strings.Contains(logOutput, "RoundTrip completed") {
		t.Error("expected 'RoundTrip completed' log message")
	}
}

func TestLoggerCacheHit(t *testing.T) {
	// Create a test server
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
	}))
	defer server.Close()

	// Create a buffer to capture log output
	var buf bytes.Buffer
	testLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	cache := newMockCacheInternal()
	transport := NewTransport(cache, WithLogger(testLogger))

	client := transport.Client()

	// First request - cache miss
	resp1, err := client.Get(server.URL + "/test")
	if err != nil {
		t.Fatalf("unexpected error on first request: %v", err)
	}
	body1 := make([]byte, 1024)
	_, _ = resp1.Body.Read(body1)
	resp1.Body.Close()

	// Reset buffer for second request
	buf.Reset()

	// Second request - cache hit
	resp2, err := client.Get(server.URL + "/test")
	if err != nil {
		t.Fatalf("unexpected error on second request: %v", err)
	}
	defer resp2.Body.Close()

	// Verify cache hit logs
	logOutput := buf.String()
	if !strings.Contains(logOutput, "cache hit") {
		t.Errorf("expected 'cache hit' log message, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "serving fresh response from cache") {
		t.Errorf("expected 'serving fresh response from cache' log message, got: %s", logOutput)
	}
}

func TestDefaultLoggerFallback(t *testing.T) {
	// Test that Transport without a custom logger falls back to slog.Default()
	cache := newMockCacheInternal()
	transport := NewTransport(cache)

	// The transport should use the default slog logger when no custom logger is set
	if transport.log() != slog.Default() {
		t.Error("transport.log() should return slog.Default() when no custom logger is set")
	}
}

func TestLoggerNilTransport(t *testing.T) {
	// Test that log() method handles nil Transport gracefully
	var t2 *Transport
	logger := t2.log()
	if logger == nil {
		t.Error("log() should return the default logger even for nil Transport")
	}
	if logger != slog.Default() {
		t.Error("log() should return slog.Default() for nil Transport")
	}
}
