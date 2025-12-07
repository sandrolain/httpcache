package httpcache

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
)

// TestRetryPolicyBuilder tests the convenience retry policy builder.
func TestRetryPolicyBuilder(t *testing.T) {
	policy := RetryPolicyBuilder().Build()

	if policy == nil {
		t.Fatal("expected non-nil policy")
	}

	// Test that it retries on errors
	attempts := 0
	fn := func() (*http.Response, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("test error")
		}
		return &http.Response{StatusCode: 200}, nil
	}

	resp, err := failsafe.With(policy).Get(fn)

	if err != nil {
		t.Fatalf("expected no error after retries, got %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

// TestCircuitBreakerBuilder tests the convenience circuit breaker builder.
func TestCircuitBreakerBuilder(t *testing.T) {
	cb := CircuitBreakerBuilder().
		WithDelay(100 * time.Millisecond).
		Build()

	if cb == nil {
		t.Fatal("expected non-nil circuit breaker")
	}

	// Initially closed
	if !cb.IsClosed() {
		t.Fatal("expected circuit to be closed initially")
	}

	// Record failures to open it
	for i := 0; i < 5; i++ {
		cb.RecordError(errors.New("test error"))
	}

	if !cb.IsOpen() {
		t.Fatal("expected circuit to be open after failures")
	}
}

// TestTransportWithRetry tests retry integration with Transport using failsafe-go.
func TestTransportWithRetry(t *testing.T) {
	attempts := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success")) //nolint:errcheck
	}))
	defer server.Close()

	cache := newMockCache()
	retryPolicy := RetryPolicyBuilder().
		WithMaxRetries(3).
		WithBackoff(10*time.Millisecond, 100*time.Millisecond).
		Build()

	transport := NewTransport(cache, WithResilience(&ResilienceConfig{
		RetryPolicy: retryPolicy,
	}))

	client := transport.Client()
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("expected 3 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

// TestTransportWithCircuitBreaker tests circuit breaker integration with Transport.
func TestTransportWithCircuitBreaker(t *testing.T) {
	failures := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&failures, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cache := newMockCache()
	cb := CircuitBreakerBuilder().
		WithFailureThreshold(3).
		WithDelay(200 * time.Millisecond).
		Build()

	transport := NewTransport(cache, WithResilience(&ResilienceConfig{
		CircuitBreaker: cb,
	}))

	client := transport.Client()

	// Make requests until circuit opens
	for i := 0; i < 5; i++ {
		resp, err := client.Get(server.URL)
		if err != nil {
			// Circuit breaker might reject requests
			if errors.Is(err, circuitbreaker.ErrOpen) {
				t.Logf("Circuit opened at attempt %d", i+1)
				break
			}
		}
		if resp != nil {
			resp.Body.Close() //nolint:errcheck
		}
	}

	// Circuit should be open now
	if !cb.IsOpen() {
		t.Fatal("expected circuit to be open after failures")
	}

	// Verify circuit rejects requests
	failureCount := atomic.LoadInt32(&failures)
	_, err := client.Get(server.URL)
	if err == nil {
		t.Fatal("expected error from open circuit")
	}
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("expected circuit open error, got %v", err)
	}

	// No new request should have been made
	if atomic.LoadInt32(&failures) != failureCount {
		t.Fatal("circuit breaker did not prevent request")
	}
}

// TestTransportWithRetryAndCircuitBreaker tests both retry and circuit breaker together.
func TestTransportWithRetryAndCircuitBreaker(t *testing.T) {
	attempts := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cache := newMockCache()

	// Configure retry and circuit breaker
	retryPolicy := RetryPolicyBuilder().
		WithMaxRetries(1).
		WithBackoff(10*time.Millisecond, 50*time.Millisecond).
		Build()

	cb := CircuitBreakerBuilder().
		WithFailureThreshold(3). // Opens after 3 failures
		WithDelay(200 * time.Millisecond).
		Build()

	transport := NewTransport(cache, WithResilience(&ResilienceConfig{
		RetryPolicy:    retryPolicy,
		CircuitBreaker: cb,
	}))

	client := transport.Client()

	// Make 3 requests that will fail, each exhausting retries
	// This should accumulate 3 failures in the circuit breaker
	for i := 0; i < 3; i++ {
		resp, err := client.Get(server.URL)
		// Either we get an error (retries exceeded) or a 503 response
		if err == nil {
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("request %d: expected 503, got %d", i+1, resp.StatusCode)
			}
			resp.Body.Close() //nolint:errcheck
		}
		// Both cases (error or 503) count as failures for circuit breaker
	}

	// Circuit should be open now
	if !cb.IsOpen() {
		t.Fatal("expected circuit to be open after 3 failed requests")
	}

	// Fourth request: circuit should reject it immediately
	_, err := client.Get(server.URL)
	if err == nil {
		t.Fatal("expected error from open circuit")
	}
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("expected circuit open error, got %v", err)
	}
}

