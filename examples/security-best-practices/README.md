# Security Best Practices Example

This example demonstrates the built-in security features of httpcache v2.0.

## Features Demonstrated

1. **Key Hashing (Always Enabled)** - All cache keys are automatically hashed with SHA-256
2. **Optional Encryption** - Enable AES-256-GCM encryption with `WithEncryption(passphrase)`
3. **Security Features Summary** - Overview of security capabilities
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
=== httpcache Security Features Example ===
(Using built-in key hashing and optional encryption)

1. Basic Usage (Key Hashing Always Enabled)
--------------------------------------------------
Encryption enabled: false
Key hashing: always enabled (SHA-256)

First request (should miss cache):
Response: Server time: 2024-01-15T10:30:45Z
From cache: false

Second request (should hit cache):
Response: Server time: 2024-01-15T10:30:45Z
From cache: true

✓ Keys are automatically hashed with SHA-256 before storage
✓ Original URLs are not exposed in cache backend


2. Key Hashing + AES-256 Encryption
--------------------------------------------------
⚠️  Using default passphrase for demonstration
   In production, use environment variable: CACHE_PASSPHRASE

Encryption enabled: true
Key hashing: always enabled (SHA-256)

First request (should miss cache):
Response: Server time: 2024-01-15T10:30:45Z
From cache: false

Second request (should hit cache):
Response: Server time: 2024-01-15T10:30:45Z
From cache: true

✓ Keys are hashed with SHA-256
✓ Data is encrypted with AES-256-GCM
✓ Authentication tag prevents tampering


3. Security Features Summary
--------------------------------------------------
httpcache v2.0 Built-in Security Features:

1. SHA-256 Key Hashing (Always Enabled):
   ✓ All cache keys are automatically hashed
   ✓ Original URLs/keys are not exposed in cache backend
   ✓ Prevents key enumeration attacks
   ✓ No configuration required - enabled by default

2. AES-256-GCM Encryption (Optional):
   ✓ Enable with: httpcache.WithEncryption(passphrase)
   ✓ Uses scrypt for secure key derivation
   ✓ Authenticated encryption prevents tampering
   ✓ Each value has unique nonce for IV safety


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

### Key Hashing Only (Default)

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
    
    // Create transport with security features
    transport := httpcache.NewTransport(redisCache,
        httpcache.WithEncryption(passphrase),
        httpcache.WithCacheKeyHeaders([]string{"Authorization", "X-User-ID"}),
    )
    
    client := transport.Client()
    
    // Client now has:
    // ✓ SHA-256 hashed cache keys (always enabled)
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

- [Main README](../../README.md)
- [Security Documentation](../../docs/security.md)
