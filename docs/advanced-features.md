# Advanced Features

## Transport Configuration

The `Transport` struct provides several configuration options:

```go
transport := httpcache.NewTransport(cache)

// Mark cached responses with X-From-Cache, X-Revalidated, and X-Stale headers
transport.MarkCachedResponses = true  // Default: true

// Skip serving server errors (5xx) from cache, even if fresh
// This forces a new request to the server for error responses
transport.SkipServerErrorsFromCache = true  // Default: false

// Configure as public/shared cache instead of private cache
transport.IsPublicCache = true  // Default: false (private cache)
```

### Private vs Public Cache

By default, httpcache operates as a **private cache** (like a web browser cache). This means:

- ✅ **Can cache** responses with `Cache-Control: private`
- ✅ **Can cache** responses with `Cache-Control: public`
- ✅ **Can cache** responses without explicit caching directives (if otherwise cacheable)
- ✅ Suitable for single-user scenarios (web browsers, API clients)

When `IsPublicCache = true`, httpcache operates as a **shared/public cache** (like a CDN or reverse proxy). This means:

- ❌ **Cannot cache** responses with `Cache-Control: private`
- ✅ **Can cache** responses with `Cache-Control: public`
- ✅ **Can cache** responses without explicit caching directives (if otherwise cacheable)
- ✅ Suitable for multi-user scenarios (CDNs, reverse proxies, shared caches)

**Example: Private Cache (default)**

```go
transport := httpcache.NewMemoryCacheTransport()
// transport.IsPublicCache = false  // Default

client := transport.Client()

// Response: Cache-Control: private, max-age=3600
resp, _ := client.Get("https://api.example.com/user/profile")
// ✅ Response is cached (private caches can cache private responses)

// Second request
resp, _ = client.Get("https://api.example.com/user/profile")
// Returns from cache (X-From-Cache: 1)
```

**Example: Public Cache**

```go
transport := httpcache.NewMemoryCacheTransport()
transport.IsPublicCache = true  // Shared cache mode

client := transport.Client()

// Response: Cache-Control: private, max-age=3600
resp, _ := client.Get("https://api.example.com/user/profile")
// ❌ Response is NOT cached (public caches must not cache private responses)

// Second request
resp, _ = client.Get("https://api.example.com/user/profile")
// Makes a fresh request to the server (not from cache)

// Response: Cache-Control: public, max-age=3600
resp, _ = client.Get("https://api.example.com/public/data")
// ✅ Response is cached (public caches can cache public responses)
```

**When to use IsPublicCache:**

- **false (default)**: Web browsers, mobile apps, API clients, desktop applications
- **true**: CDN nodes, reverse proxies, shared caching layers, multi-tenant services

This implements RFC 9111 Section 5.2.2.6 (Cache-Control: private directive).

### SkipServerErrorsFromCache

**`SkipServerErrorsFromCache`** is useful when you want to:

- Always get fresh error responses from the server
- Prevent hiding ongoing server issues with cached errors
- Ensure monitoring systems detect real-time server problems

Example:

```go
transport := httpcache.NewMemoryCacheTransport()
transport.SkipServerErrorsFromCache = true

client := transport.Client()
// Any 5xx responses in cache will be bypassed
// and a fresh request will be made to the server
```

## Custom Logger

httpcache uses Go's standard `log/slog` package for logging. The logger is used to generate warning messages for errors that were previously silent, helping you identify potential issues in cache operations.

```go
import (
    "log/slog"
    "os"
    
    "github.com/sandrolain/httpcache"
)

// Create a custom logger
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelWarn,
}))

// Set the logger for httpcache
httpcache.SetLogger(logger)

// Now all httpcache operations will use your custom logger
transport := httpcache.NewMemoryCacheTransport()
client := transport.Client()
```

If no logger is set, httpcache uses `slog.Default()`.