// TestCircuitBreakerStateTransitions tests circuit breaker state transitions.
func TestCircuitBreakerStateTransitions(t *testing.T) {
	stateChanges := []string{}
	mu := sync.Mutex{}

	cb := CircuitBreakerBuilder().
		WithFailureThreshold(2).
		WithSuccessThreshold(1).
		WithDelay(100 * time.Millisecond).
		OnOpen(func(event circuitbreaker.StateChangedEvent) {
			mu.Lock()
			defer mu.Unlock()
			stateChanges = append(stateChanges, "open")
		}).
		OnHalfOpen(func(event circuitbreaker.StateChangedEvent) {
			mu.Lock()
			defer mu.Unlock()
			stateChanges = append(stateChanges, "half-open")
		}).
		OnClose(func(event circuitbreaker.StateChangedEvent) {
			mu.Lock()
			defer mu.Unlock()
			stateChanges = append(stateChanges, "closed")
		}).
		Build()

	// Initially closed
	if !cb.IsClosed() {
		t.Fatal("expected circuit to be closed initially")
	}

	// Open the circuit with failures using failsafe executor
	executor := failsafe.With[*http.Response](cb)

	// Record 2 failures to open the circuit
	_, _ = executor.Get(func() (*http.Response, error) {
		return nil, errors.New("error 1")
	})
	_, _ = executor.Get(func() (*http.Response, error) {
		return nil, errors.New("error 2")
	})

	if !cb.IsOpen() {
		t.Fatal("expected circuit to be open")
	}
	mu.Lock()
	if len(stateChanges) != 1 || stateChanges[0] != "open" {
		mu.Unlock()
		t.Fatalf("expected 'open' state change, got %v", stateChanges)
	}
	mu.Unlock()

	// Wait for circuit to enter half-open state
	time.Sleep(150 * time.Millisecond)

	// Execute a successful request to transition from half-open to closed
	_, _ = executor.Get(func() (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	})

	if !cb.IsClosed() {
		t.Fatal("expected circuit to be closed after success in half-open")
	}

	mu.Lock()
	if len(stateChanges) < 3 {
		mu.Unlock()
		t.Fatalf("expected 3 state changes (open, half-open, closed), got %v", stateChanges)
	}
	mu.Unlock()
} // TestWithResilienceOption tests the WithResilience option.
func TestWithResilienceOption(t *testing.T) {
	cache := newMockCache()

	t.Run("valid config with retry only", func(t *testing.T) {
		retryPolicy := RetryPolicyBuilder().Build()
		transport := NewTransport(cache, WithResilience(&ResilienceConfig{
			RetryPolicy: retryPolicy,
		}))

		if transport.resilience == nil {
			t.Fatal("expected resilience config to be set")
		}
		if transport.resilience.RetryPolicy == nil {
			t.Fatal("expected retry policy to be set")
		}
	})

	t.Run("valid config with circuit breaker only", func(t *testing.T) {
		cb := CircuitBreakerBuilder().Build()
		transport := NewTransport(cache, WithResilience(&ResilienceConfig{
			CircuitBreaker: cb,
		}))

		if transport.resilience == nil {
			t.Fatal("expected resilience config to be set")
		}
		if transport.resilience.CircuitBreaker == nil {
			t.Fatal("expected circuit breaker to be set")
		}
	})

	t.Run("valid config with both", func(t *testing.T) {
		retryPolicy := RetryPolicyBuilder().Build()
		cb := CircuitBreakerBuilder().Build()

		transport := NewTransport(cache, WithResilience(&ResilienceConfig{
			RetryPolicy:    retryPolicy,
			CircuitBreaker: cb,
		}))

		if transport.resilience == nil {
			t.Fatal("expected resilience config to be set")
		}
		if transport.resilience.RetryPolicy == nil {
			t.Fatal("expected retry policy to be set")
		}
		if transport.resilience.CircuitBreaker == nil {
			t.Fatal("expected circuit breaker to be set")
		}
	})

	t.Run("nil config returns error", func(t *testing.T) {
		err := WithResilience(nil)(NewTransport(cache))
		if err == nil {
			t.Fatal("expected error for nil config")
		}
	})
}

// TestExecuteWithResilience tests the executeWithResilience method.
func TestExecuteWithResilience(t *testing.T) {
	t.Run("no resilience configured", func(t *testing.T) {
		transport := NewTransport(newMockCache())

		executed := false
		resp, err := transport.executeWithResilience(func() (*http.Response, error) {
			executed = true
			return &http.Response{StatusCode: 200}, nil
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !executed {
			t.Fatal("expected function to be executed")
		}
		if resp.StatusCode != 200 {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("empty resilience config", func(t *testing.T) {
		transport := NewTransport(newMockCache(), WithResilience(&ResilienceConfig{}))

		executed := false
		resp, err := transport.executeWithResilience(func() (*http.Response, error) {
			executed = true
			return &http.Response{StatusCode: 200}, nil
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !executed {
			t.Fatal("expected function to be executed")
		}
		if resp.StatusCode != 200 {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}
	})
}

// TestRetryOnNetworkErrors tests that retry works for network errors.
func TestRetryOnNetworkErrors(t *testing.T) {
	attempts := 0

	cache := newMockCache()
	retryPolicy := RetryPolicyBuilder().
		WithMaxRetries(2).
		WithBackoff(10*time.Millisecond, 50*time.Millisecond).
		Build()

	transport := NewTransport(cache, WithResilience(&ResilienceConfig{
		RetryPolicy: retryPolicy,
	}))

	// Create a server that closes immediately to simulate network error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			// Force connection close
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				conn.Close() //nolint:errcheck
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := transport.Client()
	resp, err := client.Get(server.URL)

	// Might still get an error due to connection issues
	if err == nil {
		defer resp.Body.Close() //nolint:errcheck
		if resp.StatusCode != 200 {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}
	}

	// Should have made multiple attempts
	if attempts < 2 {
		t.Fatalf("expected at least 2 attempts due to retries, got %d", attempts)
	}
}
