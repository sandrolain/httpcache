package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// TestAgeHeader verifies that Age header is correctly calculated and set
func TestAgeHeader(t *testing.T) {
	resetTest()

	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()
	tp.MarkCachedResponses = true
	client := &http.Client{Transport: tp}

	// First request - not cached
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.Header.Get(XFromCache) != "" {
		t.Fatal("First request should not be from cache")
	}

	if resp.Header.Get(headerAge) != "" {
		t.Fatal("First request should not have Age header")
	}

	// Wait a bit before second request
	time.Sleep(2 * time.Second)

	// Second request - from cache
	resp2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if resp2.Header.Get(XFromCache) != "1" {
		t.Fatal("Second request should be from cache")
	}

	// Age header should be present and ~2 seconds
	ageStr := resp2.Header.Get(headerAge)
	if ageStr == "" {
		t.Fatal("Age header should be present on cached response")
	}

	age, err := strconv.ParseInt(ageStr, 10, 64)
	if err != nil {
		t.Fatalf("Failed to parse Age header: %v", err)
	}

	// Age should be approximately 2 seconds (allow some tolerance)
	if age < 1 || age > 4 {
		t.Fatalf("Age should be ~2 seconds, got %d", age)
	}

	// Verify counter - should only have hit server once
	if counter != 1 {
		t.Fatalf("Expected 1 server hit, got %d", counter)
	}
}

// TestAgeHeaderWithRevalidation verifies Age header on 304 Not Modified
func TestAgeHeaderWithRevalidation(t *testing.T) {
	resetTest()

	counter := 0
	etag := `"test-etag"`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		if r.Header.Get("If-None-Match") == etag {
			// Return 304 Not Modified
			w.WriteHeader(http.StatusNotModified)
			return
		}
		// First request
		w.Header().Set("Cache-Control", "max-age=1")
		w.Header().Set("ETag", etag)
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()
	tp.MarkCachedResponses = true
	client := &http.Client{Transport: tp}

	// First request
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Wait for cache to become stale
	time.Sleep(2 * time.Second)

	// Second request - should revalidate with 304
	clock = &fakeClock{elapsed: 2 * time.Second}
	defer func() { clock = &realClock{} }()

	resp2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if resp2.Header.Get(XFromCache) != "1" {
		t.Fatal("Second request should be from cache")
	}

	if resp2.Header.Get(XRevalidated) != "1" {
		t.Fatal("Second request should be revalidated")
	}

	// Age header should be present after revalidation
	ageStr := resp2.Header.Get(headerAge)
	if ageStr == "" {
		t.Fatal("Age header should be present after revalidation")
	}

	age, err := strconv.ParseInt(ageStr, 10, 64)
	if err != nil {
		t.Fatalf("Failed to parse Age header: %v", err)
	}

	// Age should be approximately 2 seconds
	if age < 1 || age > 4 {
		t.Fatalf("Age should be ~2 seconds after revalidation, got %d", age)
	}

	// Should have hit server twice (initial + revalidation)
	if counter != 2 {
		t.Fatalf("Expected 2 server hits, got %d", counter)
	}
}

// TestCalculateAge tests the calculateAge function directly
func TestCalculateAge(t *testing.T) {
	resetTest()

	now := time.Now().UTC()

	tests := []struct {
		name        string
		dateHeader  string
		cachedTime  string
		ageHeader   string
		expectedMin int64
		expectedMax int64
		shouldError bool
	}{
		{
			name:        "No Date header",
			dateHeader:  "",
			shouldError: true,
		},
		{
			name:        "Fresh response without cached time",
			dateHeader:  now.Add(-10 * time.Second).Format(time.RFC1123),
			expectedMin: 9,
			expectedMax: 11,
		},
		{
			name:        "With cached time",
			dateHeader:  now.Add(-20 * time.Second).Format(time.RFC1123),
			cachedTime:  now.Add(-10 * time.Second).Format(time.RFC3339),
			expectedMin: 9,
			expectedMax: 11,
		},
		{
			name:        "With existing Age header",
			dateHeader:  now.Add(-10 * time.Second).Format(time.RFC1123),
			ageHeader:   "5",
			cachedTime:  now.Add(-5 * time.Second).Format(time.RFC3339),
			expectedMin: 9,
			expectedMax: 11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}

			if tt.dateHeader != "" {
				headers.Set("Date", tt.dateHeader)
			}
			if tt.cachedTime != "" {
				headers.Set(XCachedTime, tt.cachedTime)
			}
			if tt.ageHeader != "" {
				headers.Set(headerAge, tt.ageHeader)
			}

			age, err := calculateAge(headers)

			if tt.shouldError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			ageSeconds := int64(age.Seconds())
			if ageSeconds < tt.expectedMin || ageSeconds > tt.expectedMax {
				t.Fatalf("Age %d not in expected range [%d, %d]", ageSeconds, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

// TestFormatAge tests the formatAge function
func TestFormatAge(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{0, "0"},
		{1 * time.Second, "1"},
		{10 * time.Second, "10"},
		{3600 * time.Second, "3600"},
		{-5 * time.Second, "0"}, // Negative should be 0
	}

	for _, tt := range tests {
		result := formatAge(tt.duration)
		if result != tt.expected {
			t.Errorf("formatAge(%v) = %q, want %q", tt.duration, result, tt.expected)
		}
	}
}

// TestAgeHeaderNotOnFreshResponse verifies Age header is not added to fresh responses from server
func TestAgeHeaderNotOnFreshResponse(t *testing.T) {
	resetTest()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))
		w.Write([]byte("test"))
	}))
	defer ts.Close()

	tp := NewMemoryCacheTransport()
	client := &http.Client{Transport: tp}

	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Fresh response from server should not have Age header
	if resp.Header.Get(headerAge) != "" {
		t.Fatal("Fresh response from server should not have Age header")
	}
}
