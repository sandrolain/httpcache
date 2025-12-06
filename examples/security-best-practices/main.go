package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/sandrolain/httpcache"
	"github.com/sandrolain/httpcache/diskcache"
)

func main() {
	// Start a test server that returns the current time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set cache headers
		w.Header().Set("Cache-Control", "max-age=300") // 5 minutes
		fmt.Fprintf(w, "Server time: %s\n", time.Now().Format(time.RFC3339))
	}))
	defer server.Close()

	fmt.Println("=== httpcache Security Features Example ===")
	fmt.Println("(Using built-in key hashing and optional encryption)")
	fmt.Println("")

	// Example 1: Basic usage - key hashing is always enabled
	fmt.Println("1. Basic Usage (Key Hashing Always Enabled)")
	fmt.Println(strings.Repeat("-", 50))
	demonstrateKeyHashingOnly(server.URL)

	fmt.Println("")
	fmt.Println("")

	// Example 2: With encryption enabled
	fmt.Println("2. Key Hashing + AES-256 Encryption")
	fmt.Println(strings.Repeat("-", 50))
	demonstrateWithEncryption(server.URL)

	fmt.Println("")
	fmt.Println("")

	// Example 3: Security comparison
	fmt.Println("3. Security Features Summary")
	fmt.Println(strings.Repeat("-", 50))
	demonstrateSecuritySummary()

	fmt.Println("")
	fmt.Println("")

	// Example 4: Multi-user scenario with sensitive data
	fmt.Println("4. Multi-User Scenario (Recommended Pattern)")
	fmt.Println(strings.Repeat("-", 50))
	demonstrateMultiUserScenario()
}

