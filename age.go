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

	return parseHTTPDate(dateHeader)
}

// parseHTTPDate accepts the three HTTP-date forms required of recipients by
// RFC 9110 §5.6.7 while rejecting obsolete zone abbreviations.
func parseHTTPDate(value string) (time.Time, error) {
	if date, err := time.Parse(http.TimeFormat, value); err == nil {
		return date, nil
	}
	if strings.HasSuffix(value, " GMT") {
		if date, err := time.Parse(time.RFC850, value); err == nil {
			// RFC 9110 §5.6.7: an RFC 850 year that would be more than
			// 50 years in the future is interpreted as the most recent past year
			// with the same final two digits.
			if date.Year() < time.Now().Year()-50 {
				candidate := date.AddDate(100, 0, 0)
				if candidate.Year() <= time.Now().Year()+50 {
					date = candidate
				}
			}
			return date, nil
		}
	}
	return time.Parse(time.ANSIC, value)
}

// maxDeltaSeconds is the greatest representable delta-seconds value.
// RFC 9111 Section 1.2.2: a value that overflows MUST be treated as
// 2147483648 (2^31), never as a negative value in later calculations.
const maxDeltaSeconds = int64(2147483648)

// parseAgeHeader parses the Age header according to RFC 9111 Section 5.1.
// Returns the age duration and a boolean indicating if the header is valid.
//
// RFC 9111 requirements:
//   - If multiple Age headers (or a list-based value) exist, use the first
//     member and discard others (Section 5.1 SHOULD)
//   - If the value is invalid (negative, non-numeric), ignore it completely
//   - A value that overflows is treated as 2147483648 (Section 1.2.2)
//   - The value must be a non-negative integer (1*DIGIT) representing seconds
func parseAgeHeader(headers http.Header, log *slog.Logger) (age time.Duration, valid bool) {
	ageValues := headers.Values(headerAge)

	if len(ageValues) == 0 {
		return 0, false
	}

	// RFC 9111 Section 5.1: although Age is a singleton field, a cache
	// encountering a list-based value SHOULD use the first member and
	// discard subsequent ones.
	first := ageValues[0]
	if member, _, found := strings.Cut(first, ","); found {
		log.Warn("list-based Age header value, using first member",
			"first", strings.TrimSpace(member),
			"all", first)
		first = member
	}

	if len(ageValues) > 1 {
		log.Warn("multiple Age headers detected, using first value",
			"count", len(ageValues),
			"first", strings.TrimSpace(first),
			"all", ageValues)
	}

	ageStr := strings.TrimSpace(first)

	// delta-seconds = 1*DIGIT: reject sign prefixes and empty values
	if ageStr == "" || ageStr[0] == '+' || ageStr[0] == '-' {
		log.Warn("invalid Age header value, ignoring", "value", ageStr)
		return 0, false
	}

	ageInt, err := strconv.ParseInt(ageStr, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			// RFC 9111 Section 1.2.2: overflow must be treated as 2147483648
			log.Warn("Age header value overflow, clamping to 2147483648", "value", ageStr)
			return time.Duration(maxDeltaSeconds) * time.Second, true
		}
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

	if ageInt > maxDeltaSeconds {
		// RFC 9111 Section 1.2.2: clamp instead of wrapping
		log.Warn("Age header value overflow, clamping to 2147483648", "value", ageInt)
		ageInt = maxDeltaSeconds
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
func calculateAge(respHeaders http.Header, clock timer, log *slog.Logger) (age time.Duration, err error) {
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
