package httpcache

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMaxCacheableResponseSize_WithinLimit tests that responses within the limit are cached
func TestMaxCacheableResponseSize_WithinLimit(t *testing.T) {
	cache := newMockCache()
	maxSize := int64(1024) // 1KB limit

	// Create a server that returns a small response
	smallBody := strings.Repeat("a", 512) // 512 bytes
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(smallBody))
	}))
	defer s.Close()

	tp := NewTransport(
		cache,
		WithMaxCacheableResponseSize(maxSize),
	)
	client := tp.Client()

	// Make the first request
	resp, err := client.Get(s.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if string(body) != smallBody {
		t.Errorf("Body mismatch: got %d bytes, want %d bytes", len(body), len(smallBody))
	}

	// Verify the response was cached
	req, _ := http.NewRequest("GET", s.URL, nil)
	cacheKey := cacheKey(req)
	hashedKey := hashKey(cacheKey)

	_, ok, err := cache.Get(context.Background(), hashedKey)
	if err != nil {
		t.Fatalf("Cache Get error: %v", err)
	}
	if !ok {
		t.Error("Response was not cached despite being within size limit")
	}
}

// TestMaxCacheableResponseSize_ExceedsLimit tests that responses exceeding the limit are not cached
func TestMaxCacheableResponseSize_ExceedsLimit(t *testing.T) {
	cache := newMockCache()
	maxSize := int64(512) // 512 bytes limit

	// Create a server that returns a large response
	largeBody := strings.Repeat("b", 1024) // 1KB, exceeds limit
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeBody))
	}))
	defer s.Close()

	tp := NewTransport(
		cache,
		WithMaxCacheableResponseSize(maxSize),
	)
	client := tp.Client()

	// Make the first request
	resp, err := client.Get(s.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if string(body) != largeBody {
		t.Errorf("Body mismatch: got %d bytes, want %d bytes", len(body), len(largeBody))
	}

	// Verify the response was NOT cached
	req, _ := http.NewRequest("GET", s.URL, nil)
	cacheKey := cacheKey(req)
	hashedKey := hashKey(cacheKey)

	_, ok, err := cache.Get(context.Background(), hashedKey)
	if err != nil {
		t.Fatalf("Cache Get error: %v", err)
	}
	if ok {
		t.Error("Response was cached despite exceeding size limit")
	}
}

// TestMaxCacheableResponseSize_ExactLimit tests behavior at the exact limit boundary
func TestMaxCacheableResponseSize_ExactLimit(t *testing.T) {
	cache := newMockCache()
	maxSize := int64(1024) // 1KB limit

	// Create a server that returns exactly the limit size
	exactBody := strings.Repeat("c", int(maxSize))
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(exactBody))
	}))
	defer s.Close()

	tp := NewTransport(
		cache,
		WithMaxCacheableResponseSize(maxSize),
	)
	client := tp.Client()

	// Make the first request
	resp, err := client.Get(s.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if len(body) != len(exactBody) {
		t.Errorf("Body length mismatch: got %d bytes, want %d bytes", len(body), len(exactBody))
	}

	// Verify the response was cached (at exact limit should still cache)
	req, _ := http.NewRequest("GET", s.URL, nil)
	cacheKey := cacheKey(req)
	hashedKey := hashKey(cacheKey)

	_, ok, err := cache.Get(context.Background(), hashedKey)
	if err != nil {
		t.Fatalf("Cache Get error: %v", err)
	}
	if !ok {
		t.Error("Response at exact size limit should be cached")
	}
}

// TestMaxCacheableResponseSize_ZeroMeansUnlimited tests that 0 means no limit
func TestMaxCacheableResponseSize_ZeroMeansUnlimited(t *testing.T) {
	cache := newMockCache()

	// Create a server that returns a very large response
	largeBody := strings.Repeat("d", 10*1024) // 10KB
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeBody))
	}))
	defer s.Close()

	tp := NewTransport(
		cache,
		WithMaxCacheableResponseSize(0), // No limit
	)
	client := tp.Client()

	// Make the first request
	resp, err := client.Get(s.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if string(body) != largeBody {
		t.Errorf("Body mismatch: got %d bytes, want %d bytes", len(body), len(largeBody))
	}

	// Verify the response was cached (no limit)
	req, _ := http.NewRequest("GET", s.URL, nil)
	cacheKey := cacheKey(req)
	hashedKey := hashKey(cacheKey)

	_, ok, err := cache.Get(context.Background(), hashedKey)
	if err != nil {
		t.Fatalf("Cache Get error: %v", err)
	}
	if !ok {
		t.Error("Response should be cached when limit is 0 (unlimited)")
	}
}

