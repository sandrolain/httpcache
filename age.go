// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrNoDateHeader indicates that the HTTP headers contained no Date header.
var ErrNoDateHeader = errors.New("no Date header")

// Date parses and returns the value of the Date header.
func Date(respHeaders http.Header) (date time.Time, err error) {
	dateHeader := respHeaders.Get("date")
	if dateHeader == "" {
		err = ErrNoDateHeader
		return
	}

	return time.Parse(time.RFC1123, dateHeader)
}

// parseAgeHeader parses the Age header according to RFC 9111 Section 5.1.
// Returns the age duration and a boolean indicating if the header is valid.
//
// RFC 9111 requirements:
// - If multiple Age headers exist, use the first value and discard others
// - If the value is invalid (negative, non-numeric), ignore it completely
// - Age header value must be a non-negative integer representing seconds
func parseAgeHeader(headers http.Header, log *slog.Logger) (age time.Duration, valid bool) {
	ageValues := headers.Values(headerAge)

	if len(ageValues) == 0 {
		return 0, false
	}

	// RFC 9111: use the first value, discard others
	ageStr := strings.TrimSpace(ageValues[0])

	if len(ageValues) > 1 {
		log.Warn("multiple Age headers detected, using first value",
			"count", len(ageValues),
			"first", ageStr,
			"all", ageValues)
	}

	// Validate that it's a non-negative integer
	ageInt, err := strconv.ParseInt(ageStr, 10, 64)
	if err != nil {
		log.Warn("invalid Age header value, ignoring",
			"value", ageStr,
			"error", err)
		return 0, false
	}

	if ageInt < 0 {
		log.Warn("negative Age header value, ignoring",
			"value", ageInt)
		return 0, false
	}

	return time.Duration(ageInt) * time.Second, true
}

// calculateAge implements the Age calculation algorithm from RFC 9111 Section 4.2.3.
//
// RFC 9111 formula:
//
//	apparent_age = max(0, response_time - date_value)
//	response_delay = response_time - request_time
//	corrected_age_value = age_value + response_delay
//	corrected_initial_age = max(apparent_age, corrected_age_value)
//	resident_time = now - response_time
//	current_age = corrected_initial_age + resident_time
//
// For cached responses:
//   - request_time is stored in X-Request-Time header
//   - response_time is stored in X-Response-Time header (falls back to X-Cached-Time for compatibility)
//   - date_value comes from Date header
//   - age_value comes from Age header (if present)
func calculateAge(respHeaders http.Header, log *slog.Logger) (age time.Duration, err error) {
	// Get the Date header (required)
	dateValue, err := Date(respHeaders)
	if err != nil {
		return 0, err
	}

	// Get response_time (when we received the response)
	// Try X-Response-Time first, fall back to X-Cached-Time for backward compatibility
	responseTimeStr := respHeaders.Get(XResponseTime)
	if responseTimeStr == "" {
		responseTimeStr = respHeaders.Get(XCachedTime)
	}

	if responseTimeStr == "" {
		// If no cached time, use simplified calculation
		age = clock.since(dateValue)

		// Add any existing Age header
		if ageValue, valid := parseAgeHeader(respHeaders, log); valid {
			age += ageValue
		}

		return age, nil
	}

	// Parse response_time
	responseTime, parseErr := time.Parse(time.RFC3339, responseTimeStr)
	if parseErr != nil {
		log.Warn("failed to parse response time header",
			"header", responseTimeStr,
			"error", parseErr)

		// Fallback to simplified calculation
		age = clock.since(dateValue)
		if ageValue, valid := parseAgeHeader(respHeaders, log); valid {
			age += ageValue
		}
		return age, nil
	}

	// RFC 9111 Section 4.2.3: apparent_age = max(0, response_time - date_value)
	apparentAge := time.Duration(0)
	if responseTime.After(dateValue) {
		apparentAge = responseTime.Sub(dateValue)
	}

	// Parse age_value from Age header (if present)
	ageValue, _ := parseAgeHeader(respHeaders, log)

	// Get request_time (when we started the request)
	requestTimeStr := respHeaders.Get(XRequestTime)
	responseDelay := time.Duration(0)

	if requestTimeStr != "" {
		requestTime, parseErr := time.Parse(time.RFC3339, requestTimeStr)
		if parseErr == nil && responseTime.After(requestTime) {
			// RFC 9111: response_delay = response_time - request_time
			responseDelay = responseTime.Sub(requestTime)
		} else if parseErr != nil {
			log.Warn("failed to parse request time header",
				"header", requestTimeStr,
				"error", parseErr)
		}
	}

	// RFC 9111: corrected_age_value = age_value + response_delay
	correctedAgeValue := ageValue + responseDelay

	// RFC 9111: corrected_initial_age = max(apparent_age, corrected_age_value)
	correctedInitialAge := apparentAge
	if correctedAgeValue > correctedInitialAge {
		correctedInitialAge = correctedAgeValue
	}

	// RFC 9111: resident_time = now - response_time
	residentTime := clock.since(responseTime)

	// RFC 9111: current_age = corrected_initial_age + resident_time
	currentAge := correctedInitialAge + residentTime

	return currentAge, nil
}

// formatAge formats a duration as an Age header value (seconds)
func formatAge(age time.Duration) string {
	seconds := int64(age.Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return strconv.FormatInt(seconds, 10)
}
