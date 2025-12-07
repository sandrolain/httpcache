// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"net/http"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

// ResilienceConfig holds the configuration for resilience policies.
// Resilience features are disabled by default and must be explicitly enabled.
type ResilienceConfig struct {
	// RetryPolicy configures retry behavior using failsafe-go.
	// If nil, retry is disabled.
	// Example:
	//   retrypolicy.Builder[*http.Response]().
	//     HandleIf(func(r *http.Response, err error) bool {
	//       return err != nil || r.StatusCode >= 500
	//     }).
	//     WithMaxRetries(3).
	//     WithBackoff(100*time.Millisecond, 10*time.Second).
	//     Build()
	RetryPolicy retrypolicy.RetryPolicy[*http.Response]

	// CircuitBreaker configures circuit breaker behavior using failsafe-go.
	// If nil, circuit breaker is disabled.
	// Example:
	//   circuitbreaker.Builder[*http.Response]().
	//     HandleIf(func(r *http.Response, err error) bool {
	//       return err != nil || r.StatusCode >= 500
	//     }).
	//     WithFailureThreshold(5).
	//     WithSuccessThreshold(2).
	//     WithDelay(60*time.Second).
	//     Build()
	CircuitBreaker circuitbreaker.CircuitBreaker[*http.Response]
}

// RetryPolicyBuilder creates a pre-configured retry policy builder for HTTP requests.
// This is a convenience function that sets sensible defaults for HTTP retries.
// You can further customize the builder before calling Build().
//
// Default configuration:
//   - Retries on: network errors and 5xx status codes
//   - Max retries: 3
//   - Backoff: exponential from 100ms to 10s
//
// Example:
//
//	policy := httpcache.RetryPolicyBuilder().
//	    WithMaxRetries(5).
//	    Build()
func RetryPolicyBuilder() retrypolicy.Builder[*http.Response] {
	return retrypolicy.NewBuilder[*http.Response]().
		HandleIf(func(r *http.Response, err error) bool {
			// Retry on errors or 5xx status codes
			if err != nil {
				return true
			}
			if r != nil && r.StatusCode >= 500 {
				return true
			}
			return false
		}).
		WithMaxRetries(3).
		WithBackoff(100*time.Millisecond, 10*time.Second)
}

// CircuitBreakerBuilder creates a pre-configured circuit breaker builder for HTTP requests.
// This is a convenience function that sets sensible defaults for HTTP circuit breaking.
// You can further customize the builder before calling Build().
//
// Default configuration:
//   - Opens on: network errors and 5xx status codes
//   - Failure threshold: 5 consecutive failures
//   - Success threshold: 2 consecutive successes (in half-open state)
//   - Delay: 60 seconds before entering half-open state
//
// Example:
//
//	cb := httpcache.CircuitBreakerBuilder().
//	    WithFailureThreshold(10).
//	    OnOpen(func(e circuitbreaker.StateChangedEvent) {
//	        log.Println("Circuit breaker opened!")
//	    }).
//	    Build()
func CircuitBreakerBuilder() circuitbreaker.Builder[*http.Response] {
	return circuitbreaker.NewBuilder[*http.Response]().
		HandleIf(func(r *http.Response, err error) bool {
			// Circuit opens on errors or 5xx status codes
			if err != nil {
				return true
			}
			if r != nil && r.StatusCode >= 500 {
				return true
			}
			return false
		}).
		WithFailureThreshold(5).
		WithSuccessThreshold(2).
		WithDelay(60 * time.Second)
}

// executeWithResilience wraps the HTTP request execution with resilience policies.
// This is called internally by RoundTrip when resilience is configured.
func (t *Transport) executeWithResilience(fn func() (*http.Response, error)) (*http.Response, error) {
	if t.resilience == nil {
		// No resilience configured, execute directly
		return fn()
	}

	// Build the failsafe executor with configured policies
	var policies []failsafe.Policy[*http.Response]

	// Add retry policy first (innermost policy)
	if t.resilience.RetryPolicy != nil {
		policies = append(policies, t.resilience.RetryPolicy)
	}

	// Add circuit breaker (outermost policy)
	if t.resilience.CircuitBreaker != nil {
		policies = append(policies, t.resilience.CircuitBreaker)
	}

	if len(policies) == 0 {
		// No policies configured, execute directly
		return fn()
	}

	// Execute with failsafe
	return failsafe.With(policies...).Get(fn)
}