// TestMaxCacheableResponseSize_DefaultValue tests the default 10MB limit
func TestMaxCacheableResponseSize_DefaultValue(t *testing.T) {
	cache := newMockCache()
	tp := NewTransport(cache)

	expectedDefault := int64(10 * 1024 * 1024) // 10MB
	if tp.MaxCacheableResponseSize != expectedDefault {
		t.Errorf("Default MaxCacheableResponseSize = %d, want %d",
			tp.MaxCacheableResponseSize, expectedDefault)
	}
}

// TestMaxCacheableResponseSize_NegativeValue tests that negative values are rejected
func TestMaxCacheableResponseSize_NegativeValue(t *testing.T) {
	cache := newMockCache()

	// This should fail during option application
	tp := NewTransport(
		cache,
		WithMaxCacheableResponseSize(-100),
	)

	// Should fallback to default due to error
	expectedDefault := int64(10 * 1024 * 1024)
	if tp.MaxCacheableResponseSize != expectedDefault {
		t.Errorf("After invalid option, MaxCacheableResponseSize = %d, want default %d",
			tp.MaxCacheableResponseSize, expectedDefault)
	}
}

// TestCachingReadCloser_ExceedsLimit tests the low-level cachingReadCloser behavior
func TestCachingReadCloser_ExceedsLimit(t *testing.T) {
	data := []byte(strings.Repeat("test", 100)) // 400 bytes
	reader := io.NopCloser(bytes.NewReader(data))

	eofCalled := false
	exceededCalled := false
	var exceededSize int64

	crc := &cachingReadCloser{
		R:       reader,
		maxSize: 200, // Limit to 200 bytes
		OnEOF: func(r io.Reader) {
			eofCalled = true
		},
		OnExceeded: func(totalSize int64) {
			exceededCalled = true
			exceededSize = totalSize
		},
	}

	// Read all data
	result, err := io.ReadAll(crc)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if !bytes.Equal(result, data) {
		t.Error("Data mismatch after reading")
	}

	if eofCalled {
		t.Error("OnEOF should not be called when limit is exceeded")
	}

	if !exceededCalled {
		t.Error("OnExceeded should be called when limit is exceeded")
	}

	if exceededSize <= 200 {
		t.Errorf("exceededSize = %d, should be > 200", exceededSize)
	}
}

// TestCachingReadCloser_WithinLimit tests the low-level cachingReadCloser when within limit
func TestCachingReadCloser_WithinLimit(t *testing.T) {
	data := []byte(strings.Repeat("test", 10)) // 40 bytes
	reader := io.NopCloser(bytes.NewReader(data))

	eofCalled := false
	exceededCalled := false
	var capturedData []byte

	crc := &cachingReadCloser{
		R:       reader,
		maxSize: 100, // Limit to 100 bytes
		OnEOF: func(r io.Reader) {
			eofCalled = true
			capturedData, _ = io.ReadAll(r)
		},
		OnExceeded: func(totalSize int64) {
			exceededCalled = true
		},
	}

	// Read all data
	result, err := io.ReadAll(crc)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if !bytes.Equal(result, data) {
		t.Error("Data mismatch after reading")
	}

	if !eofCalled {
		t.Error("OnEOF should be called when within limit")
	}

	if exceededCalled {
		t.Error("OnExceeded should not be called when within limit")
	}

	if !bytes.Equal(capturedData, data) {
		t.Error("Captured data in OnEOF doesn't match original data")
	}
}

// TestCachingReadCloser_MemoryFreedOnExceed tests that buffer is freed when limit is exceeded
func TestCachingReadCloser_MemoryFreedOnExceed(t *testing.T) {
	// Create a large data source
	data := []byte(strings.Repeat("x", 1000)) // 1000 bytes
	reader := io.NopCloser(bytes.NewReader(data))

	exceededCalled := false

	crc := &cachingReadCloser{
		R:       reader,
		maxSize: 100, // Small limit
		OnExceeded: func(totalSize int64) {
			exceededCalled = true
		},
	}

	// Read all data
	_, err := io.ReadAll(crc)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if !exceededCalled {
		t.Fatal("OnExceeded should have been called")
	}

	// Check that buffer was reset (cap should be 0 after Reset)
	if crc.buf.Cap() != 0 {
		t.Errorf("Buffer capacity = %d, expected 0 (buffer should be reset)", crc.buf.Cap())
	}

	if crc.buf.Len() != 0 {
		t.Errorf("Buffer length = %d, expected 0 (buffer should be reset)", crc.buf.Len())
	}
}
