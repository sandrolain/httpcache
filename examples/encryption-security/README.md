# Encryption Security Example

This example demonstrates the enhanced encryption security features in httpcache, showing the difference between fixed salt and random salt encryption modes.

## Overview

The httpcache package provides two encryption modes:

1. **Fixed Salt Encryption** (`WithEncryption()`): Backward compatible, faster, suitable for high-performance scenarios
2. **Random Salt Encryption** (`WithRandomSaltEncryption()`): Enhanced security, unique salt per value, NIST/OWASP compliant

## Running the Example

```bash
cd examples/encryption-security
go run main.go
```

## What This Example Demonstrates

### 1. Fixed Salt Encryption (Default)

```go
transport := httpcache.NewTransport(
    cache,
    httpcache.WithEncryption("my-secret-passphrase"),
)
```

**Characteristics:**

- Uses a fixed salt derived from a constant string
- Same key for all encrypted values
- Faster encryption/decryption
- Smaller encrypted data size (+1 byte overhead)
- Backward compatible with existing deployments

**Security Considerations:**

- ❌ Vulnerable to rainbow table attacks
- ❌ Pattern analysis can reveal identical plaintexts
- ❌ Single compromised key exposes all data

**Best For:**

- Existing deployments requiring backward compatibility
- Performance-critical applications
- High-throughput caching scenarios
- Short-lived cache entries

### 2. Random Salt Encryption (Enhanced)

```go
transport := httpcache.NewTransport(
    cache,
    httpcache.WithRandomSaltEncryption("my-secret-passphrase"),
)
```

**Characteristics:**

- Generates a unique 32-byte random salt for each encrypted value
- Independent key derivation per value using scrypt
- Format versioning for future-proofing
- Backward compatible decryption (can read fixed salt data)
- **✅ Persistent across restarts** - Salt is stored with encrypted data, cache remains readable after service restart with same passphrase

**Security Benefits:**

- ✅ Rainbow table attacks infeasible
- ✅ Identical plaintexts produce different ciphertexts
- ✅ Compromised key only exposes single value
- ✅ Compliant with NIST SP 800-132 and OWASP guidelines
- ✅ No key management needed - passphrase is all you need

**Trade-offs:**

- Slower encryption/decryption (~500ms per operation for key derivation)
- Larger encrypted data size (+33 bytes overhead: 1 version + 32 salt)

**Best For:**

- New deployments
- Security-critical applications (financial, healthcare, PII)
- Compliance requirements (SOC 2, HIPAA, PCI DSS)
- Long-term cache storage
- Multi-tenant environments

## Security Comparison

| Feature | Fixed Salt | Random Salt |
|---------|-----------|-------------|
| **Rainbow Table Protection** | ❌ Vulnerable | ✅ Protected |
| **Pattern Analysis** | ❌ Reveals patterns | ✅ No patterns |
| **Key Compromise Impact** | ❌ All data exposed | ✅ Single value only |
| **Encryption Speed** | ✅ Fast (~1µs) | ⚠️ Slower (~500ms) |
| **Data Size Overhead** | ✅ +1 byte | ⚠️ +33 bytes |
| **NIST/OWASP Compliance** | ❌ Non-compliant | ✅ Compliant |
| **Backward Compatibility** | ✅ Full | ⚠️ New format only |

## Migration Scenarios

### New Deployment

Start with random salt encryption for best security:

```go
transport := httpcache.NewTransport(
    cache,
    httpcache.WithRandomSaltEncryption("secure-passphrase"),
)
```

### Existing Deployment (No Change)

Continue using fixed salt encryption:

```go
transport := httpcache.NewTransport(
    cache,
    httpcache.WithEncryption("existing-passphrase"),
)
```

### Migrate to Enhanced Security

```go
// Step 1: Clear existing cache (encrypted data won't decrypt with random salt)
cache.Clear()

// Step 2: Switch to random salt encryption
transport := httpcache.NewTransport(
    cache,
    httpcache.WithRandomSaltEncryption("existing-passphrase"),
)

// Cache will naturally repopulate with new random-salt-encrypted entries
```

## Technical Details

### Encryption Format

**Fixed Salt (Legacy):**

```
[12 bytes nonce][N bytes ciphertext]
```

**Random Salt (Version 1):**

```
[1 byte version=0x01][32 bytes salt][12 bytes nonce][N bytes ciphertext]
```

### Key Derivation

Both modes use scrypt with:

- N=32768 (CPU/memory cost factor)
- R=8 (block size)
- P=1 (parallelization factor)
- Key length=32 bytes (AES-256)

**Fixed Salt:**

```go
key = scrypt(passphrase, fixed_salt, N, R, P, 32)
// Key derived once at initialization
```

**Random Salt:**

