package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/diskcache"
)

func main() {
	// Create a disk cache
	cache := diskcache.New("/tmp/httpcache-resilience")

	// Example 1: Using retry policy only
	fmt.Println("=== Example 1: Retry Policy ===")
	retryExample(cache)

	// Example 2: Using circuit breaker only
	fmt.Println("\n=== Example 2: Circuit Breaker ===")
	circuitBreakerExample(cache)

	// Example 3: Using both retry and circuit breaker
	fmt.Println("\n=== Example 3: Combined Retry and Circuit Breaker ===")
	combinedExample(cache)
}

func retryExample(cache httpcache.Cache) {
	// Configure retry policy with custom settings
	retryPolicy := httpcache.RetryPolicyBuilder().
		WithMaxRetries(5).                                // Retry up to 5 times
		WithBackoff(100*time.Millisecond, 5*time.Second). // Exponential backoff
		WithJitter(100 * time.Millisecond).               // Add jitter to prevent thundering herd
		OnRetry(func(e failsafe.ExecutionEvent[*http.Response]) {
			fmt.Printf("  Retrying request...\n")
		}).
		Build()

	// Create transport with retry policy
	transport := httpcache.NewTransport(
		cache,
		httpcache.WithResilience(&httpcache.ResilienceConfig{
			RetryPolicy: retryPolicy,
		}),
	)

	client := transport.Client()

	// Make a request that might fail temporarily
	resp, err := client.Get("https://httpbin.org/status/503")
	if err != nil {
		log.Printf("Request failed after retries: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Response status: %s\n", resp.Status)
}

func circuitBreakerExample(cache httpcache.Cache) {
	// Configure circuit breaker with custom settings
	cb := httpcache.CircuitBreakerBuilder().
		WithFailureThreshold(3).     // Open after 3 consecutive failures
		WithSuccessThreshold(2).     // Close after 2 consecutive successes in half-open
		WithDelay(30 * time.Second). // Wait 30s before entering half-open
		OnOpen(func(e circuitbreaker.StateChangedEvent) {
			fmt.Println("  ⚠️  Circuit breaker opened!")
		}).
		OnHalfOpen(func(e circuitbreaker.StateChangedEvent) {
			fmt.Println("  🔄 Circuit breaker half-open, testing...")
		}).
		OnClose(func(e circuitbreaker.StateChangedEvent) {
			fmt.Println("  ✅ Circuit breaker closed, normal operation")
		}).
		Build()

	// Create transport with circuit breaker
	transport := httpcache.NewTransport(
		cache,
		httpcache.WithResilience(&httpcache.ResilienceConfig{
			CircuitBreaker: cb,
		}),
	)

	client := transport.Client()

	// Make multiple requests to demonstrate circuit breaker
	for i := 1; i <= 5; i++ {
		fmt.Printf("Request %d: ", i)
		resp, err := client.Get("https://httpbin.org/delay/1")
		if err != nil {
			log.Printf("Failed: %v\n", err)
			continue
		}
		resp.Body.Close()
		fmt.Printf("Success (%s)\n", resp.Status)
		time.Sleep(500 * time.Millisecond)
	}
}

func combinedExample(cache httpcache.Cache) {
	// Configure both retry and circuit breaker
	retryPolicy := httpcache.RetryPolicyBuilder().
		WithMaxRetries(2).
		WithBackoff(100*time.Millisecond, 2*time.Second).
		OnRetry(func(e failsafe.ExecutionEvent[*http.Response]) {
			fmt.Printf("  🔄 Retrying...\n")
		}).
		Build()

	cb := httpcache.CircuitBreakerBuilder().
		WithFailureThreshold(5).
		WithSuccessThreshold(2).
		WithDelay(10 * time.Second).
		OnOpen(func(e circuitbreaker.StateChangedEvent) {
			fmt.Println("  ⚠️  Circuit opened after too many failures!")
		}).
		Build()

	// Create transport with both policies
	// Note: Policies are applied in order: retry first, then circuit breaker
	transport := httpcache.NewTransport(
		cache,
		httpcache.WithResilience(&httpcache.ResilienceConfig{
			RetryPolicy:    retryPolicy,
			CircuitBreaker: cb,
		}),
	)

	client := transport.Client()

	// Make a successful request
	fmt.Println("Making a successful request:")
	resp, err := client.Get("https://httpbin.org/get")
	if err != nil {
		log.Printf("Request failed: %v\n", err)
		return
	}
	defer resp.Body.Close()
	fmt.Printf("✅ Success: %s\n", resp.Status)

	// The combination of retry and circuit breaker provides:
	// - Automatic retries for transient failures
	// - Protection against cascading failures via circuit breaking
	// - Graceful degradation when services are unavailable
}
