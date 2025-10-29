# httpcache

[![CI](https://github.com/sandrolain/httpcache/workflows/CI/badge.svg)](https://github.com/sandrolain/httpcache/actions/workflows/ci.yml)
[![Security](https://github.com/sandrolain/httpcache/workflows/Security/badge.svg)](https://github.com/sandrolain/httpcache/actions/workflows/security.yml)
[![Coverage](https://img.shields.io/badge/coverage-95%25-brightgreen.svg)](https://github.com/sandrolain/httpcache)
[![GoDoc](https://godoc.org/github.com/sandrolain/httpcache?status.svg)](https://godoc.org/github.com/sandrolain/httpcache)
[![Go Report Card](https://goreportcard.com/badge/github.com/sandrolain/httpcache)](https://goreportcard.com/report/github.com/sandrolain/httpcache)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE.txt)

**Package httpcache** provides an `http.RoundTripper` implementation that works as a mostly [RFC 7234](https://tools.ietf.org/html/rfc7234) compliant cache for HTTP responses.

> **Note**: This is a maintained fork of [gregjones/httpcache](https://github.com/gregjones/httpcache), which is no longer actively maintained. This fork aims to modernize the codebase while maintaining backward compatibility, fix bugs, and add new features.

## Features

- ✅ **RFC 7234 Compliant** (~95% compliance) - Implements HTTP caching standards
  - ✅ Age header calculation (Section 4.2.3)
  - ✅ Warning headers for stale responses (Section 5.5)
  - ✅ must-revalidate directive enforcement (Section 5.2.2.1)
  - ✅ Pragma: no-cache support (Section 5.4)
  - ✅ Cache invalidation on unsafe methods (Section 4.4)
- ✅ **Multiple Backends** - Memory, Disk, Redis, LevelDB, Memcache
- ✅ **Thread-Safe** - Safe for concurrent use
- ✅ **Zero Dependencies** - Core package uses only Go standard library
- ✅ **Easy Integration** - Drop-in replacement for `http.Client`
- ✅ **ETag & Validation** - Automatic cache revalidation
- ✅ **Stale-If-Error** - Resilient caching with RFC 5861 support
- ✅ **Stale-While-Revalidate** - Async cache updates for better performance
- ✅ **Private Cache** - Suitable for web browsers and API clients

## Quick Start

```go
package main

import (
    "fmt"
    "io"
    "net/http"
    
    "github.com/sandrolain/httpcache"
)

func main() {
    // Create a cached HTTP client
    transport := httpcache.NewMemoryCacheTransport()
    client := transport.Client()
    
    // Make requests - second request will be cached!
    resp, _ := client.Get("https://example.com")
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    
    // Check if response came from cache
    if resp.Header.Get(httpcache.XFromCache) == "1" {
        fmt.Println("Response was cached!")
    }
}
```

## Installation

```bash
go get github.com/sandrolain/httpcache
```

## Cache Backends

httpcache supports multiple storage backends. Choose the one that fits your use case:

### Built-in Backends

| Backend | Speed | Persistence | Distributed | Use Case |
|---------|-------|-------------|-------------|----------|
| **Memory** | ⚡⚡⚡ Fastest | ❌ No | ❌ No | Development, testing, single-instance apps |
| **[Disk](./diskcache)** | ⚡ Slow | ✅ Yes | ❌ No | Desktop apps, CLI tools |
| **[LevelDB](./leveldbcache)** | ⚡⚡ Fast | ✅ Yes | ❌ No | High-performance local cache |
| **[Redis](./redis)** | ⚡⚡ Fast | ✅ Configurable | ✅ Yes | Microservices, distributed systems |
| **[PostgreSQL](./postgresql)** | ⚡⚡ Fast | ✅ Yes | ✅ Yes | Existing PostgreSQL infrastructure, SQL-based systems |
| **[Memcache](./memcache)** | ⚡⚡ Fast | ❌ No | ✅ Yes | Distributed systems, App Engine |

### Third-Party Backends

- [`sourcegraph.com/sourcegraph/s3cache`](https://sourcegraph.com/github.com/sourcegraph/s3cache) - Amazon S3 storage
- [`github.com/die-net/lrucache`](https://github.com/die-net/lrucache) - In-memory with LRU eviction
- [`github.com/die-net/lrucache/twotier`](https://github.com/die-net/lrucache/tree/master/twotier) - Multi-tier caching (e.g., memory + disk)
- [`github.com/birkelund/boltdbcache`](https://github.com/birkelund/boltdbcache) - BoltDB implementation

### Related Projects

- [`github.com/moul/hcfilters`](https://github.com/moul/hcfilters) - HTTP cache middleware and filters for advanced cache control

## Usage Examples

### Memory Cache (Default)

```go
transport := httpcache.NewMemoryCacheTransport()
client := transport.Client()
```

**Best for**: Testing, development, single-instance applications

### Disk Cache

```go
import "github.com/sandrolain/httpcache/diskcache"

cache := diskcache.New("/tmp/my-cache")
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: Desktop applications, CLI tools that run repeatedly

> ⚠️ **Breaking Change**: The disk cache hashing algorithm has been changed from MD5 to SHA-256 for security reasons. Existing caches created with the original fork (gregjones/httpcache) are **not compatible** and will need to be regenerated.

### Redis Cache

```go
import (
    "github.com/gomodule/redigo/redis"
    rediscache "github.com/sandrolain/httpcache/redis"
)

conn, _ := redis.Dial("tcp", "localhost:6379")
cache := rediscache.NewWithClient(conn)
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: Microservices, distributed systems, high availability

### LevelDB Cache

```go
import "github.com/sandrolain/httpcache/leveldbcache"

cache, _ := leveldbcache.New("/path/to/cache")
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: High-performance local caching with persistence

### PostgreSQL Cache

```go
import "github.com/sandrolain/httpcache/postgresql"

ctx := context.Background()
cache, _ := postgresql.New(ctx, "postgres://user:pass@localhost/dbname", nil)
transport := httpcache.NewTransport(cache)
client := &http.Client{Transport: transport}
```

**Best for**: Applications with existing PostgreSQL infrastructure, SQL-based systems

### Custom Transport Configuration

```go
// Use a custom underlying transport
transport := httpcache.NewTransport(cache)
transport.Transport = &http.Transport{
    MaxIdleConns:        100,
    IdleConnTimeout:     90 * time.Second,
    DisableCompression:  false,
}
transport.MarkCachedResponses = true // Add X-From-Cache header

client := &http.Client{
    Transport: transport,
    Timeout:   30 * time.Second,
}
```

## Practical Examples

See the [`examples/`](./examples) directory for complete, runnable examples:

- **[Basic](./examples/basic/)** - Simple in-memory caching
- **[Disk Cache](./examples/diskcache/)** - Persistent filesystem cache
- **[Redis](./examples/redis/)** - Distributed caching with Redis
- **[LevelDB](./examples/leveldb/)** - High-performance persistent cache
- **[PostgreSQL](./examples/postgresql/)** - SQL-based persistent cache
- **[Custom Backend](./examples/custom-backend/)** - Build your own cache backend

Each example includes:

- Complete working code
- Detailed README
- Use case explanations
- Best practices

## How It Works

httpcache implements RFC 7234 (HTTP Caching) by:

1. **Intercepting HTTP requests** through a custom `RoundTripper`
2. **Checking cache** for matching responses
3. **Validating freshness** using Cache-Control headers and Age calculation
4. **Revalidating** with ETag/Last-Modified when stale (respecting must-revalidate)
5. **Updating cache** with new responses
6. **Invalidating cache** on unsafe methods (POST, PUT, DELETE, PATCH)
7. **Adding headers** (Age, Warning) per RFC specifications

### Cache Headers Supported

**Request Headers:**

- `Cache-Control` (max-age, max-stale, min-fresh, no-cache, no-store, only-if-cached)
- `Pragma: no-cache` (HTTP/1.0 backward compatibility per RFC 7234 Section 5.4)
- `If-None-Match` (ETag validation)
- `If-Modified-Since` (Last-Modified validation)

**Response Headers:**

- `Cache-Control` (max-age, no-cache, no-store, must-revalidate, stale-if-error, stale-while-revalidate)
- `ETag` (entity tag validation)
- `Last-Modified` (date-based validation)
- `Expires` (expiration date)
- `Vary` (content negotiation)
- `Age` (time in cache per RFC 7234 Section 4.2.3)
- `Warning` (cache warnings per RFC 7234 Section 5.5)
- `stale-if-error` (RFC 5861)
- `stale-while-revalidate` (RFC 5861)

### Detecting Cache Hits

When `MarkCachedResponses` is enabled, cached responses include the `X-From-Cache` header set to "1".

Additionally, the `X-Cache-Freshness` header indicates the freshness state of the cached response:

- `fresh` - Response is within its max-age and can be served directly
- `stale` - Response has expired and will be revalidated
- `stale-while-revalidate` - Response is stale but can be served immediately while being revalidated asynchronously
- `transparent` - Response should not be served from cache

When a cached response is revalidated with the server (receiving a 304 Not Modified), the `X-Revalidated` header is also set to "1". This allows you to distinguish between:

- Responses served directly from cache (only `X-From-Cache: 1`)
- Responses that were revalidated with the server (both `X-From-Cache: 1` and `X-Revalidated: 1`)

When a stale response is served due to an error (using `stale-if-error`), the `X-Stale` header is set to "1". This indicates:

- Responses served from cache due to backend errors (has `X-From-Cache: 1` and `X-Stale: 1`)

## Advanced Features

### Transport Configuration

The `Transport` struct provides several configuration options:

```go
transport := httpcache.NewTransport(cache)

// Mark cached responses with X-From-Cache, X-Revalidated, and X-Stale headers
transport.MarkCachedResponses = true  // Default: true

// Skip serving server errors (5xx) from cache, even if fresh
// This forces a new request to the server for error responses
transport.SkipServerErrorsFromCache = true  // Default: false
```

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

### Custom Logger

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

### Stale-If-Error Support

Automatically serve stale cached content when the backend is unavailable:

```go
// Server returns 500, but cached response is served instead
resp, _ := client.Get(url) // Returns cached response, not 500 error
// Response will have X-From-Cache: 1 and X-Stale: 1 headers
```

This implements [RFC 5861](https://tools.ietf.org/html/rfc5861) for better resilience.

### Stale-While-Revalidate Support

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

### Cache Key Headers

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

### Custom Cache Control with ShouldCache

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

### Vary Header Support

⚠️ **Current Limitation**: The `Vary` response header is currently used for **validation only**, not for creating separate cache entries.

**What this means:**

- The cached response stores the values of headers specified in `Vary` (e.g., `Accept`, `Accept-Language`)
- When retrieving from cache, httpcache checks if the current request headers match the stored values
- If they don't match, the cache is considered invalid and a new request is made
- **However**, the new response **overwrites** the previous cache entry instead of creating a separate entry

**Example of current behavior:**

```go
// Server responds with: Vary: Accept

// Request 1: Accept: application/json
resp1, _ := client.Do(req1)  // Fetches from server, caches with Accept: application/json

// Request 2: Accept: text/html (different Accept header)
resp2, _ := client.Do(req2)  // Cache miss (doesn't match), fetches from server
                              // ❌ OVERWRITES previous cache entry

// Request 3: Accept: application/json (same as Request 1)
resp3, _ := client.Do(req3)  // ❌ Cache miss! (was overwritten by Request 2)
```

**Recommended Solution:**

Use `CacheKeyHeaders` to create true separate cache entries based on request headers:

```go
transport := httpcache.NewMemoryCacheTransport()
transport.CacheKeyHeaders = []string{"Accept", "Accept-Language"}

// Now each unique combination creates a separate cache entry
req1.Header.Set("Accept", "application/json")
client.Do(req1)  // Cached separately

req2.Header.Set("Accept", "text/html")
client.Do(req2)  // Cached separately (doesn't overwrite req1)

req3.Header.Set("Accept", "application/json")
client.Do(req3)  // ✅ Cache hit! (separate entry still exists)
```

**Note**: This limitation may be addressed in a future version to fully comply with RFC 7234 Section 4.1 (Vary header semantics).

### RFC 7234 Compliance Features

httpcache implements several important RFC 7234 features for production-ready HTTP caching:

#### Age Header (Section 4.2.3)

The `Age` header is automatically calculated and added to all cached responses, indicating how long the response has been in the cache:

```go
resp, _ := client.Get(url)
age := resp.Header.Get("Age")  // e.g., "120" (seconds)
// Clients can calculate: time_until_expiration = max-age - age
```

#### Warning Headers (Section 5.5)

Warning headers are automatically added to inform clients about cache conditions:

- `Warning: 110 - "Response is Stale"` - When serving stale content
- `Warning: 111 - "Revalidation Failed"` - When revalidation fails and stale content is served

```go
resp, _ := client.Get(url)
if warning := resp.Header.Get("Warning"); warning != "" {
    log.Printf("Cache warning: %s", warning)
}
```

#### must-revalidate Directive (Section 5.2.2.1)

The `must-revalidate` directive is enforced, ensuring that stale responses are always revalidated:

```go
// Server response: Cache-Control: max-age=60, must-revalidate
// After 60s, cache MUST revalidate (ignores client's max-stale)
```

This is critical for security-sensitive content that must not be served stale.

#### Pragma: no-cache Support (Section 5.4)

HTTP/1.0 backward compatibility via `Pragma: no-cache` request header:

```go
req, _ := http.NewRequest("GET", url, nil)
req.Header.Set("Pragma", "no-cache")
resp, _ := client.Do(req)
// Bypasses cache (when Cache-Control is absent)
```

#### Cache Invalidation (Section 4.4)

Cache is automatically invalidated for affected URIs when unsafe methods succeed:

```go
// POST/PUT/DELETE/PATCH with 2xx or 3xx response invalidates:
// - Request-URI
// - Location header URI (if present)
// - Content-Location header URI (if present)

client.Post(url, "application/json", body)  // Invalidates GET cache for url
```

This ensures cache consistency after data modifications.

### Custom Cache Implementation

Implement the `Cache` interface for custom backends:

```go
type Cache interface {
    Get(key string) (responseBytes []byte, ok bool)
    Set(key string, responseBytes []byte)
    Delete(key string)
}
```

See [examples/custom-backend](./examples/custom-backend/) for a complete example.

## Security Considerations

### Private Cache and Multi-User Applications

⚠️ **Important**: httpcache implements a **private cache** (similar to browser cache), not a shared cache. This has important implications for multi-user applications:

**The Problem:**

If you use the same `Transport` instance to make requests on behalf of different users, responses may be incorrectly shared between users unless properly configured:

```go
// ❌ DANGEROUS: Same transport for different users
transport := httpcache.NewMemoryCacheTransport()
client := transport.Client()

// User 1 requests their profile
req1, _ := http.NewRequest("GET", "https://api.example.com/user/profile", nil)
req1.Header.Set("Authorization", "Bearer user1_token")
client.Do(req1)  // Cached with key: https://api.example.com/user/profile

// User 2 requests their profile (same URL!)
req2, _ := http.NewRequest("GET", "https://api.example.com/user/profile", nil)
req2.Header.Set("Authorization", "Bearer user2_token")
client.Do(req2)  // ❌ Gets User 1's cached response!
```

**Solutions:**

1. **Use `CacheKeyHeaders`** to include user-identifying headers in cache keys:

```go
// ✅ SAFE: Different cache entries per Authorization token
transport := httpcache.NewMemoryCacheTransport()
transport.CacheKeyHeaders = []string{"Authorization"}
client := transport.Client()

// Each user gets their own cache entry
req1.Header.Set("Authorization", "Bearer user1_token")
client.Do(req1)  // Cached: https://api.example.com/user/profile|Authorization:Bearer user1_token

req2.Header.Set("Authorization", "Bearer user2_token")
client.Do(req2)  // Cached: https://api.example.com/user/profile|Authorization:Bearer user2_token
```

2. **Server-side `Vary` header** - ⚠️ **Current Limitation**: While the `Vary` response header is supported for validation, the current implementation **does NOT create separate cache entries** for different header values. Instead, it **overwrites the previous cache entry** with the same URL.

```go
// Server response headers:
// Cache-Control: max-age=3600
// Vary: Authorization

// ❌ CURRENT BEHAVIOR:
// Request 1 (Authorization: Bearer token1) -> Cached
// Request 2 (Authorization: Bearer token2) -> Overwrites previous cache
// Request 3 (Authorization: Bearer token1) -> Cache miss (was overwritten)

// ✅ USE CacheKeyHeaders INSTEAD for true separate cache entries:
transport.CacheKeyHeaders = []string{"Authorization"}
```

**Important**: If you rely on the server's `Vary` header for cache separation, you **must also configure `CacheKeyHeaders`** with the same headers to ensure separate cache entries are created. This is a known limitation that may be addressed in a future version.

3. **Prevent caching of user-specific data** - Use `Cache-Control` or `Pragma` headers:

```go
// Server response for sensitive user data:
// Cache-Control: private, no-store
// or
// Pragma: no-cache

// These responses will never be cached
```

**⚠️ Important Limitation**: httpcache currently **ignores** the `private` directive because it's designed as a "private cache". This means:

- `Cache-Control: private` does **NOT** prevent caching in httpcache
- This is **correct** for single-user scenarios (browser, CLI tool)
- This is **problematic** in multi-user scenarios (web server, API gateway)

**Why this matters:**

```go
// Server tries to prevent shared caching:
// HTTP/1.1 200 OK
// Cache-Control: private, max-age=3600
// {"user": "john", "email": "john@example.com"}

// httpcache IGNORES "private" and caches it anyway!
// If same Transport serves multiple users → data leak!
```

**Workarounds for multi-user applications:**

- **Best**: Use `Cache-Control: no-store` (httpcache respects this)
- **Alternative**: Configure `CacheKeyHeaders` to separate cache by user
- **Alternative**: Use separate Transport instances per user

4. **Separate Transport per user** - Create individual cache instances:

```go
// ✅ SAFE: Each user has isolated cache
func getClientForUser(userID string) *http.Client {
    cache := diskcache.New(fmt.Sprintf("/tmp/cache/%s", userID))
    transport := httpcache.NewTransport(cache)
    return &http.Client{Transport: transport}
}
```

**When is this a concern?**

- ✅ **Web servers** handling requests from multiple users
- ✅ **API gateways** proxying authenticated requests
- ✅ **Background workers** processing jobs for different accounts
- ❌ **CLI tools** (single user per instance)
- ❌ **Desktop apps** (single user per instance)
- ❌ **Single-user services**

**Best Practice:**

Always use `CacheKeyHeaders` or ensure the server sends appropriate `Vary` headers when caching user-specific or tenant-specific data.

⚠️ **Security Risk**: When using `CacheKeyHeaders` with sensitive headers (e.g., `Authorization`, `X-API-Key`), these values may be stored **in plain text** in the cache backend.

## Monitoring with Prometheus (Optional)

httpcache includes **optional** Prometheus metrics integration to monitor cache performance, HTTP requests, and resource usage. The metrics system is:

- **Zero-dependency by default** - Metrics are opt-in and don't add dependencies to the core package
- **Non-intrusive** - Works with any existing cache backend
- **Production-ready** - Battle-tested metric types and labels
- **Integration-friendly** - Easily integrates with existing Prometheus setups

### Quick Start

```go
import (
    "github.com/sandrolain/httpcache"
    prommetrics "github.com/sandrolain/httpcache/metrics/prometheus"
)

// Create metrics collector
collector := prommetrics.NewCollector()

// Wrap your cache
cache := httpcache.NewMemoryCache()
instrumentedCache := prommetrics.NewInstrumentedCache(cache, "memory", collector)

// Wrap your transport
transport := httpcache.NewTransport(instrumentedCache)
instrumentedTransport := prommetrics.NewInstrumentedTransport(transport, collector)

// Use the instrumented client
client := instrumentedTransport.Client()

// Metrics are automatically exposed via Prometheus default registry
// Access them at your /metrics endpoint
```

### Available Metrics

#### Cache Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `httpcache_cache_requests_total` | Counter | `backend`, `operation`, `result` | Total cache operations (get/set/delete) |
| `httpcache_cache_operation_duration_seconds` | Histogram | `backend`, `operation` | Cache operation latency |
| `httpcache_cache_size_bytes` | Gauge | `backend` | Current cache size in bytes |
| `httpcache_cache_entries` | Gauge | `backend` | Number of cached entries |

#### HTTP Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `httpcache_http_requests_total` | Counter | `method`, `cache_status` | HTTP requests by cache hit/miss/revalidated |
| `httpcache_http_request_duration_seconds` | Histogram | `method`, `cache_status` | HTTP request duration |
| `httpcache_http_response_size_bytes_total` | Counter | `method`, `cache_status` | Total response sizes |
| `httpcache_stale_responses_total` | Counter | `method` | Stale responses served (RFC 5861) |

### Example PromQL Queries

**Cache hit rate:**

```promql
rate(httpcache_cache_requests_total{result="hit"}[5m]) /
rate(httpcache_cache_requests_total{operation="get"}[5m]) * 100
```

**P95 latency:**

```promql
histogram_quantile(0.95,
  rate(httpcache_cache_operation_duration_seconds_bucket[5m]))
```

**Bandwidth saved:**

```promql
httpcache_http_response_size_bytes_total{cache_status="hit"}
```

### Integration with Existing Metrics

If your application already has Prometheus metrics, httpcache metrics are automatically included:

```go
// Your existing Prometheus setup
http.Handle("/metrics", promhttp.Handler())

// httpcache metrics use the default registry
collector := prommetrics.NewCollector()

// All metrics are exposed together at /metrics
```

For custom namespaces or registries:

```go
customRegistry := prometheus.NewRegistry()
collector := prommetrics.NewCollectorWithConfig(prommetrics.CollectorConfig{
    Registry:  customRegistry,
    Namespace: "myapp", // Use "myapp_cache_requests_total" instead of "httpcache_..."
})
```

### Full Example

See [examples/prometheus](./examples/prometheus/) for a complete working example that demonstrates:

- Setting up Prometheus metrics collection
- Making HTTP requests with cache hits/misses
- Accessing metrics via HTTP endpoint
- Example PromQL queries for monitoring
- Integration with existing Prometheus setups

### Production Considerations

- **Low cardinality** - Default labels are carefully chosen to avoid cardinality explosion
- **Meaningful names** - Use descriptive backend names (e.g., "redis-sessions", "disk-api-cache")
- **Custom namespaces** - Avoid metric name conflicts in multi-cache scenarios
- **Alerting** - Set up alerts for low hit rates, high latency, or resource limits

For detailed information, see the [Prometheus example documentation](./examples/prometheus/README.md).

## Limitations

- **Private cache only** - Not suitable for shared proxy caching
- **No automatic eviction** - MemoryCache grows unbounded (use size-limited backends)
- **GET/HEAD only** - Only caches GET and HEAD requests
- **No range requests** - Range requests bypass the cache

## Performance

Typical performance characteristics:

| Operation | Memory | Disk | LevelDB | Redis (local) |
|-----------|--------|------|---------|---------------|
| Cache Hit | ~1µs | ~1ms | ~100µs | ~1ms |
| Cache Miss | Network latency + ~1µs overhead ||||
| Storage | RAM | Disk | Disk (compressed) | RAM/Disk |

*Benchmarks vary based on response size, hardware, and network conditions.*

## Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run benchmarks
go test -bench=. ./...
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes with tests
4. Submit a pull request

## Documentation

- [GoDoc](https://godoc.org/github.com/sandrolain/httpcache) - API documentation
- [Examples](./examples/) - Practical usage examples

## Acknowledgments

This project is a maintained fork of [gregjones/httpcache](https://github.com/gregjones/httpcache), originally created by [@gregjones](https://github.com/gregjones). The original project was archived in 2023.

We're grateful for the original work and continue to maintain this project with:

- Bug fixes and security updates
- Modern Go practices and tooling
- Enhanced documentation and examples
- Backward compatibility with the original

## License

[MIT License](LICENSE.txt)

Copyright (c) 2012 Greg Jones (original)  
Copyright (c) 2025 Sandro Lain (fork maintainer)

## Support

- 📖 [Documentation](https://godoc.org/github.com/sandrolain/httpcache)
- 💬 [Issues](https://github.com/sandrolain/httpcache/issues)
- 🔧 [Examples](./examples/)
