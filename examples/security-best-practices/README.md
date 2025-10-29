# Security Best Practices Example

This example demonstrates the `securecache` wrapper for adding security to any cache backend.

## Features Demonstrated

1. **Key Hashing Only** - Basic privacy protection
2. **Key Hashing + Encryption** - Full data protection
3. **Security Comparison** - Visual comparison of different approaches
4. **Multi-User Scenario** - Recommended pattern for user-specific data

## Running the Example

```bash
# Basic usage
go run main.go

# With custom passphrase
CACHE_PASSPHRASE="your-secret-passphrase" go run main.go
```

## Expected Output

```
=== SecureCache Example ===

1. Key Hashing Only (No Encryption)
--------------------------------------------------
Encryption enabled: false

First request (should miss cache):
Response: Server time: 2024-01-15T10:30:45Z
From cache: false

Second request (should hit cache):
Response: Server time: 2024-01-15T10:30:45Z
From cache: true

✓ Keys are hashed with SHA-256 before storage
✓ Original URLs are not exposed in cache backend


2. Key Hashing + AES-256 Encryption
--------------------------------------------------
⚠️  Using default passphrase for demonstration
   In production, use environment variable: CACHE_PASSPHRASE

Encryption enabled: true

First request (should miss cache):
Response: Server time: 2024-01-15T10:30:45Z
From cache: false

Second request (should hit cache):
Response: Server time: 2024-01-15T10:30:45Z
From cache: true

✓ Keys are hashed with SHA-256
✓ Data is encrypted with AES-256-GCM
✓ Authentication tag prevents tampering


3. Security Comparison
--------------------------------------------------
Security Comparison:

1. Normal Cache (No Security):
   Key stored as: http://example.com/api/user/123
   ❌ Original URL visible in cache
   ❌ Response data readable
   ❌ Sensitive data (SSN) exposed

2. SecureCache (Key Hashing Only):
   Key stored as: SHA-256 hash (64 hex characters)
   ✓ Original URL hidden
   ❌ Response data readable
   ⚠️  Sensitive data (SSN) exposed

3. SecureCache (Hashing + Encryption):
   Key stored as: SHA-256 hash (64 hex characters)
   ✓ Original URL hidden
   ✓ Response data encrypted
   ✓ Sensitive data (SSN) protected
   ✓ Authentication tag prevents tampering


4. Multi-User Scenario (Recommended Pattern)
--------------------------------------------------
Recommended pattern for user-specific data:

User 1 Request:
   X-User-ID: user-123
   ✓ Cache key includes SHA-256(URL + user-123)
   ✓ Response encrypted with AES-256-GCM

User 2 Request:
   X-User-ID: user-456
   ✓ Cache key includes SHA-256(URL + user-456)
   ✓ Different cache entry (user isolation)
   ✓ Response encrypted with AES-256-GCM

Security Benefits:
   ✓ Each user gets isolated cache entries
   ✓ User IDs not visible in cache (hashed)
   ✓ User data encrypted and authenticated
   ✓ No cross-user data leakage
   ✓ Compliant with GDPR/CCPA requirements
```

## When to Use Each Approach

### Key Hashing Only

Use when:

- Caching public API responses
- Performance is critical (minimal overhead)
- URL privacy is the main concern
- Data is not sensitive

### Key Hashing + Encryption

Use when:

- Caching user-specific data
- Storing authentication tokens
- Handling PII (Personally Identifiable Information)
- GDPR/CCPA compliance required
- HIPAA-regulated healthcare data
- PCI DSS credit card information

## Production Configuration

### Environment Variables

```bash
# Required for encryption
export CACHE_PASSPHRASE="your-very-strong-random-passphrase-at-least-32-chars"

# Optional: Backend-specific configuration
export REDIS_URL="redis://localhost:6379"
export POSTGRES_URL="postgres://user:pass@localhost/dbname"
```

### Passphrase Best Practices

1. **Generation**: Use a cryptographically secure random generator

   ```bash
   # Generate a strong passphrase
   openssl rand -base64 32
   ```

2. **Storage**: Use a secret management system
   - AWS Secrets Manager
   - HashiCorp Vault
   - Kubernetes Secrets
   - Environment variables (minimum)

3. **Rotation**: Plan for passphrase rotation
   - Changing passphrase invalidates all cached data
   - Consider blue/green deployment with cache warming

## Real-World Example

```go
package main

import (
    "log"
    "os"
    
    "github.com/sandrolain/httpcache"
    "github.com/sandrolain/httpcache/redis"
    "github.com/sandrolain/httpcache/wrapper/securecache"
)

func main() {
    // Get passphrase from secure storage
    passphrase := os.Getenv("CACHE_PASSPHRASE")
    if passphrase == "" {
        log.Fatal("CACHE_PASSPHRASE environment variable required")
    }
    
    // Connect to Redis
    redisClient := redis.MustNewClient("localhost:6379", "", 0)
    redisCache := redis.NewWithClient(redisClient)
    
    // Wrap with security
    cache, err := securecache.New(securecache.Config{
        Cache:      redisCache,
        Passphrase: passphrase,
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Use with transport
    transport := httpcache.NewTransport(cache)
    transport.CacheKeyHeaders = []string{"Authorization", "X-User-ID"}
    
    client := transport.Client()
    
    // Client now has:
    // ✓ SHA-256 hashed cache keys
    // ✓ AES-256-GCM encrypted data
    // ✓ User-specific caching
    // ✓ Authentication tag verification
}
```

## Performance Considerations

Encryption adds overhead (~1-2ms per operation):

| Operation | No Encryption | With Encryption | Overhead |
|-----------|--------------|-----------------|----------|
| Cache Set | ~100ns       | ~1.2ms          | +1.1ms   |
| Cache Get | ~80ns        | ~1.0ms          | +0.9ms   |
| Delete    | ~50ns        | ~50ns           | none     |

For most applications, this overhead is negligible compared to network latency.

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

## See Also

- [securecache package documentation](../../wrapper/securecache/README.md)
- [Main README](../../README.md)
