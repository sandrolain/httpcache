# Resilience Features

httpcache provides built-in resilience features to make your HTTP clients more reliable and fault-tolerant. These features are powered by [failsafe-go](https://failsafe-go.dev/), a production-grade resilience library.

## Overview

Resilience features help your application handle:

- **Transient failures**: Temporary network issues, service hiccups
- **Service outages**: When a service is unavailable for extended periods
- **Cascading failures**: Preventing one service failure from affecting others
- **Rate limiting**: Backpressure when services are overloaded

## Features

### Retry Policy

Automatically retries failed HTTP requests with exponential backoff and jitter.

**When to use:**

- Handling temporary network failures
- Recovering from transient 5xx errors
- Dealing with rate-limited APIs

**Example:**

```go
retryPolicy := httpcache.RetryPolicyBuilder().
    WithMaxRetries(5).
    WithBackoff(100*time.Millisecond, 5*time.Second).
    WithJitter(100 * time.Millisecond).
    Build()

transport := httpcache.NewTransport(
    cache,
    httpcache.WithResilience(&httpcache.ResilienceConfig{
        RetryPolicy: retryPolicy,
    }),
)
```

### Circuit Breaker

Prevents cascading failures by "opening the circuit" after detecting too many consecutive failures.

**When to use:**

- Protecting against service outages
- Preventing resource exhaustion
- Fast-failing when services are unavailable
- Allowing services time to recover

**Example:**

```go
cb := httpcache.CircuitBreakerBuilder().
    WithFailureThreshold(5).
    WithSuccessThreshold(2).
    WithDelay(30 * time.Second).
    OnOpen(func(e circuitbreaker.StateChangedEvent) {
        log.Println("Circuit breaker opened!")
    }).
    Build()

transport := httpcache.NewTransport(
    cache,
    httpcache.WithResilience(&httpcache.ResilienceConfig{
        CircuitBreaker: cb,
    }),
)
```

### Combined Resilience

Use both retry and circuit breaker together for comprehensive fault tolerance.

**Example:**

```go
transport := httpcache.NewTransport(
    cache,
    httpcache.WithResilience(&httpcache.ResilienceConfig{
        RetryPolicy:    httpcache.RetryPolicyBuilder().Build(),
        CircuitBreaker: httpcache.CircuitBreakerBuilder().Build(),
    }),
)
```

## Configuration

### Retry Policy Configuration

The `RetryPolicyBuilder()` provides sensible defaults and allows customization:

```go
retryPolicy := httpcache.RetryPolicyBuilder().
    // Maximum number of retry attempts (default: 3)
    WithMaxRetries(5).
    
    // Exponential backoff range (default: 100ms to 10s)
    WithBackoff(100*time.Millisecond, 10*time.Second).
    
    // Add random jitter to prevent thundering herd
    WithJitter(200 * time.Millisecond).
    
    // Custom retry condition (default: errors and 5xx)
    HandleIf(func(r *http.Response, err error) bool {
        return err != nil || (r != nil && r.StatusCode >= 500)
    }).
    
    // Callback on retry attempts
    OnRetry(func(e any) {
        log.Println("Retrying request")
    }).
    
    Build()
```

### Circuit Breaker Configuration

The `CircuitBreakerBuilder()` provides sensible defaults and allows customization:

```go
cb := httpcache.CircuitBreakerBuilder().
    // Open circuit after N consecutive failures (default: 5)
    WithFailureThreshold(10).
    
    // Close circuit after N consecutive successes in half-open (default: 2)
    WithSuccessThreshold(3).
    
    // Wait time before entering half-open state (default: 60s)
    WithDelay(30 * time.Second).
    
    // Custom failure condition (default: errors and 5xx)
    HandleIf(func(r *http.Response, err error) bool {
        return err != nil || (r != nil && r.StatusCode >= 500)
    }).
    
    // State change callbacks
    OnOpen(func(e circuitbreaker.StateChangedEvent) {
        log.Println("Circuit opened!")
    }).
    OnHalfOpen(func(e circuitbreaker.StateChangedEvent) {
        log.Println("Circuit half-open, testing...")
    }).
    OnClose(func(e circuitbreaker.StateChangedEvent) {
        log.Println("Circuit closed, normal operation")
    }).
    
    Build()
```

## Default Behavior

### Retry Policy Defaults

- **Max retries**: 3 attempts
- **Backoff**: Exponential from 100ms to 10s
- **Retry on**: Network errors and HTTP 5xx status codes

### Circuit Breaker Defaults

- **Failure threshold**: 5 consecutive failures
- **Success threshold**: 2 consecutive successes (in half-open state)
- **Delay**: 60 seconds before entering half-open
- **Opens on**: Network errors and HTTP 5xx status codes

## Circuit Breaker States

The circuit breaker can be in one of three states:

### Closed (Normal Operation)

- All requests pass through
- Failures are counted
- Opens when failure threshold is reached

### Open (Failing Fast)

- Requests are rejected immediately with `circuitbreaker.ErrOpen`
- No requests are sent to the failing service
- After the delay period, transitions to half-open

### Half-Open (Testing Recovery)

- Limited requests are allowed through to test if service recovered
- If successful requests reach success threshold, closes the circuit
- If any request fails, opens the circuit again

## Monitoring

You can monitor resilience behavior using the state change callbacks:

```go
cb := httpcache.CircuitBreakerBuilder().
    OnOpen(func(e circuitbreaker.StateChangedEvent) {
        metrics.RecordCircuitOpen()
        log.Warn("Circuit breaker opened")
    }).
    OnClose(func(e circuitbreaker.StateChangedEvent) {
        metrics.RecordCircuitClosed()
        log.Info("Circuit breaker closed")
    }).
    Build()

retryPolicy := httpcache.RetryPolicyBuilder().
    OnRetry(func(e any) {
        metrics.RecordRetry()
        log.Debug("Retrying request")
    }).
    Build()
```

## Best Practices

### 1. Choose Appropriate Thresholds

- **Low traffic services**: Lower failure thresholds (3-5 failures)
- **High traffic services**: Higher failure thresholds (10-20 failures)
- **Critical services**: Aggressive retries with longer delays

### 2. Set Reasonable Delays

- **Fast recovery**: Shorter delays (10-30s) for services that recover quickly
- **Slow recovery**: Longer delays (60s+) for services that need time to stabilize
- **Exponential backoff**: Use exponential backoff for retries to avoid overwhelming recovering services

### 3. Monitor and Adjust

- Track circuit breaker state changes
- Monitor retry rates
- Adjust thresholds based on observed behavior
- Alert on persistent circuit opens

### 4. Combine with Caching

The real power comes from combining resilience with caching:

```go
transport := httpcache.NewTransport(
    cache,
    httpcache.WithResilience(&httpcache.ResilienceConfig{
        RetryPolicy:    httpcache.RetryPolicyBuilder().Build(),
        CircuitBreaker: httpcache.CircuitBreakerBuilder().Build(),
    }),
    httpcache.WithLogger(logger),
)
```

When the circuit is open, cached responses can still be served, providing graceful degradation.

### 5. Use Jitter

Always add jitter to retry backoff to prevent thundering herd problems:

```go
retryPolicy := httpcache.RetryPolicyBuilder().
    WithBackoff(100*time.Millisecond, 5*time.Second).
    WithJitter(100 * time.Millisecond).
    Build()
```

## Error Handling

### Retry Exhausted

When retries are exhausted, you get an error containing the last result:

```go
resp, err := client.Get(url)
if err != nil {
    // Could be "retries exceeded" error
    log.Printf("Request failed: %v", err)
}
```

### Circuit Open

When the circuit is open, requests fail immediately:

```go
resp, err := client.Get(url)
if errors.Is(err, circuitbreaker.ErrOpen) {
    // Circuit is open, service is unavailable
    // Consider serving stale cached content
}
```

## Examples

See the [resilience example](../examples/resilience) for complete working examples of:

- Retry policy only
- Circuit breaker only
- Combined retry and circuit breaker

## Performance Considerations

### Memory Usage

- Circuit breakers maintain state for each instance
- Retry policies add minimal overhead
- Use shared circuit breakers across multiple transports when appropriate

### Latency

- Retries increase latency for failed requests
- Circuit breakers reduce latency by failing fast
- Choose backoff strategies that balance recovery time and latency

### Concurrency

All resilience features are thread-safe and can be used concurrently without additional synchronization.

## Further Reading

- [failsafe-go Documentation](https://failsafe-go.dev/)
- [Retry Pattern](https://failsafe-go.dev/retry/)
- [Circuit Breaker Pattern](https://failsafe-go.dev/circuit-breaker/)
- [Release It! - Michael Nygard](https://pragprog.com/titles/mnee2/release-it-second-edition/)
