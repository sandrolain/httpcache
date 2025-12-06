# Security Considerations

## Built-in Security Features (v2.0)

httpcache v2.0 includes built-in security features:

### SHA-256 Key Hashing (Always Enabled)

All cache keys are automatically hashed with SHA-256 before being passed to the backend. This provides:

- **Privacy**: Original URLs/keys are not exposed in the cache backend
- **Consistency**: Same input always produces the same hash
- **Security**: Prevents enumeration of cached URLs

No configuration required - this is always enabled by default.

### Optional AES-256-GCM Encryption

Enable encryption of cached data using `WithEncryption`:

```go
import "github.com/sandrolain/httpcache"

cache := diskcache.New("/tmp/cache")
transport := httpcache.NewTransport(cache,
    httpcache.WithEncryption("your-secret-passphrase"),
)
client := transport.Client()
```

**Security Features:**

- ✓ **AES-256-GCM Encryption** - Industry-standard authenticated encryption
- ✓ **scrypt Key Derivation** - Strong passphrase protection (N=32768, r=8, p=1)
- ✓ **Unique Nonce per Value** - Prevents IV reuse attacks
- ✓ **Authentication Tag** - Prevents tampering with cached data

### Check Encryption Status

```go
if transport.IsEncryptionEnabled() {
    fmt.Println("Cache is using encryption")
}
```

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
client.Do(req1)  // Cached with key: SHA-256(https://api.example.com/user/profile)

// User 2 requests their profile (same URL!)
req2, _ := http.NewRequest("GET", "https://api.example.com/user/profile", nil)
req2.Header.Set("Authorization", "Bearer user2_token")
client.Do(req2)  // ❌ Gets User 1's cached response!
```

### Solutions

#### 1. Use `WithCacheKeyHeaders` (Recommended)

Include user-identifying headers in cache keys:

```go
// ✅ SAFE: Different cache entries per Authorization token
cache := diskcache.New("/tmp/cache")
transport := httpcache.NewTransport(cache,
    httpcache.WithCacheKeyHeaders([]string{"Authorization"}),
)
client := transport.Client()

// Each user gets their own cache entry
req1.Header.Set("Authorization", "Bearer user1_token")
client.Do(req1)  // Cached: SHA-256(URL|Authorization:Bearer user1_token)

req2.Header.Set("Authorization", "Bearer user2_token")
client.Do(req2)  // Cached: SHA-256(URL|Authorization:Bearer user2_token)
```

#### 2. Server-side `Vary` Header

⚠️ **Current Limitation**: While the `Vary` response header is supported for validation, the current implementation **does NOT create separate cache entries** for different header values by default.

Enable Vary separation with `WithVarySeparation`:

```go
transport := httpcache.NewTransport(cache,
    httpcache.WithVarySeparation(true),
)
```

Or combine with `WithCacheKeyHeaders` for explicit control:

```go
transport := httpcache.NewTransport(cache,
    httpcache.WithCacheKeyHeaders([]string{"Authorization"}),
)
```

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

**Workarounds for multi-user applications:**

- **Best**: Use `Cache-Control: no-store` (httpcache respects this)
- **Alternative**: Configure `WithCacheKeyHeaders` to separate cache by user
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

Always use `WithCacheKeyHeaders` or ensure the server sends appropriate `Vary` headers when caching user-specific or tenant-specific data.

## Complete Security Example

```go
import (
    "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/redis"
)

// Get passphrase from secure storage
passphrase := os.Getenv("CACHE_PASSPHRASE")
if passphrase == "" {
    log.Fatal("CACHE_PASSPHRASE environment variable required")
}

// Connect to Redis
redisCache := redis.NewWithClient(redisConn)

// Create secure transport
transport := httpcache.NewTransport(redisCache,
    httpcache.WithEncryption(passphrase),
    httpcache.WithCacheKeyHeaders([]string{"Authorization", "X-User-ID"}),
)

client := transport.Client()
```

**Security Features:**

- ✓ **SHA-256 Key Hashing** (always enabled) - Prevents key enumeration
- ✓ **AES-256-GCM Encryption** - Encrypts cached data
- ✓ **Authenticated Encryption** - Prevents tampering
- ✓ **scrypt Key Derivation** - Strong passphrase protection
- ✓ **User-specific Cache Keys** - Prevents cross-user data leakage

## Additional Security Recommendations

1. **Use HTTPS** for all cached requests
2. **Set appropriate timeouts** to prevent resource exhaustion
3. **Limit cache size** to prevent DoS attacks
4. **Monitor cache hit rates** to detect anomalies
5. **Rotate encryption passphrases** periodically
6. **Use secure cache backends** (encrypted Redis, PostgreSQL with TLS)
7. **Audit cache access** in production environments

## Passphrase Management

### Generation

Use a cryptographically secure random generator:

```bash
# Generate a strong passphrase
openssl rand -base64 32
```

### Storage

Use a secret management system:

- AWS Secrets Manager
- HashiCorp Vault
- Kubernetes Secrets
- Environment variables (minimum)

### Rotation

Plan for passphrase rotation:

- Changing passphrase invalidates all cached data
- Consider blue/green deployment with cache warming

## Compliance

This implementation helps meet requirements for:

- **GDPR** (General Data Protection Regulation)
  - Article 32: Security of processing
  - Article 25: Data protection by design

- **CCPA** (California Consumer Privacy Act)
  - Security safeguards for personal information

- **HIPAA** (Health Insurance Portability and Accountability Act)
  - PHI (Protected Health Information) encryption

- **PCI DSS** (Payment Card Industry Data Security Standard)
  - Requirement 3: Protect stored cardholder data
  - Requirement 4: Encrypt transmission of cardholder data

**Note**: This package provides the technical implementation. Compliance also requires proper key management, access controls, audit logging, and other organizational measures.
