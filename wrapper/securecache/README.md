# Secure Cache Wrapper

Package `securecache` provides a security wrapper for any `httpcache.Cache` implementation, adding:

- **SHA-256 Key Hashing** (always enabled) - Cache keys are hashed before storage to prevent key enumeration
- **AES-256-GCM Encryption** (optional) - Cached data is encrypted when a passphrase is provided

## Features

- ✅ **Key Privacy**: All cache keys are hashed with SHA-256 before storage
- ✅ **Data Encryption**: Optional AES-256-GCM encryption for cached responses
- ✅ **Authenticated Encryption**: GCM mode provides both confidentiality and authenticity
- ✅ **Key Derivation**: Uses scrypt for strong key derivation from passphrase
- ✅ **Transparent**: Works with any `httpcache.Cache` implementation
- ✅ **Zero Dependencies**: Uses only Go standard library and `golang.org/x/crypto`

## Installation

```bash
go get github.com/sandrolain/httpcache/wrapper/securecache
```

## Usage

### Basic Usage (Key Hashing Only)

```go
import (
    "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/wrapper/securecache"
)

// Wrap any cache backend with key hashing
cache := diskcache.New("/tmp/cache")
secureCache, err := securecache.New(securecache.Config{
    Cache: cache,
    // No passphrase = only key hashing, no encryption
})
if err != nil {
    panic(err)
}

transport := httpcache.NewTransport(secureCache)
client := transport.Client()
```

### With Encryption (Key Hashing + AES-256)

```go
import (
    "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/redis"
    "github.com/sandrolain/httpcache/wrapper/securecache"
)

// Wrap Redis cache with encryption
redisCache := redis.NewWithClient(redisConn)
secureCache, err := securecache.New(securecache.Config{
    Cache: redisCache,
    Passphrase: "your-secret-passphrase-keep-it-safe",
})
if err != nil {
    panic(err)
}

transport := httpcache.NewTransport(secureCache)
client := transport.Client()
```

### Checking Encryption Status

```go
if secureCache.IsEncrypted() {
    fmt.Println("Cache is using encryption")
} else {
    fmt.Println("Cache is using key hashing only")
}
```

## Security Considerations

### Key Hashing (Always Enabled)

All cache keys are hashed using SHA-256 before being passed to the underlying cache. This provides:

- **Privacy**: Original URLs/keys are not exposed in the cache backend
- **Consistency**: Same input always produces the same hash
- **Security**: Prevents enumeration of cached URLs

### Data Encryption (Optional)

When a passphrase is provided:

1. **Key Derivation**: The passphrase is processed through scrypt with strong parameters:
   - N=32768 (CPU/memory cost)
   - r=8 (block size)
   - p=1 (parallelization)
   - 32-byte output (256-bit key for AES-256)

2. **Encryption**: Data is encrypted using AES-256 in GCM mode:
   - Provides both confidentiality and authenticity
   - Random nonce for each encryption operation
   - Authentication tag prevents tampering

3. **Storage Format**: `[12-byte nonce][encrypted data + 16-byte auth tag]`

### Best Practices

1. **Passphrase Management**:
   - Use a strong, random passphrase (at least 32 characters)
   - Store passphrase securely (environment variable, secret manager)
   - Never commit passphrases to version control
   - Use the same passphrase across application restarts

2. **Key Rotation**:
   - Changing the passphrase invalidates all existing cached data
   - Plan for cache invalidation when rotating passphrases
   - Consider a migration strategy if needed

3. **Performance**:
   - Encryption adds ~1-2ms overhead per operation
   - scrypt key derivation happens once at initialization
   - Consider the trade-off between security and performance

4. **Compliance**:
   - Use encryption for sensitive data (PII, tokens, etc.)
   - Key hashing alone may be sufficient for public data
   - Consult your security team for compliance requirements

## Use Cases

### When to Use Key Hashing Only

- Public API responses
- Non-sensitive data
- Performance-critical applications
- Basic privacy protection

### When to Add Encryption

- User-specific data (with `CacheKeyHeaders`)
- Authentication tokens in responses
- PII (Personally Identifiable Information)
- GDPR/CCPA compliance requirements
- HIPAA-regulated healthcare data
- PCI DSS credit card information

## Examples

See [`examples/security-best-practices/`](../../examples/security-best-practices/) for:

- Complete working example
- Multi-user caching with encryption
- Passphrase management
- Performance comparisons

## Performance

Benchmarks on Apple M2 (results may vary):

| Operation | Without Encryption | With Encryption | Overhead |
|-----------|-------------------|-----------------|----------|
| Set       | ~100ns            | ~1.2ms          | +1.1ms   |
| Get       | ~80ns             | ~1.0ms          | +0.9ms   |
| Delete    | ~50ns             | ~50ns           | none     |

Encryption overhead is primarily from:

- Random nonce generation (~50μs)
- AES-256-GCM encryption/decryption (~900μs for typical HTTP response)

## Limitations

- Changing the passphrase invalidates all cached data
- Encrypted data is ~12 bytes larger (nonce) + 16 bytes (auth tag)
- No built-in key rotation mechanism
- All instances must use the same passphrase to share cache

## License

See the main [LICENSE.txt](../../LICENSE.txt) file in the repository root.
