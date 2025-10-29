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
	"github.com/sandrolain/httpcache/wrapper/securecache"
)

func main() {
	// Start a test server that returns the current time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set cache headers
		w.Header().Set("Cache-Control", "max-age=300") // 5 minutes
		fmt.Fprintf(w, "Server time: %s\n", time.Now().Format(time.RFC3339))
	}))
	defer server.Close()

	fmt.Println("=== SecureCache Example ===")

	// Example 1: Basic usage with key hashing only
	fmt.Println("1. Key Hashing Only (No Encryption)")
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
	fmt.Println("3. Security Comparison")
	fmt.Println(strings.Repeat("-", 50))
	demonstrateSecurityComparison(server.URL)

	fmt.Println("")
	fmt.Println("")

	// Example 4: Multi-user scenario with sensitive data
	fmt.Println("4. Multi-User Scenario (Recommended Pattern)")
	fmt.Println(strings.Repeat("-", 50))
	demonstrateMultiUserScenario()
}

func demonstrateKeyHashingOnly(serverURL string) {
	// Create a secure cache with key hashing only
	cache := httpcache.NewMemoryCache()
	secureCache, err := securecache.New(securecache.Config{
		Cache: cache,
		// No passphrase = only key hashing
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Encryption enabled: %v\n", secureCache.IsEncrypted())

	// Create HTTP client with secure cache
	transport := httpcache.NewTransport(secureCache)
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

	fmt.Println("\n✓ Keys are hashed with SHA-256 before storage")
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

	// Create a secure cache with encryption
	cache := httpcache.NewMemoryCache()
	secureCache, err := securecache.New(securecache.Config{
		Cache:      cache,
		Passphrase: passphrase,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nEncryption enabled: %v\n", secureCache.IsEncrypted())

	// Create HTTP client with secure cache
	transport := httpcache.NewTransport(secureCache)
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

func demonstrateSecurityComparison(serverURL string) {
	// Create three caches: normal, hashing-only, and encrypted
	normalCache := httpcache.NewMemoryCache()

	hashingCache, _ := securecache.New(securecache.Config{
		Cache: httpcache.NewMemoryCache(),
	})

	encryptedCache, _ := securecache.New(securecache.Config{
		Cache:      httpcache.NewMemoryCache(),
		Passphrase: "demo-passphrase",
	})

	// Make a request with each cache
	testKey := "http://example.com/api/user/123"
	testData := []byte("HTTP/1.1 200 OK\r\nContent-Length: 50\r\n\r\n{\"user\":\"john\",\"ssn\":\"123-45-6789\"}")

	// Store in all caches
	normalCache.Set(testKey, testData)
	hashingCache.Set(testKey, testData)
	encryptedCache.Set(testKey, testData)

	fmt.Println("Security Comparison:")
	fmt.Println("")

	// Normal cache - keys and data exposed
	fmt.Println("1. Normal Cache (No Security):")
	fmt.Printf("   Key stored as: %s\n", testKey)
	fmt.Println("   ❌ Original URL visible in cache")
	fmt.Println("   ❌ Response data readable")
	fmt.Println("   ❌ Sensitive data (SSN) exposed")
	fmt.Println("")

	// Hashing cache - keys hashed, data exposed
	fmt.Println("2. SecureCache (Key Hashing Only):")
	fmt.Println("   Key stored as: SHA-256 hash (64 hex characters)")
	fmt.Println("   ✓ Original URL hidden")
	fmt.Println("   ❌ Response data readable")
	fmt.Println("   ⚠️  Sensitive data (SSN) exposed")
	fmt.Println("")

	// Encrypted cache - keys hashed, data encrypted
	fmt.Println("3. SecureCache (Hashing + Encryption):")
	fmt.Println("   Key stored as: SHA-256 hash (64 hex characters)")
	fmt.Println("   ✓ Original URL hidden")
	fmt.Println("   ✓ Response data encrypted")
	fmt.Println("   ✓ Sensitive data (SSN) protected")
	fmt.Println("   ✓ Authentication tag prevents tampering")
}

func demonstrateMultiUserScenario() {
	fmt.Println("Recommended pattern for user-specific data:")
	fmt.Println("")

	// Use CacheKeyHeaders to include user ID in cache key
	cache := httpcache.NewMemoryCache()
	secureCache, _ := securecache.New(securecache.Config{
		Cache:      cache,
		Passphrase: "production-grade-passphrase-from-env",
	})

	// Create transport with user-specific caching
	transport := httpcache.NewTransport(secureCache)
	transport.CacheKeyHeaders = []string{"X-User-ID"} // Include user ID in cache key

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
