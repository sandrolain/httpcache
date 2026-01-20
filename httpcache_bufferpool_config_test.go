package httpcache

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestMaxPooledBufferSize tests the configurable buffer pool size
func TestMaxPooledBufferSize(t *testing.T) {
	tests := []struct {
		name           string
		maxPooledSize  int64
		responseSize   int
		shouldBePooled bool
		description    string
	}{
		{
			name:           "default_size_small_response",
			maxPooledSize:  64 * 1024, // 64KB default
			responseSize:   32 * 1024, // 32KB
			shouldBePooled: true,
			description:    "32KB response should be pooled with 64KB limit",
		},
		{
			name:           "default_size_exact_match",
			maxPooledSize:  64 * 1024,
			responseSize:   64 * 1024,
			shouldBePooled: true,
			description:    "64KB response should be pooled with 64KB limit (exact match)",
		},
		{
			name:           "default_size_large_response",
			maxPooledSize:  64 * 1024,
			responseSize:   128 * 1024, // 128KB
			shouldBePooled: false,
			description:    "128KB response should NOT be pooled with 64KB limit",
		},
		{
			name:           "larger_limit_medium_response",
			maxPooledSize:  128 * 1024, // 128KB
			responseSize:   96 * 1024,  // 96KB
			shouldBePooled: true,
			description:    "96KB response should be pooled with 128KB limit",
		},
		{
			name:           "larger_limit_large_response",
			maxPooledSize:  128 * 1024,
			responseSize:   129 * 1024,
			shouldBePooled: false,
			description:    "129KB response should NOT be pooled with 128KB limit",
		},
		{
			name:           "small_limit_tiny_response",
			maxPooledSize:  16 * 1024, // 16KB
			responseSize:   8 * 1024,  // 8KB
			shouldBePooled: true,
			description:    "8KB response should be pooled with 16KB limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server that returns a response of the specified size
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "max-age=3600")
				w.WriteHeader(http.StatusOK)
				// Write response of specified size
				data := bytes.Repeat([]byte("x"), tt.responseSize)
				w.Write(data)
			}))
			defer server.Close()

			// Create transport with specified buffer pool size
			cache := &mockCache{items: make(map[string][]byte)}
			transport := NewTransport(cache,
				WithMaxPooledBufferSize(tt.maxPooledSize),
			)

			// Make request
			client := transport.Client()
			resp, err := client.Get(server.URL)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			// Read response body
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}

			if len(body) != tt.responseSize {
				t.Errorf("Expected response size %d, got %d", tt.responseSize, len(body))
			}

			// Verify transport configuration
			if transport.MaxPooledBufferSize != tt.maxPooledSize {
				t.Errorf("Expected MaxPooledBufferSize=%d, got %d", tt.maxPooledSize, transport.MaxPooledBufferSize)
			}

			t.Logf("✓ %s: Response size=%d, MaxPooled=%d, ShouldPool=%v",
				tt.description, tt.responseSize, tt.maxPooledSize, tt.shouldBePooled)
		})
	}
}

// TestMaxPooledBufferSizeDefault tests default buffer pool size
func TestMaxPooledBufferSizeDefault(t *testing.T) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache)

	expectedDefault := int64(64 * 1024) // 64KB
	if transport.MaxPooledBufferSize != expectedDefault {
		t.Errorf("Expected default MaxPooledBufferSize=%d, got %d", expectedDefault, transport.MaxPooledBufferSize)
	}
}

// TestMaxPooledBufferSizeZero tests buffer pooling disabled (size = 0)
func TestMaxPooledBufferSizeZero(t *testing.T) {
	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache,
		WithMaxPooledBufferSize(0),
	)

	if transport.MaxPooledBufferSize != 0 {
		t.Errorf("Expected MaxPooledBufferSize=0, got %d", transport.MaxPooledBufferSize)
	}

	// Verify transport still works with pooling disabled
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	client := transport.Client()
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if string(body) != "test response" {
		t.Errorf("Expected 'test response', got '%s'", string(body))
	}
}

