# Security Considerations

## Built-in Security Features (v2.0)

httpcache v2.0 includes built-in security features:

### Key Hashing (Always Enabled)

All cache keys are automatically hashed before being passed to the backend. Two algorithms are available:

#### SHA-256 (Default)

```go
cache := diskcache.New("/tmp/cache")
transport := httpcache.NewTransport(cache)
// SHA-256 is used by default
```

**SHA-256 Characteristics:**

- ✓ **Cryptographically Secure** - 256-bit hash function
- ✓ **Privacy**: Original URLs/keys are not exposed in cache backend
- ✓ **Consistency**: Same input always produces same hash
- ✓ **Security**: Prevents enumeration of cached URLs
- ✓ **Collision Resistance**: ~0% probability for practical purposes
- ✓ **Backward Compatible**: Default algorithm
- ⚠️ **Performance**: ~149 ns/op

**Best For:** Security-sensitive applications, distributed caches across trust boundaries, existing deployments.

#### xxHash (High Performance)

```go
cache := diskcache.New("/tmp/cache")
transport := httpcache.NewTransport(cache,
    httpcache.WithHashAlgorithm(httpcache.HashAlgorithmXXHash),
)
```

**xxHash Characteristics:**

- ✓ **High Performance** - ~2.7x faster than SHA-256 (~54 ns/op)
- ✓ **Small Output** - 72% smaller keys (12 chars vs 43 chars)
- ✓ **Low Memory** - 91% less memory allocated
- ✓ **Fast Lookups** - Smaller keys improve cache efficiency
- ⚠️ **Not Cryptographically Secure** - 64-bit non-cryptographic hash
- ⚠️ **Collision Probability** - Negligible for cache keys (~2^32 operations)

**Suitable For:** Cache key generation (internal, not user-exposed), high-throughput scenarios, in-memory caches with short TTL.

**Not Suitable For:** Cryptographic signatures, password hashing, security tokens, data integrity in adversarial environments.

#### Hash Algorithm Comparison

| Aspect | SHA-256 | xxHash |
|--------|---------|--------|
| **Speed** | 149 ns/op | 54 ns/op (2.7x faster) |
| **Output Size** | 43 chars | 12 chars (72% smaller) |
| **Memory** | 215 B/op | 18 B/op (91% less) |
| **Cryptographic Security** | ✅ Yes | ❌ No |
| **Cache Key Suitability** | ✅ Yes | ✅ Yes |
| **Password Hashing** | ⚠️ Not recommended | ❌ Never |
| **Backward Compatible** | ✅ Default | ⚠️ Opt-in |

**Recommendation:**

- Use **SHA-256** (default) for distributed caches and security-sensitive scenarios
- Use **xxHash** for high-throughput, performance-critical applications where cache keys are internal

⚠️ **Warning:** Changing hash algorithms invalidates existing cache entries. Plan cache warming strategy.

See [Advanced Features - Hash Algorithm Selection](./advanced-features.md#hash-algorithm-selection) for detailed configuration.

### Optional AES-256-GCM Encryption

Enable encryption of cached data with two modes available:

#### Fixed Salt Encryption (Default)

```go
import "github.com/sandrolain/httpcache"

cache := diskcache.New("/tmp/cache")
transport := httpcache.NewTransport(cache,
    httpcache.WithEncryption("your-secret-passphrase"),
)
client := transport.Client()
```

**Fixed Salt Characteristics:**

- ✓ **AES-256-GCM Encryption** - Industry-standard authenticated encryption
- ✓ **scrypt Key Derivation** - Strong passphrase protection (N=32768, r=8, p=1)
- ✓ **Fixed Salt** - Same salt for all values (faster, backward compatible)
- ✓ **Unique Nonce per Value** - Prevents IV reuse attacks
- ✓ **Authentication Tag** - Prevents tampering with cached data
- ✓ **Faster Performance** - Single key derivation at initialization
- ✓ **Smaller Overhead** - Only 1 byte format indicator

**Best For:** Existing deployments, performance-critical applications, high-throughput caching.

#### Random Salt Encryption (Enhanced Security)

```go
import "github.com/sandrolain/httpcache"

cache := diskcache.New("/tmp/cache")
transport := httpcache.NewTransport(cache,
    httpcache.WithRandomSaltEncryption("your-secret-passphrase"),
)
client := transport.Client()
```

**Random Salt Characteristics:**

- ✓ **Unique 32-byte Salt per Value** - Maximum security against rainbow table attacks
- ✓ **Per-Value Key Derivation** - Independent encryption keys for each cache entry
- ✓ **Pattern Protection** - Identical plaintexts produce different ciphertexts
- ✓ **NIST SP 800-132 Compliant** - Follows government security standards
- ✓ **OWASP Recommended** - Industry best practices for encryption
- ✓ **Format Versioning** - Future-proof design for algorithm upgrades
- ✓ **Backward Compatible Decryption** - Can read fixed salt encrypted data
- ⚠️ **Slower Performance** - Key derivation per encryption (~500ms)
- ⚠️ **Larger Overhead** - 33 bytes per value (1 version + 32 salt)

**Best For:** New deployments, security-critical applications (financial, healthcare, PII), compliance requirements (SOC 2, HIPAA, PCI DSS), long-term storage.

#### Security Comparison

| Security Aspect | Fixed Salt | Random Salt |
|----------------|------------|-------------|
| **Rainbow Table Protection** | ❌ Vulnerable | ✅ Protected |
| **Pattern Analysis** | ❌ Reveals patterns | ✅ No patterns |
| **Key Compromise Impact** | ❌ All data exposed | ✅ Single value only |
| **NIST/OWASP Compliance** | ❌ Non-compliant | ✅ Compliant |
| **Performance** | ✅ Fast (~1µs) | ⚠️ Slower (~500ms) |
| **Storage Overhead** | ✅ +1 byte | ⚠️ +33 bytes |

**Recommendation:** Use `WithRandomSaltEncryption()` for new deployments and security-critical applications. Use `WithEncryption()` for backward compatibility and performance-focused scenarios.

See the [Encryption Security Example](../examples/encryption-security/) for detailed comparison, migration strategies, and compliance information.

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
