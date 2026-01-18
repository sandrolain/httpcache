package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/diskcache"
)

func main() {
	// Create a disk cache in a temporary directory
	diskCache := diskcache.New("/tmp/httpcache-encryption-example")

	fmt.Println("=== HTTP Cache Encryption Security Example ===")
	fmt.Println()

	// Example 1: Default encryption (fixed salt - backward compatible)
	fmt.Println("1. Using WithEncryption() - Fixed Salt (Default)")
	fmt.Println("   - Backward compatible with existing encrypted data")
	fmt.Println("   - Faster encryption/decryption")
	fmt.Println("   - Smaller encrypted data size")
	fmt.Println("   - Suitable for high-performance scenarios")
	fmt.Println()

	transportFixed := httpcache.NewTransport(
		diskCache,
		httpcache.WithEncryption("my-secret-passphrase-fixed"),
	)
	clientFixed := &http.Client{
		Transport: transportFixed,
		Timeout:   10 * time.Second,
	}

	resp1, err := clientFixed.Get("https://httpbin.org/cache/60")
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp1.Body.Close()
	io.Copy(io.Discard, resp1.Body)

	fmt.Printf("   Response: %s\n", resp1.Status)
	fmt.Printf("   From cache: %s\n", resp1.Header.Get("X-From-Cache"))
	fmt.Printf("   Encryption enabled: %v\n\n", transportFixed.IsEncryptionEnabled())

	// Example 2: Enhanced encryption (random salt per value)
	fmt.Println("2. Using WithRandomSaltEncryption() - Random Salt (Enhanced Security)")
	fmt.Println("   - Unique salt for each encrypted value")
	fmt.Println("   - Protection against rainbow table attacks")
	fmt.Println("   - Compliance with NIST SP 800-132 and OWASP guidelines")
	fmt.Println("   - Recommended for new deployments and security-critical applications")
	fmt.Println()

	// Create a new disk cache for random salt example
	diskCacheRandom := diskcache.New("/tmp/httpcache-encryption-random-example")

	transportRandom := httpcache.NewTransport(
		diskCacheRandom,
		httpcache.WithRandomSaltEncryption("my-secret-passphrase-random"),
	)
	clientRandom := &http.Client{
		Transport: transportRandom,
		Timeout:   10 * time.Second,
	}

	resp2, err := clientRandom.Get("https://httpbin.org/cache/60")
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp2.Body.Close()
	io.Copy(io.Discard, resp2.Body)

	fmt.Printf("   Response: %s\n", resp2.Status)
	fmt.Printf("   From cache: %s\n", resp2.Header.Get("X-From-Cache"))
	fmt.Printf("   Encryption enabled: %v\n\n", transportRandom.IsEncryptionEnabled())

	// Example 3: Security comparison
	fmt.Println("3. Security Comparison")
	fmt.Println()
	fmt.Println("   Fixed Salt (WithEncryption):")
	fmt.Println("   ❌ Vulnerable to rainbow table attacks")
	fmt.Println("   ❌ Pattern analysis reveals identical plaintexts")
	fmt.Println("   ❌ Single compromised key exposes all data")
	fmt.Println("   ✅ Faster encryption/decryption")
	fmt.Println("   ✅ Smaller encrypted data (+1 byte overhead)")
	fmt.Println()

	fmt.Println("   Random Salt (WithRandomSaltEncryption):")
	fmt.Println("   ✅ Rainbow table attacks infeasible")
	fmt.Println("   ✅ Identical plaintexts produce different ciphertexts")
	fmt.Println("   ✅ Compromised key only exposes single value")
	fmt.Println("   ✅ NIST/OWASP compliant")
	fmt.Println("   ⚠️  Slower encryption/decryption (per-value key derivation)")
	fmt.Println("   ⚠️  Larger encrypted data (+33 bytes overhead)")
	fmt.Println()

	// Example 4: Migration scenarios
	fmt.Println("4. Migration Scenarios")
	fmt.Println()
	fmt.Println("   New Deployment:")
	fmt.Println("   → Use WithRandomSaltEncryption() from the start")
	fmt.Println("   → No migration needed")
	fmt.Println()

	fmt.Println("   Existing Deployment (Backward Compatibility):")
	fmt.Println("   → Continue using WithEncryption()")
	fmt.Println("   → No changes required")
	fmt.Println()

	fmt.Println("   Migrate to Enhanced Security:")
	fmt.Println("   → Clear existing cache")
	fmt.Println("   → Switch to WithRandomSaltEncryption()")
	fmt.Println("   → Cache will repopulate with new encrypted entries")
	fmt.Println()

	// Example 5: Use case recommendations
	fmt.Println("5. When to Use Each Mode")
	fmt.Println()
	fmt.Println("   Use WithEncryption() (Fixed Salt) for:")
	fmt.Println("   • Existing deployments (backward compatibility)")
	fmt.Println("   • Performance-critical applications")
	fmt.Println("   • High-throughput caching")
	fmt.Println("   • Short-lived cache entries (seconds/minutes TTL)")
	fmt.Println()

	fmt.Println("   Use WithRandomSaltEncryption() (Random Salt) for:")
	fmt.Println("   • New deployments")
	fmt.Println("   • Security-critical applications (financial, healthcare, PII)")
	fmt.Println("   • Compliance requirements (SOC 2, HIPAA, PCI DSS)")
	fmt.Println("   • Long-term cache storage")
	fmt.Println("   • Multi-tenant environments")
	fmt.Println()

	fmt.Println("=== Example Complete ===")
}
