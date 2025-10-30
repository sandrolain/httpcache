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
			expectedMin: 19, // RFC 9111: apparent_age(10) + resident_time(10) = ~20
			expectedMax: 21,
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

// TestParseAgeHeaderValid tests parseAgeHeader with valid Age values
func TestParseAgeHeaderValid(t *testing.T) {
	tests := []struct {
		name     string
		ageValue string
		want     time.Duration
	}{
		{
			name:     "zero age",
			ageValue: "0",
			want:     0,
		},
		{
			name:     "positive age",
			ageValue: "3600",
			want:     3600 * time.Second,
		},
		{
			name:     "large age",
			ageValue: "86400",
			want:     86400 * time.Second,
		},
		{
			name:     "age with whitespace",
			ageValue: "  300  ",
			want:     300 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set(headerAge, tt.ageValue)

			got, valid := parseAgeHeader(headers)
			if !valid {
				t.Errorf("parseAgeHeader() valid = %v, want true", valid)
				return
			}
			if got != tt.want {
				t.Errorf("parseAgeHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseAgeHeaderInvalid tests parseAgeHeader with invalid Age values
func TestParseAgeHeaderInvalid(t *testing.T) {
	tests := []struct {
		name     string
		ageValue string
	}{
		{
			name:     "negative age",
			ageValue: "-100",
		},
		{
			name:     "non-numeric age",
			ageValue: "invalid",
		},
		{
			name:     "float age",
			ageValue: "3600.5",
		},
		{
			name:     "empty age",
			ageValue: "",
		},
		{
			name:     "whitespace only",
			ageValue: "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			if tt.ageValue != "" {
				headers.Set(headerAge, tt.ageValue)
			}

			got, valid := parseAgeHeader(headers)
			if valid {
				t.Errorf("parseAgeHeader() valid = true, want false for value %q", tt.ageValue)
			}
			if got != 0 {
				t.Errorf("parseAgeHeader() = %v, want 0 for invalid value", got)
			}
		})
	}
}

// TestParseAgeHeaderMultipleValues tests parseAgeHeader with multiple Age headers
func TestParseAgeHeaderMultipleValues(t *testing.T) {
	headers := http.Header{}
	headers.Add(headerAge, "300")
	headers.Add(headerAge, "600")
	headers.Add(headerAge, "900")

	got, valid := parseAgeHeader(headers)
	if !valid {
		t.Errorf("parseAgeHeader() valid = false, want true")
		return
	}

	// RFC 9111: Should use the first value
	want := 300 * time.Second
	if got != want {
		t.Errorf("parseAgeHeader() = %v, want %v (first value)", got, want)
	}
}

// TestParseAgeHeaderNoAgeHeader tests parseAgeHeader with no Age header
func TestParseAgeHeaderNoAgeHeader(t *testing.T) {
	headers := http.Header{}

	got, valid := parseAgeHeader(headers)
	if valid {
		t.Errorf("parseAgeHeader() valid = true, want false for missing header")
	}
	if got != 0 {
		t.Errorf("parseAgeHeader() = %v, want 0 for missing header", got)
	}
}

// TestCalculateAgeWithRequestAndResponseTime tests the full RFC 9111 formula
func TestCalculateAgeWithRequestAndResponseTime(t *testing.T) {
	now := time.Now().UTC()
	requestTime := now.Add(-10 * time.Second)
	responseTime := now.Add(-8 * time.Second)
	dateValue := now.Add(-12 * time.Second)

	headers := http.Header{}
	headers.Set("Date", dateValue.Format(http.TimeFormat))
	headers.Set(XRequestTime, requestTime.Format(time.RFC3339))
	headers.Set(XResponseTime, responseTime.Format(time.RFC3339))
	headers.Set(headerAge, "5") // Age from origin server

	age, err := calculateAge(headers)
	if err != nil {
		t.Fatalf("calculateAge() error = %v", err)
	}

	// RFC 9111 formula:
	// apparent_age = max(0, response_time - date_value) = max(0, -8 - (-12)) = 4 seconds
	// response_delay = response_time - request_time = -8 - (-10) = 2 seconds
	// corrected_age_value = age_value + response_delay = 5 + 2 = 7 seconds
	// corrected_initial_age = max(apparent_age, corrected_age_value) = max(4, 7) = 7 seconds
	// resident_time = now - response_time = 0 - (-8) = 8 seconds
	// current_age = corrected_initial_age + resident_time = 7 + 8 = 15 seconds

	expectedAge := 15 * time.Second
	// Allow 1 second tolerance for test execution time
	if age < expectedAge-time.Second || age > expectedAge+time.Second {
		t.Errorf("calculateAge() = %v, want ~%v", age, expectedAge)
	}
}

// TestCalculateAgeWithoutRequestTime tests Age calculation without request_time
func TestCalculateAgeWithoutRequestTime(t *testing.T) {
	now := time.Now().UTC()
	responseTime := now.Add(-10 * time.Second)
	dateValue := now.Add(-15 * time.Second)

	headers := http.Header{}
	headers.Set("Date", dateValue.Format(http.TimeFormat))
	headers.Set(XResponseTime, responseTime.Format(time.RFC3339))
	headers.Set(headerAge, "3")

	age, err := calculateAge(headers)
	if err != nil {
		t.Fatalf("calculateAge() error = %v", err)
	}

	// Without request_time, response_delay = 0
	// apparent_age = max(0, response_time - date_value) = max(0, -10 - (-15)) = 5 seconds
	// corrected_age_value = age_value + 0 = 3 seconds
	// corrected_initial_age = max(5, 3) = 5 seconds
	// resident_time = now - response_time = 0 - (-10) = 10 seconds
	// current_age = 5 + 10 = 15 seconds

	expectedAge := 15 * time.Second
	if age < expectedAge-time.Second || age > expectedAge+time.Second {
		t.Errorf("calculateAge() = %v, want ~%v", age, expectedAge)
	}
}

// TestCalculateAgeBackwardCompatibility tests backward compatibility with X-Cached-Time
func TestCalculateAgeBackwardCompatibility(t *testing.T) {
	now := time.Now().UTC()
	cachedTime := now.Add(-20 * time.Second)
	dateValue := now.Add(-25 * time.Second)

	headers := http.Header{}
	headers.Set("Date", dateValue.Format(http.TimeFormat))
	headers.Set(XCachedTime, cachedTime.Format(time.RFC3339))
	headers.Set(headerAge, "8")

	age, err := calculateAge(headers)
	if err != nil {
		t.Fatalf("calculateAge() error = %v", err)
	}

	// Falls back to X-Cached-Time when X-Response-Time not present
	// Should calculate correctly using X-Cached-Time as response_time
	expectedAge := 28 * time.Second // approximate
	if age < expectedAge-2*time.Second || age > expectedAge+2*time.Second {
		t.Errorf("calculateAge() = %v, want ~%v", age, expectedAge)
	}
}

// TestCalculateAgeClockSkew tests handling of clock skew (Date > response_time)
func TestCalculateAgeClockSkew(t *testing.T) {
	now := time.Now().UTC()
	responseTime := now.Add(-5 * time.Second)
	dateValue := now // Date is AFTER response_time (clock skew)

	headers := http.Header{}
	headers.Set("Date", dateValue.Format(http.TimeFormat))
	headers.Set(XResponseTime, responseTime.Format(time.RFC3339))
	headers.Set(headerAge, "0")

	age, err := calculateAge(headers)
	if err != nil {
		t.Fatalf("calculateAge() error = %v", err)
	}

	// apparent_age = max(0, response_time - date_value) = max(0, -5) = 0
	// Should handle gracefully and not return negative age
	if age < 0 {
		t.Errorf("calculateAge() = %v, must not be negative", age)
	}
}

// TestCalculateAgeResponseDelayCalculation tests the response_delay component
func TestCalculateAgeResponseDelayCalculation(t *testing.T) {
	now := time.Now().UTC()
	requestTime := now.Add(-10 * time.Second)
	responseTime := now.Add(-7 * time.Second) // 3 seconds response delay
	dateValue := now.Add(-8 * time.Second)

	headers := http.Header{}
	headers.Set("Date", dateValue.Format(http.TimeFormat))
	headers.Set(XRequestTime, requestTime.Format(time.RFC3339))
	headers.Set(XResponseTime, responseTime.Format(time.RFC3339))
	headers.Set(headerAge, "0") // No age from origin

	age, err := calculateAge(headers)
	if err != nil {
		t.Fatalf("calculateAge() error = %v", err)
	}

	// RFC 9111 formula verification:
	// apparent_age = max(0, -7 - (-8)) = 1 second
	// response_delay = -7 - (-10) = 3 seconds
	// corrected_age_value = 0 + 3 = 3 seconds
	// corrected_initial_age = max(1, 3) = 3 seconds
	// resident_time = now - (-7) = 7 seconds
	// current_age = 3 + 7 = 10 seconds

	expectedAge := 10 * time.Second
	if age < expectedAge-time.Second || age > expectedAge+time.Second {
		t.Errorf("calculateAge() = %v, want ~%v (response_delay should be included)", age, expectedAge)
	}
}
