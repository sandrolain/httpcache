// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockCache is a simple in-memory cache for testing
type mockCacheInternal struct {
	data   map[string][]byte
	stales map[string]bool
}

func newMockCacheInternal() *mockCacheInternal {
	return &mockCacheInternal{
		data:   make(map[string][]byte),
		stales: make(map[string]bool),
	}
}

func (m *mockCacheInternal) Get(ctx context.Context, key string) ([]byte, bool, error) {
	data, ok := m.data[key]
	return data, ok, nil
}

func (m *mockCacheInternal) Set(ctx context.Context, key string, data []byte) error {
	m.data[key] = data
	delete(m.stales, key) // Clear stale marker on set
	return nil
}

func (m *mockCacheInternal) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	delete(m.stales, key)
	return nil
}

func (m *mockCacheInternal) MarkStale(ctx context.Context, key string) error {
	if _, exists := m.data[key]; exists {
		m.stales[key] = true
	}
	return nil
}

func (m *mockCacheInternal) IsStale(ctx context.Context, key string) (bool, error) {
	return m.stales[key], nil
}

func (m *mockCacheInternal) GetStale(ctx context.Context, key string) ([]byte, bool, error) {
	if !m.stales[key] {
		return nil, false, nil
	}
	data, ok := m.data[key]
	return data, ok, nil
}