For more information on configuring slog loggers, see the [official slog documentation](https://pkg.go.dev/log/slog).

## Stale-If-Error Support

Automatically serve stale cached content when the backend is unavailable:

```go
// Server returns 500, but cached response is served instead
resp, _ := client.Get(url) // Returns cached response, not 500 error
// Response will have X-From-Cache: 1 and X-Stale: 1 headers
```

This implements [RFC 5861](https://tools.ietf.org/html/rfc5861) for better resilience.

## Stale-While-Revalidate Support

Improve perceived performance by serving stale content immediately while updating the cache in the background:

```go
transport := httpcache.NewMemoryCacheTransport()

// Optional: Set timeout for async revalidation requests
transport.AsyncRevalidateTimeout = 30 * time.Second  // Default: 0 (no timeout)

client := transport.Client()

// Server responds with: Cache-Control: max-age=60, stale-while-revalidate=300
// First request: Fetches from server and caches (60s fresh)
// Second request (after 70s): Returns stale cache immediately + revalidates in background
// Third request (after 80s): Returns fresh cache (updated by background revalidation)
```

This implements the `stale-while-revalidate` directive from [RFC 5861](https://tools.ietf.org/html/rfc5861), which:

- **Reduces latency**: Returns cached response immediately without waiting for revalidation
- **Improves UX**: Users get instant responses even when cache is slightly stale
- **Updates cache**: Background goroutine fetches fresh data for subsequent requests

**How it works:**

1. When a response is stale but within the `stale-while-revalidate` window
2. The cached response is returned immediately to the client
3. A background goroutine makes a fresh request to update the cache
4. Subsequent requests get the updated cached response

**Configuration:**

```go
transport.AsyncRevalidateTimeout = 30 * time.Second  // Timeout for background updates
transport.MarkCachedResponses = true                 // See X-Cache-Freshness header
```

**Detecting stale-while-revalidate responses:**

```go
if resp.Header.Get(httpcache.XFreshness) == "stale-while-revalidate" {
    fmt.Println("Serving stale cache, updating in background")
}
```

## Cache Key Headers

Differentiate cache entries based on request header values. This is useful when different header values should result in separate cache entries.

**Common Use Cases:**

- **User-specific caching**: Different cache per user (via Authorization header)
- **Internationalization**: Language-specific responses (via Accept-Language)
- **API versioning**: Version-specific responses (via API-Version header)
- **Multi-tenant apps**: Tenant-specific responses (via X-Tenant-ID header)

**Important:** This is different from the HTTP `Vary` response header mechanism, which is handled separately by httpcache. `CacheKeyHeaders` allows you to specify which **request** headers should be included in the cache key generation.

**Configuration:**

```go
transport := httpcache.NewMemoryCacheTransport()

// Specify headers to include in cache key
transport.CacheKeyHeaders = []string{"Authorization", "Accept-Language"}

client := transport.Client()

// Each unique combination of Authorization + Accept-Language gets its own cache entry
```

**Example Scenario:**

```go
transport := httpcache.NewMemoryCacheTransport()
transport.CacheKeyHeaders = []string{"Authorization"}

client := transport.Client()

// Request 1: Authorization: Bearer token1
req1, _ := http.NewRequest("GET", "https://api.example.com/user/profile", nil)
req1.Header.Set("Authorization", "Bearer token1")
resp1, _ := client.Do(req1)  // Cache miss, fetches from server
io.Copy(io.Discard, resp1.Body)
resp1.Body.Close()

// Request 2: Authorization: Bearer token2 (different token)
req2, _ := http.NewRequest("GET", "https://api.example.com/user/profile", nil)
req2.Header.Set("Authorization", "Bearer token2")
resp2, _ := client.Do(req2)  // Cache miss, fetches from server (different cache entry)
io.Copy(io.Discard, resp2.Body)
resp2.Body.Close()

// Request 3: Authorization: Bearer token1 (same as request 1)
req3, _ := http.NewRequest("GET", "https://api.example.com/user/profile", nil)
req3.Header.Set("Authorization", "Bearer token1")
resp3, _ := client.Do(req3)  // Cache hit! Serves cached response from request 1
io.Copy(io.Discard, resp3.Body)
resp3.Body.Close()

fmt.Println(resp3.Header.Get(httpcache.XFromCache))  // "1"
```

**Cache Key Format:**

Without CacheKeyHeaders:

```
http://api.example.com/data
```

With CacheKeyHeaders:

```
http://api.example.com/data|Accept-Language:en|Authorization:Bearer token1
```

**Important Notes:**

- Header names are case-insensitive (automatically canonicalized)
- Headers are sorted alphabetically for consistent key generation
- Only non-empty header values are included in the key
- Empty `CacheKeyHeaders` slice maintains backward compatibility (headers not included)

**⚠️ Interaction with Server `Vary` Header:**

Even when using `CacheKeyHeaders`, the server's `Vary` header is **still validated**. This means:

1. **Matching headers**: If `CacheKeyHeaders` includes the same headers as server's `Vary`, everything works correctly:

   ```go
   transport.CacheKeyHeaders = []string{"Authorization"}
   // Server responds with: Vary: Authorization
   // ✅ Works perfectly - separate cache entries + validation
   ```

2. **Missing headers**: If server's `Vary` includes headers NOT in `CacheKeyHeaders`, cache will be invalidated:

   ```go
   transport.CacheKeyHeaders = []string{"Authorization"}
   // Server responds with: Vary: Authorization, Accept
   
   // Request 1: Auth: token1, Accept: json → Cached
   // Request 2: Auth: token1, Accept: html → Same cache key, but Vary validation fails
   // ❌ Cache invalidated and overwritten
   ```

**Best Practice**: Always include **all headers mentioned in server's `Vary` response** in your `CacheKeyHeaders` configuration to avoid cache invalidation and overwrites.

## Custom Cache Control with ShouldCache

Override default caching behavior for specific HTTP status codes using the `ShouldCache` hook:

```go
transport := httpcache.NewMemoryCacheTransport()

// Cache 404 Not Found responses
transport.ShouldCache = func(resp *http.Response) bool {
    return resp.StatusCode == http.StatusNotFound
}

client := transport.Client()
// Now 404 responses with appropriate Cache-Control headers will be cached
```

**Default Cacheable Status Codes** (per RFC 7231):

- `200` OK
- `203` Non-Authoritative Information
- `204` No Content
- `206` Partial Content  
- `300` Multiple Choices
- `301` Moved Permanently
- `404` Not Found
- `405` Method Not Allowed
- `410` Gone
- `414` Request-URI Too Long
- `501` Not Implemented

**Use Cases:**

```go
// Cache temporary redirects (302, 307)
transport.ShouldCache = func(resp *http.Response) bool {
    return resp.StatusCode == http.StatusFound || 
           resp.StatusCode == http.StatusTemporaryRedirect
}

// Cache specific error pages for offline support
transport.ShouldCache = func(resp *http.Response) bool {
    if resp.StatusCode == http.StatusNotFound {
        // Only cache 404s from specific domain
        return strings.HasPrefix(resp.Request.URL.Host, "api.example.com")
    }
    return false
}

// Complex caching logic
transport.ShouldCache = func(resp *http.Response) bool {
    switch resp.StatusCode {
    case http.StatusOK:
        return true  // Already cached by default, but explicit
    case http.StatusNotFound:
        // Cache 404s but only for GET requests with specific header
        return resp.Request.Method == "GET" && 
               resp.Request.Header.Get("X-Cache-404") == "true"
    case http.StatusBadRequest:
        // Cache validation errors to reduce server load
        return resp.Header.Get("Content-Type") == "application/json"
    default:
        return false
    }
}
```

**Important Notes:**

- `ShouldCache` is called AFTER checking `Cache-Control` headers
- Responses without appropriate cache headers (e.g., `no-store`, `max-age=0`) are never cached
- The hook only adds additional status codes to cache, it doesn't remove default ones
- Set `ShouldCache = nil` to use default RFC 7231 behavior

## Vary Header Support

⚠️ **Current Limitation**: The `Vary` response header is currently used for **validation only**, not for creating separate cache entries.

See [How It Works](./how-it-works.md) for details on Vary header handling.

## Multi-Tier Caching

For sophisticated caching strategies with multiple storage backends, use the [`multicache`](../wrapper/multicache/README.md) wrapper:

```go
import "github.com/sandrolain/httpcache/wrapper/multicache"

// Create individual cache tiers
memCache := httpcache.NewMemoryCache()          // Fast, volatile
diskCache := diskcache.New("/tmp/cache")        // Medium, persistent
redisCache, _ := redis.New("localhost:6379")    // Distributed, shared

// Combine into multi-tier cache (order matters!)
mc := multicache.New(memCache, diskCache, redisCache)

transport := httpcache.NewTransport(mc)
client := &http.Client{Transport: transport}
```

**Benefits:**

- **Performance**: Hot data in fast tiers, cold data in slow tiers
- **Resilience**: Automatic fallback if faster tiers are empty
- **Automatic promotion**: Popular data migrates to faster tiers
- **Flexibility**: Each tier can have different eviction policies

**Common Patterns:**

- Memory → Disk → Database (performance + persistence)
- Local → Redis → PostgreSQL (local + distributed)
- Edge → Regional → Origin (CDN-like architecture)

See the [MultiCache documentation](../wrapper/multicache/README.md) for complete details and examples.
