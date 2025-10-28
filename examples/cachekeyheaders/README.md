# Cache Key Headers Example

This example demonstrates how to use `CacheKeyHeaders` to create separate cache entries based on request header values.

## What This Example Shows

- How to configure `CacheKeyHeaders` for multiple headers
- Different cache entries for different Authorization tokens
- Different cache entries for different Accept-Language values
- How cache hits and misses work with header-based keys

## Use Cases

This feature is useful for:

1. **User-Specific Caching**: Each user (identified by Authorization header) gets their own cache
2. **Internationalization**: Different languages get separate cached responses
3. **Multi-Tenant Applications**: Different tenants get separate cache entries
4. **API Versioning**: Different API versions can have separate caches

## Prerequisites

This example requires internet access to reach `httpbin.org`.

## Running the Example

```bash
cd examples/cachekeyheaders
go run main.go
```

## Expected Output

```
=== Cache Key Headers Example ===

Scenario: Different Authorization tokens should have separate cache entries

1. First request with Authorization: Bearer token1
   Request 1: ✗ CACHE MISS (fetched from server)
   Cache-Control: ...

2. Second request with Authorization: Bearer token2
   Request 2: ✗ CACHE MISS (fetched from server)
   Cache-Control: ...

3. Third request with Authorization: Bearer token1 (same as first)
   Request 3: ✓ CACHE HIT (Freshness: fresh)

4. Fourth request with Authorization: Bearer token1, Accept-Language: it
   Request 4: ✗ CACHE MISS (fetched from server)
   Cache-Control: ...

5. Fifth request (same as fourth, should be cached)
   Request 5: ✓ CACHE HIT (Freshness: fresh)

=== Summary ===
✓ Request 1: Cache MISS (new token1 + en)
✓ Request 2: Cache MISS (new token2 + en)
✓ Request 3: Cache HIT (same as request 1: token1 + en)
✓ Request 4: Cache MISS (new combination: token1 + it)
✓ Request 5: Cache HIT (same as request 4: token1 + it)

Each unique combination of Authorization + Accept-Language creates a separate cache entry!
```

## How It Works

1. **Configuration**: `transport.CacheKeyHeaders = []string{"Authorization", "Accept-Language"}`
   - Tells httpcache to include these header values in the cache key

2. **Cache Key Generation**:
   - Without headers: `https://httpbin.org/headers`
   - With headers: `https://httpbin.org/headers|Accept-Language:en|Authorization:Bearer token1`

3. **Separate Entries**: Each unique combination of header values creates a distinct cache entry
   - `token1 + en` → Cache entry 1
   - `token2 + en` → Cache entry 2
   - `token1 + it` → Cache entry 3

## Important Notes

- Headers are case-insensitive (automatically canonicalized)
- Headers are sorted alphabetically in the cache key for consistency
- Only non-empty header values are included
- This is different from the HTTP `Vary` response header mechanism

## Real-World Example

In a real application, you might use this for:

```go
// API client with per-user caching
transport := httpcache.NewMemoryCacheTransport()
transport.CacheKeyHeaders = []string{"Authorization"}

client := transport.Client()

// Each user gets their own cached responses
req1, _ := http.NewRequest("GET", "https://api.example.com/user/profile", nil)
req1.Header.Set("Authorization", "Bearer user1_token")
resp1, _ := client.Do(req1)

req2, _ := http.NewRequest("GET", "https://api.example.com/user/profile", nil)
req2.Header.Set("Authorization", "Bearer user2_token")
resp2, _ := client.Do(req2)

// user1 and user2 have separate cache entries
```

## See Also

- [Main README](../../README.md#cache-key-headers) - Full documentation
- [Basic Example](../basic/) - Simple caching example
- [Custom Backend Example](../custom-backend/) - Custom cache implementations