// TestMaxPooledBufferSizeValidation tests validation of buffer pool size
func TestMaxPooledBufferSizeValidation(t *testing.T) {
	tests := []struct {
		name        string
		size        int64
		shouldError bool
		description string
	}{
		{
			name:        "valid_zero",
			size:        0,
			shouldError: false,
			description: "Zero is valid (disables pooling)",
		},
		{
			name:        "valid_1kb",
			size:        1024,
			shouldError: false,
			description: "1KB minimum is valid",
		},
		{
			name:        "valid_64kb",
			size:        64 * 1024,
			shouldError: false,
			description: "64KB default is valid",
		},
		{
			name:        "valid_large",
			size:        1024 * 1024, // 1MB
			shouldError: false,
			description: "1MB is valid",
		},
		{
			name:        "invalid_negative",
			size:        -1,
			shouldError: true,
			description: "Negative values should error",
		},
		{
			name:        "invalid_too_small",
			size:        512, // Less than 1KB
			shouldError: true,
			description: "Values below 1KB (except 0) should error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := &mockCache{items: make(map[string][]byte)}
			transport := &Transport{Cache: cache}

			opt := WithMaxPooledBufferSize(tt.size)
			err := opt(transport)

			if tt.shouldError && err == nil {
				t.Errorf("Expected error for size=%d, got nil", tt.size)
			}

			if !tt.shouldError && err != nil {
				t.Errorf("Expected no error for size=%d, got: %v", tt.size, err)
			}

			if !tt.shouldError && err == nil {
				if transport.MaxPooledBufferSize != tt.size {
					t.Errorf("Expected MaxPooledBufferSize=%d, got %d", tt.size, transport.MaxPooledBufferSize)
				}
			}

			t.Logf("✓ %s: size=%d, error=%v", tt.description, tt.size, err)
		})
	}
}

// TestMaxPooledBufferSizeConcurrent tests buffer pool under concurrent load
func TestMaxPooledBufferSizeConcurrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		// 80KB response - above 64KB default, below 128KB custom limit
		data := bytes.Repeat([]byte("x"), 80*1024)
		w.Write(data)
	}))
	defer server.Close()

	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache,
		WithMaxPooledBufferSize(128*1024), // 128KB limit
	)

	client := transport.Client()

	// Make concurrent requests
	const numRequests = 50
	results := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			resp, err := client.Get(server.URL)
			if err != nil {
				results <- err
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				results <- err
				return
			}

			if len(body) != 80*1024 {
				results <- err
				return
			}

			results <- nil
		}(i)
	}

	// Wait for all requests to complete
	var errors []error
	for i := 0; i < numRequests; i++ {
		if err := <-results; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		t.Errorf("Got %d errors out of %d requests: %v", len(errors), numRequests, errors[0])
	}

	t.Logf("✓ %d concurrent requests completed successfully with custom buffer pool size", numRequests)
}

// TestMaxPooledBufferSizeWithCachedResponse tests buffer pooling with cached responses
func TestMaxPooledBufferSizeWithCachedResponse(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		// 48KB response
		data := bytes.Repeat([]byte("y"), 48*1024)
		w.Write(data)
	}))
	defer server.Close()

	cache := &mockCache{items: make(map[string][]byte)}
	transport := NewTransport(cache,
		WithMaxPooledBufferSize(64*1024), // 64KB limit - should pool 48KB
	)

	client := transport.Client()

	// First request - cache miss
	resp1, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if len(body1) != 48*1024 {
		t.Errorf("Expected first response size 48KB, got %d", len(body1))
	}

	if requestCount != 1 {
		t.Errorf("Expected 1 server request, got %d", requestCount)
	}

	// Second request - cache hit
	time.Sleep(10 * time.Millisecond) // Small delay to ensure cache write completes
	resp2, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if len(body2) != 48*1024 {
		t.Errorf("Expected second response size 48KB, got %d", len(body2))
	}

	if requestCount != 1 {
		t.Errorf("Expected still 1 server request (cached), got %d", requestCount)
	}

	// Verify cache hit
	if resp2.Header.Get(XFromCache) != "1" {
		t.Error("Expected second response to be from cache")
	}

	t.Log("✓ Buffer pooling works correctly with cached responses")
}