func TestHashKey(t *testing.T) {
	// Test that hashKey produces consistent results
	key := "https://example.com/test"
	hash1 := hashKey(key)
	hash2 := hashKey(key)

	if hash1 != hash2 {
		t.Errorf("hashKey should produce consistent results: %s != %s", hash1, hash2)
	}

	// Test that hashKey produces 64 character hex string (SHA-256)
	if len(hash1) != 64 {
		t.Errorf("hashKey should produce 64 character hex string, got %d", len(hash1))
	}

	// Test that different keys produce different hashes
	key2 := "https://example.com/other"
	hash3 := hashKey(key2)
	if hash1 == hash3 {
		t.Error("hashKey should produce different hashes for different keys")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	passphrase := "test-passphrase-12345"
	gcm, err := initEncryption(passphrase)
	if err != nil {
		t.Fatalf("failed to init encryption: %v", err)
	}

	plaintext := []byte("Hello, World! This is a test message for encryption.")

	// Encrypt
	ciphertext, err := encrypt(gcm, plaintext)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	// Ciphertext should be different from plaintext
	if string(ciphertext) == string(plaintext) {
		t.Error("ciphertext should not equal plaintext")
	}

	// Decrypt
	decrypted, err := decrypt(gcm, ciphertext)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted text should match plaintext: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecryptWithNilGCM(t *testing.T) {
	// Test that encrypt/decrypt with nil GCM returns data unchanged
	data := []byte("test data")

	encrypted, err := encrypt(nil, data)
	if err != nil {
		t.Fatalf("encrypt with nil should not error: %v", err)
	}
	if string(encrypted) != string(data) {
		t.Error("encrypt with nil should return unchanged data")
	}

	decrypted, err := decrypt(nil, data)
	if err != nil {
		t.Fatalf("decrypt with nil should not error: %v", err)
	}
	if string(decrypted) != string(data) {
		t.Error("decrypt with nil should return unchanged data")
	}
}

func TestDecryptWithShortCiphertext(t *testing.T) {
	passphrase := "test-passphrase-12345"
	gcm, err := initEncryption(passphrase)
	if err != nil {
		t.Fatalf("failed to init encryption: %v", err)
	}

	// Try to decrypt data shorter than nonce size
	shortData := []byte("short")
	_, err = decrypt(gcm, shortData)
	if err == nil {
		t.Error("decrypt should fail with short ciphertext")
	}
}

func TestTransportWithEncryption(t *testing.T) {
	cache := newMockCacheInternal()
	transport := NewTransport(cache, WithEncryption("test-passphrase"))

	if !transport.IsEncryptionEnabled() {
		t.Error("encryption should be enabled")
	}
}

func TestTransportWithoutEncryption(t *testing.T) {
	cache := newMockCacheInternal()
	transport := NewTransport(cache)

	if transport.IsEncryptionEnabled() {
		t.Error("encryption should not be enabled by default")
	}
}

func TestWithEncryptionEmptyPassphrase(t *testing.T) {
	cache := newMockCacheInternal()
	transport := &Transport{Cache: cache, MarkCachedResponses: true}

	opt := WithEncryption("")
	err := opt(transport)

	if err == nil {
		t.Error("WithEncryption with empty passphrase should return error")
	}
}

func TestCacheGetSetWithHashing(t *testing.T) {
	cache := newMockCacheInternal()
	transport := NewTransport(cache)

	ctx := context.Background()
	key := "https://example.com/test"
	data := []byte("test data")

	// Set data
	err := transport.cacheSet(ctx, key, data)
	if err != nil {
		t.Fatalf("cacheSet failed: %v", err)
	}

	// The key should be hashed in the underlying cache
	hashedKey := hashKey(key)
	storedData, ok := cache.data[hashedKey]
	if !ok {
		t.Error("data should be stored with hashed key")
	}
	if string(storedData) != string(data) {
		t.Errorf("stored data mismatch: got %q, want %q", storedData, data)
	}

	// Get data
	retrieved, ok, err := transport.cacheGet(ctx, key)
	if err != nil {
		t.Fatalf("cacheGet failed: %v", err)
	}
	if !ok {
		t.Error("data should be found")
	}
	if string(retrieved) != string(data) {
		t.Errorf("retrieved data mismatch: got %q, want %q", retrieved, data)
	}
}

func TestCacheGetSetWithEncryption(t *testing.T) {
	cache := newMockCacheInternal()
	transport := NewTransport(cache, WithEncryption("test-passphrase"))

	ctx := context.Background()
	key := "https://example.com/test"
	data := []byte("test data for encryption")

	// Set data
	err := transport.cacheSet(ctx, key, data)
	if err != nil {
		t.Fatalf("cacheSet failed: %v", err)
	}

	// The stored data should be encrypted
	hashedKey := hashKey(key)
	storedData, ok := cache.data[hashedKey]
	if !ok {
		t.Error("data should be stored")
	}
	if string(storedData) == string(data) {
		t.Error("stored data should be encrypted, not plaintext")
	}

	// Get data - should be decrypted
	retrieved, ok, err := transport.cacheGet(ctx, key)
	if err != nil {
		t.Fatalf("cacheGet failed: %v", err)
	}
	if !ok {
		t.Error("data should be found")
	}
	if string(retrieved) != string(data) {
		t.Errorf("retrieved data mismatch: got %q, want %q", retrieved, data)
	}
}

func TestCacheDelete(t *testing.T) {
	cache := newMockCacheInternal()
	transport := NewTransport(cache)

	ctx := context.Background()
	key := "https://example.com/test"
	data := []byte("test data")

	// Set data
	_ = transport.cacheSet(ctx, key, data)

	// Delete data
	err := transport.cacheDelete(ctx, key)
	if err != nil {
		t.Fatalf("cacheDelete failed: %v", err)
	}

	// Data should be gone
	_, ok, _ := transport.cacheGet(ctx, key)
	if ok {
		t.Error("data should be deleted")
	}
}

func TestTransportOptions(t *testing.T) {
	cache := newMockCacheInternal()

	tests := []struct {
		name    string
		opts    []TransportOption
		check   func(*Transport) bool
		message string
	}{
		{
			name: "WithMarkCachedResponses",
			opts: []TransportOption{WithMarkCachedResponses(false)},
			check: func(tr *Transport) bool {
				return !tr.MarkCachedResponses
			},
			message: "MarkCachedResponses should be false",
		},
		{
			name: "WithSkipServerErrorsFromCache",
			opts: []TransportOption{WithSkipServerErrorsFromCache(true)},
			check: func(tr *Transport) bool {
				return tr.SkipServerErrorsFromCache
			},
			message: "SkipServerErrorsFromCache should be true",
		},
		{
			name: "WithPublicCache",
			opts: []TransportOption{WithPublicCache(true)},
			check: func(tr *Transport) bool {
				return tr.IsPublicCache
			},
			message: "IsPublicCache should be true",
		},
		{
			name: "WithVarySeparation",
			opts: []TransportOption{WithVarySeparation(true)},
			check: func(tr *Transport) bool {
				return tr.EnableVarySeparation
			},
			message: "EnableVarySeparation should be true",
		},
		{
			name: "WithDisableWarningHeader",
			opts: []TransportOption{WithDisableWarningHeader(true)},
			check: func(tr *Transport) bool {
				return tr.DisableWarningHeader
			},
			message: "DisableWarningHeader should be true",
		},
		{
			name: "WithCacheKeyHeaders",
			opts: []TransportOption{WithCacheKeyHeaders([]string{"Authorization", "Accept-Language"})},
			check: func(tr *Transport) bool {
				return len(tr.CacheKeyHeaders) == 2
			},
			message: "CacheKeyHeaders should have 2 elements",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := NewTransport(cache, tt.opts...)
			if !tt.check(transport) {
				t.Error(tt.message)
			}
		})
	}
}

func TestIntegrationWithEncryption(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello, World!"))
	}))
	defer server.Close()

	cache := newMockCacheInternal()
	transport := NewTransport(cache, WithEncryption("integration-test-passphrase"))
	client := &http.Client{Transport: transport}

	// First request - should cache
	resp, err := client.Get(server.URL + "/test")
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	// Must read body to trigger caching (OnEOF callback)
	body := make([]byte, 1024)
	n, _ := resp.Body.Read(body)
	resp.Body.Close()
	body = body[:n]

	// Verify the response content
	if string(body) != "Hello, World!" {
		t.Errorf("unexpected body: %q", string(body))
	}

	// Verify data is encrypted in cache
	hashedKey := hashKey(server.URL + "/test")
	storedData, ok := cache.data[hashedKey]
	if !ok {
		t.Error("response should be cached")
	} else {
		// Stored data should be encrypted (not contain "Hello, World!" in plaintext)
		if containsSubstring(storedData, "Hello, World!") {
			t.Error("cached data should be encrypted, not contain plaintext response")
		}
	}

	// Second request - should be served from cache
	resp2, err := client.Get(server.URL + "/test")
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.Header.Get(XFromCache) != "1" {
		t.Error("second request should be served from cache")
	}
}

// containsSubstring checks if data contains the substring
func containsSubstring(data []byte, substr string) bool {
	return len(data) >= len(substr) && string(data) != "" &&
		(string(data) == substr || contains(data, []byte(substr)))
}

func contains(data, substr []byte) bool {
	for i := 0; i <= len(data)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if data[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
