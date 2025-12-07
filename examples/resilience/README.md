# Resilience Example

This example demonstrates how to use httpcache with resilience features like retry policies and circuit breakers using the [failsafe-go](https://failsafe-go.dev/) library.

## Features Demonstrated

1. **Retry Policy**: Automatically retry failed requests with exponential backoff
2. **Circuit Breaker**: Prevent cascading failures by opening the circuit after consecutive failures
3. **Combined Approach**: Use both retry and circuit breaker together for maximum resilience

## Running the Example

```bash
cd examples/resilience
go run main.go
```

## Configuration Options

### Retry Policy

```go
retryPolicy := httpcache.RetryPolicyBuilder().
    WithMaxRetries(5).                                    // Maximum retry attempts
    WithBackoff(100*time.Millisecond, 5*time.Second).     // Exponential backoff range
    WithJitter(100 * time.Millisecond).                   // Random jitter to prevent thundering herd
    OnRetry(func(e any) {
        log.Println("Retrying request...")
    }).
    Build()
```

### Circuit Breaker

```go
cb := httpcache.CircuitBreakerBuilder().
    WithFailureThreshold(3).      // Open after N consecutive failures
    WithSuccessThreshold(2).      // Close after N consecutive successes in half-open
    WithDelay(30 * time.Second).  // Wait before entering half-open state
    OnOpen(func(e circuitbreaker.StateChangedEvent) {
        log.Println("Circuit opened!")
    }).
    Build()
```

### Using Resilience

```go
transport := httpcache.NewTransport(
    cache,
    httpcache.WithResilience(&httpcache.ResilienceConfig{
        RetryPolicy:    retryPolicy,    // Optional
        CircuitBreaker: cb,             // Optional
    }),
)
```

## How It Works

### Retry Policy

When a request fails (network error or 5xx status), the retry policy:

1. Waits according to the backoff configuration
2. Retries the request
3. Repeats until max retries is reached or request succeeds

### Circuit Breaker

The circuit breaker has three states:

- **Closed**: Normal operation, requests pass through
- **Open**: Too many failures detected, requests are rejected immediately
- **Half-Open**: Testing if service recovered, limited requests allowed

When failures exceed the threshold, the circuit opens and rejects requests for the configured delay period. After the delay, it enters half-open state to test if the service recovered.

### Combined Usage

When using both retry and circuit breaker:

1. Retry policy tries to recover from transient failures
2. Circuit breaker protects against prolonged outages
3. Provides both immediate recovery and long-term protection

## Default Configurations

### Default Retry Policy

- Max retries: 3
- Backoff: 100ms to 10s (exponential)
- Retries on: network errors and 5xx status codes

### Default Circuit Breaker

- Failure threshold: 5 consecutive failures
- Success threshold: 2 consecutive successes
- Delay: 60 seconds
- Opens on: network errors and 5xx status codes

## Learn More

- [failsafe-go Documentation](https://failsafe-go.dev/)
- [Retry Patterns](https://failsafe-go.dev/retry/)
- [Circuit Breaker Patterns](https://failsafe-go.dev/circuit-breaker/)
