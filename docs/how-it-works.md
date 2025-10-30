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

✅ **RFC 9111 Compliance** (Optional): httpcache supports **full Vary header separation** as specified in RFC 9111 Section 4.1 when `EnableVarySeparation` is set to `true`.

**Configuration:**

```go
transport := httpcache.NewMemoryCacheTransport()
transport.EnableVarySeparation = true  // Enable RFC 9111 compliant vary separation
```

**Default Behavior (EnableVarySeparation = false):**

- Responses with Vary headers use the Vary header for **validation only**
- New variants **overwrite** previous cache entries for the same URL
- This is the legacy behavior maintained for backward compatibility

**New Behavior (EnableVarySeparation = true):**

- Responses with Vary headers create **separate cache entries** for each variant
- Each unique combination of vary header values gets its own cache entry
- Variants do not overwrite each other
- Full RFC 9111 compliance for content negotiation

**How it works when enabled:**

- When a response includes a `Vary` header (e.g., `Vary: Accept-Language`), httpcache creates separate cache entries for each unique combination of vary header values
- Each variant is stored with a cache key that includes both the URL and the values of the varied headers
- Subsequent requests automatically retrieve the correct variant based on their header values
- This ensures proper content negotiation and prevents variants from overwriting each other

**Example with EnableVarySeparation = true:**

```go
transport := httpcache.NewMemoryCacheTransport()
transport.EnableVarySeparation = true  // Enable vary separation

// Server responds with: Vary: Accept-Language, Cache-Control: max-age=3600

// Request 1: Accept-Language: en
resp1, _ := client.Do(req1)  // Fetches from server, caches English variant
// Cache key: "GET|https://example.com/api|vary:Accept-Language:en"

// Request 2: Accept-Language: fr (different language)
resp2, _ := client.Do(req2)  // Fetches from server, caches French variant
// Cache key: "GET|https://example.com/api|vary:Accept-Language:fr"
// ✅ DOES NOT overwrite English variant

// Request 3: Accept-Language: en (same as Request 1)
resp3, _ := client.Do(req3)  // ✅ Cache hit! Returns English variant
```

**Example with EnableVarySeparation = false (default):**

```go
// Default behavior - variants overwrite each other
transport := httpcache.NewMemoryCacheTransport()
// EnableVarySeparation defaults to false

// Request 1: Accept-Language: en
resp1, _ := client.Do(req1)  // Fetches from server, caches with Accept-Language: en

// Request 2: Accept-Language: fr (different language)
resp2, _ := client.Do(req2)  // Cache miss (doesn't match), fetches from server
// ❌ OVERWRITES previous cache entry

// Request 3: Accept-Language: en (same as Request 1)
resp3, _ := client.Do(req3)  // ❌ Cache miss! (was overwritten by Request 2)
```

**Multiple Vary headers:**

When a response varies by multiple headers, all are included in the cache key:

```go
transport.EnableVarySeparation = true

// Server responds with: Vary: Accept, Accept-Language

req1.Header.Set("Accept", "application/json")
req1.Header.Set("Accept-Language", "en")
client.Do(req1)  // Cache key includes both headers

req2.Header.Set("Accept", "application/json")
req2.Header.Set("Accept-Language", "fr")
client.Do(req2)  // Different cache entry (different language)

req3.Header.Set("Accept", "text/html")
req3.Header.Set("Accept-Language", "en")
client.Do(req3)  // Different cache entry (different Accept)
```

**Additional control with CacheKeyHeaders:**

You can still use `CacheKeyHeaders` for custom cache separation beyond server-specified Vary headers:

```go
transport := httpcache.NewMemoryCacheTransport()
transport.EnableVarySeparation = true
// Separate cache entries by user, even if server doesn't specify Vary
transport.CacheKeyHeaders = []string{"X-User-ID"}

req1.Header.Set("X-User-ID", "user-123")
client.Do(req1)  // Cached separately for user-123

req2.Header.Set("X-User-ID", "user-456")
client.Do(req2)  // Cached separately for user-456
```

**When to enable vary separation:**

- ✅ Enable when you need full RFC 9111 compliance
- ✅ Enable for proper content negotiation (language-specific content, different formats)
- ✅ Enable when caching APIs that return different content based on Accept headers
- ⚠️ Be aware that this may increase cache storage usage
- ⚠️ Default is disabled for backward compatibility

**Benefits when enabled:**

- ✅ Full RFC 9111 compliance for content negotiation
- ✅ Correctly handles language-specific content (Accept-Language)
- ✅ Supports multiple content types (Accept)
- ✅ Works with encoding preferences (Accept-Encoding)
- ✅ Prevents cache pollution from mixed variants
- ✅ Each variant is cached independently

## must-understand Directive Support (RFC 9111 Section 5.2.2.3)

The `must-understand` directive is a new cache directive introduced in RFC 9111 that allows responses to be cached even when they would normally be prohibited by other directives, **if and only if** the cache understands the response's status code.

### How it Works

The `must-understand` directive modifies the caching behavior as follows:

1. **Known Status Codes**: If the response status code is "understood" by the cache (see list below), the response CAN be cached even if `no-store` is present
2. **Unknown Status Codes**: If the response status code is NOT understood, the response MUST NOT be cached, regardless of other directives

### Understood Status Codes

httpcache recognizes the following HTTP status codes as "understood":

- **2xx Success**: 200 (OK), 203 (Non-Authoritative Information), 204 (No Content), 206 (Partial Content)
- **3xx Redirection**: 300 (Multiple Choices), 301 (Moved Permanently)
- **4xx Client Errors**: 404 (Not Found), 405 (Method Not Allowed), 410 (Gone), 414 (URI Too Long)
- **5xx Server Errors**: 501 (Not Implemented)

### Examples

**Example 1: Known status + must-understand + no-store → CACHED**

```go
// Server response:
HTTP/1.1 200 OK
Cache-Control: must-understand, no-store, max-age=3600
Content-Type: application/json

// Result: Response IS cached because:
// - Status 200 is understood
// - must-understand is present
// - must-understand overrides no-store for understood status codes
```

**Example 2: Unknown status + must-understand → NOT CACHED**

```go
// Server response:
HTTP/1.1 418 I'm a teapot
Cache-Control: must-understand, max-age=3600
Content-Type: text/plain

// Result: Response is NOT cached because:
// - Status 418 is NOT in the understood list
// - must-understand requires understood status for caching
```

**Example 3: Known status + must-understand + private → Respects IsPublicCache**

```go
// Private cache (default):
transport := httpcache.NewMemoryCacheTransport()
transport.IsPublicCache = false  // default

// Server response:
HTTP/1.1 200 OK
Cache-Control: must-understand, private, max-age=3600

// Result: Response IS cached (private cache can cache private responses)

// Public cache:
transport.IsPublicCache = true

// Result: Response is NOT cached (public cache cannot cache private responses)
```

### Use Cases

The `must-understand` directive is useful when:

1. **Custom Status Codes**: You use custom or extended HTTP status codes and want to ensure caches only cache them if they understand what they mean
2. **Sensitive Errors**: You want to cache certain error responses (like 404) but not others
3. **Future-Proofing**: Your API evolves with new status codes, and older caches won't accidentally cache responses they don't understand

### Implementation Notes

- The directive is parsed automatically from `Cache-Control` headers
- No configuration needed - works out of the box
- Compatible with all other cache directives (except `no-store` for understood status codes)
- Follows RFC 9111 Section 5.2.2.3 specification exactly

## RFC 7234 Compliance Features

httpcache implements several important RFC 7234 features for production-ready HTTP caching:

### Age Header (RFC 9111 Section 4.2.3)

The `Age` header is automatically calculated and added to all cached responses using the complete RFC 9111 formula. This header indicates how long the response has been in the cache and helps clients determine remaining freshness lifetime.

#### RFC 9111 Age Calculation Algorithm

httpcache implements the full RFC 9111 Section 4.2.3 algorithm for precise Age calculation:

```text
apparent_age = max(0, response_time - date_value)
response_delay = response_time - request_time
corrected_age_value = age_value + response_delay
corrected_initial_age = max(apparent_age, corrected_age_value)
resident_time = now - response_time
current_age = corrected_initial_age + resident_time
```

**Components:**

- **request_time**: When the HTTP request was initiated (tracked in `X-Request-Time` header)
- **response_time**: When the HTTP response was received (tracked in `X-Response-Time` header)
- **date_value**: Value from the `Date` header in the response
- **age_value**: Value from the `Age` header (if present from origin server)
- **response_delay**: Network transmission delay
- **resident_time**: How long the response has been stored in cache

#### Age Header Validation (RFC 9111 Section 5.1)

httpcache validates Age headers according to RFC 9111 Section 5.1 requirements:

- **Multiple Age headers**: Uses the first value, logs warning about others
- **Invalid values**: Rejects negative numbers, non-numeric values, floats
- **Graceful handling**: Logs warnings for invalid Age headers without failing requests

```go
// Example: Multiple Age headers
// Age: 300
// Age: 600
// Result: Uses 300 (first value), logs warning

// Example: Invalid Age header
// Age: -100
// Result: Ignores header, logs warning

// Example: Non-numeric Age header
// Age: invalid
// Result: Ignores header, logs warning
```

#### Usage Example

```go
resp, _ := client.Get(url)
age := resp.Header.Get("Age")  // e.g., "120" (seconds)

// Calculate remaining freshness lifetime
maxAge := getMaxAge(resp)  // From Cache-Control: max-age
remainingLife := maxAge - age  // Time until expiration
```

#### Precision and Timing

The Age calculation maintains high precision by:

1. Tracking exact request and response times
2. Accounting for network transmission delays
3. Using RFC3339 timestamp format for internal storage
4. Recalculating Age on every cache retrieval

#### Backward Compatibility

The implementation maintains backward compatibility:

- Falls back to `X-Cached-Time` header when `X-Response-Time` is not present
- Supports simplified Age calculation for legacy cached responses
- Provides smooth migration path from older cache entries

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
