# Security Considerations

## Private Cache and Multi-User Applications

⚠️ **Important**: httpcache implements a **private cache** (similar to browser cache), not a shared cache. This has important implications for multi-user applications.

### The Problem

If you use the same `Transport` instance to make requests on behalf of different users, responses may be incorrectly shared between users unless properly configured:

```go
import "github.com/sandrolain/httpcache/diskcache"

// ❌ DANGEROUS: Same transport for different users
cache := diskcache.New("/tmp/cache")
transport := httpcache.NewTransport(cache)
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

### Solutions

#### 1. Use `CacheKeyHeaders`

Include user-identifying headers in cache keys:

```go
// ✅ SAFE: Different cache entries per Authorization token
cache := diskcache.New("/tmp/cache")
transport := httpcache.NewTransport(cache)
transport.CacheKeyHeaders = []string{"Authorization"}
client := transport.Client()

// Each user gets their own cache entry
req1.Header.Set("Authorization", "Bearer user1_token")
client.Do(req1)  // Cached: https://api.example.com/user/profile|Authorization:Bearer user1_token

req2.Header.Set("Authorization", "Bearer user2_token")
client.Do(req2)  // Cached: https://api.example.com/user/profile|Authorization:Bearer user2_token
```

#### 2. Server-side `Vary` Header

⚠️ **Current Limitation**: While the `Vary` response header is supported for validation, the current implementation **does NOT create separate cache entries** for different header values. Instead, it **overwrites the previous cache entry** with the same URL.

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

#### 3. Prevent Caching of User-Specific Data

Use `Cache-Control` or `Pragma` headers:

```go
// Server response for sensitive user data:
// Cache-Control: private, no-store
// or
// Pragma: no-cache

// These responses will never be cached
```

⚠️ **Important Limitation**: httpcache currently **ignores** the `private` directive because it's designed as a "private cache". This means:

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

#### 4. Separate Transport Per User

Create individual cache instances:

```go
// ✅ SAFE: Each user has isolated cache
func getClientForUser(userID string) *http.Client {
    cache := diskcache.New(fmt.Sprintf("/tmp/cache/%s", userID))
    transport := httpcache.NewTransport(cache)
    return &http.Client{Transport: transport}
}
```

### When Is This a Concern?

- ✅ **Web servers** handling requests from multiple users
- ✅ **API gateways** proxying authenticated requests
- ✅ **Background workers** processing jobs for different accounts
- ❌ **CLI tools** (single user per instance)
- ❌ **Desktop apps** (single user per instance)
- ❌ **Single-user services**

### Best Practice

Always use `CacheKeyHeaders` or ensure the server sends appropriate `Vary` headers when caching user-specific or tenant-specific data.

## Secure Cache Wrapper

⚠️ **Security Risk**: When using `CacheKeyHeaders` with sensitive headers (e.g., `Authorization`, `X-API-Key`), these values may be stored **in plain text** in the cache backend.

**Solution**: Use the [`securecache`](../wrapper/securecache/README.md) wrapper to add encryption:

```go
import (
    "github.com/sandrolain/httpcache/wrapper/securecache"
    "github.com/sandrolain/httpcache/redis"
)

// Wrap any backend with security layer
redisCache := redis.NewWithClient(redisConn)
secureCache, _ := securecache.New(securecache.Config{
    Cache:      redisCache,
    Passphrase: os.Getenv("CACHE_PASSPHRASE"),
})

transport := httpcache.NewTransport(secureCache)
transport.CacheKeyHeaders = []string{"Authorization"}
```

**Security Features:**

- ✓ **SHA-256 Key Hashing** (always enabled) - Prevents key enumeration
- ✓ **AES-256-GCM Encryption** (optional) - Encrypts cached data
- ✓ **Authenticated Encryption** - Prevents tampering
- ✓ **scrypt Key Derivation** - Strong passphrase protection

See [`securecache/README.md`](../wrapper/securecache/README.md) for details.

## Additional Security Recommendations

1. **Use HTTPS** for all cached requests
2. **Set appropriate timeouts** to prevent resource exhaustion
3. **Limit cache size** to prevent DoS attacks
4. **Monitor cache hit rates** to detect anomalies
5. **Rotate encryption passphrases** periodically
6. **Use secure cache backends** (encrypted Redis, PostgreSQL with TLS)
7. **Audit cache access** in production environments
