# How It Works

httpcache implements RFC 7234 (HTTP Caching) by:

1. **Intercepting HTTP requests** through a custom `RoundTripper`
2. **Checking cache** for matching responses
3. **Validating freshness** using Cache-Control headers and Age calculation
4. **Revalidating** with ETag/Last-Modified when stale (respecting must-revalidate)
5. **Updating cache** with new responses
6. **Invalidating cache** on unsafe methods (POST, PUT, DELETE, PATCH)
7. **Adding headers** (Age, Warning) per RFC specifications

## Cache Headers Supported

### Request Headers

- `Cache-Control` (max-age, max-stale, min-fresh, no-cache, no-store, only-if-cached)
- `Pragma: no-cache` (HTTP/1.0 backward compatibility per RFC 7234 Section 5.4)
- `If-None-Match` (ETag validation)
- `If-Modified-Since` (Last-Modified validation)

### Response Headers

- `Cache-Control` (max-age, no-cache, no-store, must-revalidate, stale-if-error, stale-while-revalidate)
- `ETag` (entity tag validation)
- `Last-Modified` (date-based validation)
- `Expires` (expiration date)
- `Vary` (content negotiation)
- `Age` (time in cache per RFC 7234 Section 4.2.3)
- `Warning` (cache warnings per RFC 7234 Section 5.5)
- `stale-if-error` (RFC 5861)
- `stale-while-revalidate` (RFC 5861)

## Detecting Cache Hits

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

## Vary Header Support

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

## RFC 7234 Compliance Features

httpcache implements several important RFC 7234 features for production-ready HTTP caching:

### Age Header (Section 4.2.3)

The `Age` header is automatically calculated and added to all cached responses, indicating how long the response has been in the cache:

```go
resp, _ := client.Get(url)
age := resp.Header.Get("Age")  // e.g., "120" (seconds)
// Clients can calculate: time_until_expiration = max-age - age
```

### Warning Headers (Section 5.5)

Warning headers are automatically added to inform clients about cache conditions:

- `Warning: 110 - "Response is Stale"` - When serving stale content
- `Warning: 111 - "Revalidation Failed"` - When revalidation fails and stale content is served

```go
resp, _ := client.Get(url)
if warning := resp.Header.Get("Warning"); warning != "" {
    log.Printf("Cache warning: %s", warning)
}
```

### must-revalidate Directive (Section 5.2.2.1)

The `must-revalidate` directive is enforced, ensuring that stale responses are always revalidated:

```go
// Server response: Cache-Control: max-age=60, must-revalidate
// After 60s, cache MUST revalidate (ignores client's max-stale)
```

This is critical for security-sensitive content that must not be served stale.

### Pragma: no-cache Support (Section 5.4)

HTTP/1.0 backward compatibility via `Pragma: no-cache` request header:

```go
req, _ := http.NewRequest("GET", url, nil)
req.Header.Set("Pragma", "no-cache")
resp, _ := client.Do(req)
// Bypasses cache (when Cache-Control is absent)
```

### Cache Invalidation (Section 4.4)

Cache is automatically invalidated for affected URIs when unsafe methods (POST, PUT, DELETE, PATCH) receive successful responses (2xx or 3xx status codes):

```go
// POST/PUT/DELETE/PATCH with 2xx or 3xx response invalidates:
// 1. Request-URI
// 2. Location header URI (if present, same-origin only)
// 3. Content-Location header URI (if present, same-origin only)

client.Post(url, "application/json", body)  // Invalidates GET cache for url
```

This ensures cache consistency after data modifications per RFC 9111 Section 4.4.

#### Content-Location and Location Header Invalidation

httpcache implements **RFC 9111 Section 4.4** compliant invalidation with enhanced support for `Content-Location` and `Location` headers:

**Key Features:**

- **Same-origin enforcement**: Only invalidates URIs on the same origin (scheme://host:port) to prevent cross-origin cache poisoning
- **Relative URI support**: Properly resolves relative URIs in headers
- **Error response protection**: Skips invalidation for error responses (status >= 400)
- **Graceful error handling**: Invalid URIs are logged without causing failures

**Example - RESTful API with Content-Location:**

```go
// Server responds to PUT /api/users/123 with:
// Status: 200 OK
// Content-Location: /api/users/123
// This invalidates the cached GET /api/users/123

client := transport.Client()
req, _ := http.NewRequest("PUT", "https://api.example.com/api/users/123", body)
resp, _ := client.Do(req)
// Cache for GET https://api.example.com/api/users/123 is now invalidated
```

**Example - Resource Creation with Location:**

```go
// Server responds to POST /api/users with:
// Status: 201 Created
// Location: /api/users/456
// This invalidates the cached GET /api/users/456

resp, _ := client.Post("https://api.example.com/api/users", "application/json", body)
// Cache for GET https://api.example.com/api/users/456 is now invalidated
```

**Security - Same-origin protection:**

```go
// Cross-origin Content-Location is IGNORED for security
// POST to https://api.example.com/resource
// Response: Content-Location: https://evil.com/resource
// ✅ https://evil.com/resource is NOT invalidated (different origin)
// ✅ Only same-origin URIs are invalidated
```

**Error response handling:**

```go
// Error responses (5xx) do NOT trigger invalidation
// PUT /api/users/123 returns 500 Internal Server Error
// ✅ Cache for /api/users/123 remains valid
// This prevents cache invalidation on temporary failures
```

**Use Cases:**

1. **RESTful APIs**: Automatic cache invalidation when resources are modified
2. **Content Negotiation**: Invalidate specific content variants via Content-Location
3. **Resource Creation**: Location header points to newly created resource
4. **Redirects**: Location header invalidates redirect target cache
5. **Multi-representation resources**: Invalidate both request URI and Content-Location

When debugging is enabled, invalidation actions are logged for troubleshooting.

## Custom Cache Implementation

Implement the `Cache` interface for custom backends:

```go
type Cache interface {
    Get(key string) (responseBytes []byte, ok bool)
    Set(key string, responseBytes []byte)
    Delete(key string)
}
```

See [examples/custom-backend](../examples/custom-backend/) for a complete example.