```go
// ENCRYPTION (stores salt with data)
salt = random(32)  // Per encryption operation
key = scrypt(passphrase, salt, N, R, P, 32)
encrypted = [version][salt][nonce][ciphertext]
// Salt stored IN the encrypted data

// DECRYPTION (extracts salt from data)
salt = encrypted[1:33]  // Extract salt from encrypted data
key = scrypt(passphrase, salt, N, R, P, 32)  // Re-derive key
plaintext = decrypt(key, encrypted)
// ✅ Works after restart - salt is in the data, passphrase from config
```

**Important:** The random salt is **stored with the encrypted data**, not in memory. This means:

- ✅ Cache remains readable after service restart
- ✅ Only need to provide the same passphrase
- ✅ No key management required
- ✅ Works with persistent backends (disk, Redis, PostgreSQL, etc.)

### Automatic Format Detection

The decryption function automatically detects format:

```go
if data[0] == 0x01 {
    // Version 1: Extract salt and derive key
    salt := data[1:33]
    key := scrypt(passphrase, salt, ...)
    // Decrypt with derived key
} else {
    // Legacy format: Use pre-derived fixed salt key
    // Decrypt with fixed GCM cipher
}
```

## Performance Considerations

### Encryption Performance

| Operation | Fixed Salt | Random Salt |
|-----------|-----------|-------------|
| Key Derivation | 1x at init (~500ms) | Per operation (~500ms) |
| Encryption | ~1µs | ~500ms + ~1µs |
| Data Overhead | +1 byte | +33 bytes |

### Decryption Performance

| Operation | Fixed Salt | Random Salt |
|-----------|-----------|-------------|
| Format Detection | N/A | ~1ns |
| Legacy Format | ~1µs | ~1µs (uses fixed GCM) |
| New Format | N/A | ~500ms + ~1µs |

### When Performance Matters

For high-throughput applications (>1000 req/s), consider:

1. **Use fixed salt** if security trade-offs are acceptable
2. **Cache long-lived data** to amortize encryption cost
3. **Increase cache TTL** to reduce re-encryption frequency
4. **Profile your application** to measure actual impact

## Compliance

### NIST SP 800-132

Random salt mode complies with:

- ✅ Unique salt per password-based encryption
- ✅ Minimum 128-bit salt length (we use 256-bit)
- ✅ Cryptographically secure random generation

### OWASP Cryptographic Storage

Random salt mode follows:

- ✅ Unique salt per encryption operation
- ✅ Salt stored with encrypted data
- ✅ Proper key derivation function (scrypt)

## Frequently Asked Questions

### Can I decrypt the cache after a service restart?

**Yes!** Both encryption modes preserve cache across restarts:

**Fixed Salt Mode:**

- Same fixed salt is always derived from the passphrase
- Provide the same passphrase → cache is readable

**Random Salt Mode:**

- Salt is stored WITH each encrypted value
- Provide the same passphrase → cache is readable
- The system extracts the salt from data and re-derives the key

**Example - Service Restart:**

```go
// First run - encrypt and cache data
transport1 := httpcache.NewTransport(cache,
    httpcache.WithRandomSaltEncryption("my-passphrase"),
)
client1 := &http.Client{Transport: transport1}
resp, _ := client1.Get("https://example.com/api")
// Data encrypted with random salt and cached

// Service restarts...

// Second run - decrypt cached data
transport2 := httpcache.NewTransport(cache,
    httpcache.WithRandomSaltEncryption("my-passphrase"),  // Same passphrase
)
client2 := &http.Client{Transport: transport2}
resp, _ := client2.Get("https://example.com/api")
// ✅ Data decrypted successfully - salt was stored with data
// Response served from cache
```

### What happens if I change the passphrase?

If you change the passphrase after data is encrypted:

- ❌ **Decryption fails** - key derivation produces different key
- ⚠️ **Cache entries become unreadable** - treated as cache miss
- ✅ **New data encrypted with new passphrase**
- ℹ️ **Old entries eventually expire** per TTL

**Recommendation:** Keep the passphrase consistent or clear the cache when changing it.

### Do I need to store or backup the random salts?

**No!** Salts are automatically stored with the encrypted data in the cache backend.

- The salt is part of the encrypted value format
- No separate salt management needed
- Works with any cache backend (disk, Redis, PostgreSQL, etc.)
- Only the passphrase needs to be configured

## Further Reading

- [NIST SP 800-132: Password-Based Key Derivation](https://nvlpubs.nist.gov/nistpubs/Legacy/SP/nistspecialpublication800-132.pdf)
- [OWASP Cryptographic Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html)
- [RFC 7914: The scrypt Function](https://datatracker.ietf.org/doc/html/rfc7914)
- [NIST SP 800-38D: AES-GCM](https://csrc.nist.gov/publications/detail/sp/800-38d/final)