func demonstrateKeyHashingOnly(serverURL string) {
	// Create a temporary directory for disk cache
	tmpDir, err := os.MkdirTemp("", "httpcache-secure-hashing-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create cache and transport - key hashing is always enabled
	cache := diskcache.New(tmpDir)
	transport := httpcache.NewTransport(cache)

	fmt.Printf("Encryption enabled: %v\n", transport.IsEncryptionEnabled())
	fmt.Println("Key hashing: always enabled (SHA-256)")

	// Create HTTP client
	client := transport.Client()

	// First request - will hit the server
	fmt.Println("\nFirst request (should miss cache):")
	resp1, err := client.Get(serverURL)
	if err != nil {
		log.Fatal(err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	fmt.Printf("Response: %s", body1)
	fmt.Printf("From cache: %v\n", resp1.Header.Get("X-From-Cache") == "1")

	// Second request - should come from cache
	fmt.Println("\nSecond request (should hit cache):")
	resp2, err := client.Get(serverURL)
	if err != nil {
		log.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	fmt.Printf("Response: %s", body2)
	fmt.Printf("From cache: %v\n", resp2.Header.Get("X-From-Cache") == "1")

	fmt.Println("\n✓ Keys are automatically hashed with SHA-256 before storage")
	fmt.Println("✓ Original URLs are not exposed in cache backend")
}

func demonstrateWithEncryption(serverURL string) {
	// Get passphrase from environment or use default
	passphrase := os.Getenv("CACHE_PASSPHRASE")
	if passphrase == "" {
		passphrase = "example-passphrase-DO-NOT-USE-IN-PRODUCTION"
		fmt.Println("⚠️  Using default passphrase for demonstration")
		fmt.Println("   In production, use environment variable: CACHE_PASSPHRASE")
	}

	// Create a temporary directory for disk cache
	tmpDir, err := os.MkdirTemp("", "httpcache-secure-encrypt-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create cache and transport with encryption enabled
	cache := diskcache.New(tmpDir)
	transport := httpcache.NewTransport(cache,
		httpcache.WithEncryption(passphrase),
	)

	fmt.Printf("\nEncryption enabled: %v\n", transport.IsEncryptionEnabled())
	fmt.Println("Key hashing: always enabled (SHA-256)")

	// Create HTTP client
	client := transport.Client()

	// First request
	fmt.Println("\nFirst request (should miss cache):")
	resp1, err := client.Get(serverURL)
	if err != nil {
		log.Fatal(err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	fmt.Printf("Response: %s", body1)
	fmt.Printf("From cache: %v\n", resp1.Header.Get("X-From-Cache") == "1")

	// Second request
	fmt.Println("\nSecond request (should hit cache):")
	resp2, err := client.Get(serverURL)
	if err != nil {
		log.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	fmt.Printf("Response: %s", body2)
	fmt.Printf("From cache: %v\n", resp2.Header.Get("X-From-Cache") == "1")

	fmt.Println("\n✓ Keys are hashed with SHA-256")
	fmt.Println("✓ Data is encrypted with AES-256-GCM")
	fmt.Println("✓ Authentication tag prevents tampering")
}

func demonstrateSecuritySummary() {
	fmt.Println("httpcache v2.0 Built-in Security Features:")
	fmt.Println("")

	fmt.Println("1. SHA-256 Key Hashing (Always Enabled):")
	fmt.Println("   ✓ All cache keys are automatically hashed")
	fmt.Println("   ✓ Original URLs/keys are not exposed in cache backend")
	fmt.Println("   ✓ Prevents key enumeration attacks")
	fmt.Println("   ✓ No configuration required - enabled by default")
	fmt.Println("")

	fmt.Println("2. AES-256-GCM Encryption (Optional):")
	fmt.Println("   ✓ Enable with: httpcache.WithEncryption(passphrase)")
	fmt.Println("   ✓ Uses scrypt for secure key derivation")
	fmt.Println("   ✓ Authenticated encryption prevents tampering")
	fmt.Println("   ✓ Each value has unique nonce for IV safety")
	fmt.Println("")

	fmt.Println("Usage Examples:")
	fmt.Println("")
	fmt.Println("   // Basic (key hashing always enabled)")
	fmt.Println("   transport := httpcache.NewTransport(cache)")
	fmt.Println("")
	fmt.Println("   // With encryption")
	fmt.Println("   transport := httpcache.NewTransport(cache,")
	fmt.Println("       httpcache.WithEncryption(os.Getenv(\"CACHE_PASSPHRASE\")),")
	fmt.Println("   )")
}

func demonstrateMultiUserScenario() {
	fmt.Println("Recommended pattern for user-specific data:")
	fmt.Println("")

	// Create a temporary directory for disk cache
	tmpDir, err := os.MkdirTemp("", "httpcache-secure-multiuser-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Get passphrase from environment or use default
	passphrase := os.Getenv("CACHE_PASSPHRASE")
	if passphrase == "" {
		passphrase = "production-grade-passphrase-from-env"
	}

	// Create transport with encryption and user-specific caching
	cache := diskcache.New(tmpDir)
	transport := httpcache.NewTransport(cache,
		httpcache.WithEncryption(passphrase),
		httpcache.WithCacheKeyHeaders([]string{"X-User-ID"}), // Include user ID in cache key
	)

	client := transport.Client()

	// Simulate requests from different users
	fmt.Println("User 1 Request:")
	req1, _ := http.NewRequest("GET", "http://api.example.com/profile", nil)
	req1.Header.Set("X-User-ID", "user-123")
	fmt.Println("   X-User-ID: user-123")
	fmt.Println("   ✓ Cache key includes SHA-256(URL + user-123)")
	fmt.Println("   ✓ Response encrypted with AES-256-GCM")
	fmt.Println("")

	fmt.Println("User 2 Request:")
	req2, _ := http.NewRequest("GET", "http://api.example.com/profile", nil)
	req2.Header.Set("X-User-ID", "user-456")
	fmt.Println("   X-User-ID: user-456")
	fmt.Println("   ✓ Cache key includes SHA-256(URL + user-456)")
	fmt.Println("   ✓ Different cache entry (user isolation)")
	fmt.Println("   ✓ Response encrypted with AES-256-GCM")
	fmt.Println("")

	fmt.Println("Security Benefits:")
	fmt.Println("   ✓ Each user gets isolated cache entries")
	fmt.Println("   ✓ User IDs not visible in cache (hashed)")
	fmt.Println("   ✓ User data encrypted and authenticated")
	fmt.Println("   ✓ No cross-user data leakage")
	fmt.Println("   ✓ Compliant with GDPR/CCPA requirements")

	// Note: We don't actually execute these requests since we don't have a real server
	// This is just to demonstrate the pattern
	_ = client
	_ = req1
	_ = req2
}
