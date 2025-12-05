package securecache

import (
	"bytes"
	"context"
	"testing"
)

// mockCache is a simple in-memory cache for testing.
type mockCache struct {
	data map[string][]byte
}

func newMockCache() *mockCache {
	return &mockCache{
		data: make(map[string][]byte),
	}
}

func (m *mockCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	val, ok := m.data[key]
	return val, ok, nil
}

func (m *mockCache) Set(_ context.Context, key string, val []byte) error {
	m.data[key] = val
	return nil
}

func (m *mockCache) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}

// TestNewSecureCache tests the creation of a SecureCache.
func TestNewSecureCache(t *testing.T) {
	cache := newMockCache()

	// Test without encryption
	sc, err := New(Config{Cache: cache})
	if err != nil {
		t.Fatalf("New() without encryption failed: %v", err)
	}
	if sc.IsEncrypted() {
		t.Error("Expected IsEncrypted() to be false")
	}

	// Test with encryption
	scEncrypted, err := New(Config{
		Cache:      cache,
		Passphrase: "test-passphrase-123",
	})
	if err != nil {
		t.Fatalf("New() with encryption failed: %v", err)
	}
	if !scEncrypted.IsEncrypted() {
		t.Error("Expected IsEncrypted() to be true")
	}
}

// TestNewSecureCacheNilCache tests that New() fails with nil cache.
func TestNewSecureCacheNilCache(t *testing.T) {
	_, err := New(Config{Cache: nil})
	if err == nil {
		t.Error("Expected error when cache is nil")
	}
}

// TestKeyHashing tests that keys are always hashed.
func TestKeyHashing(t *testing.T) {
	ctx := context.Background()
	cache := newMockCache()
	sc, err := New(Config{Cache: cache})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	key := "test-key"
	value := []byte("test-value")

	// Set a value
	if err := sc.Set(ctx, key, value); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// The key should be hashed in the underlying cache
	hashedKey := sc.hashKey(key)
	if _, ok, _ := cache.Get(ctx, hashedKey); !ok {
		t.Error("Expected hashed key to exist in underlying cache")
	}

	// Original key should not exist
	if _, ok, _ := cache.Get(ctx, key); ok {
		t.Error("Original key should not exist in underlying cache")
	}

	// Get should return the value
	retrieved, ok, err := sc.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if !ok {
		t.Error("Get() should return true for existing key")
	}
	if !bytes.Equal(retrieved, value) {
		t.Errorf("Get() = %s, want %s", retrieved, value)
	}
}

// TestEncryptionDecryption tests encryption and decryption of data.
func TestEncryptionDecryption(t *testing.T) {
	ctx := context.Background()
	cache := newMockCache()
	sc, err := New(Config{
		Cache:      cache,
		Passphrase: "secure-passphrase-456",
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	key := "encrypted-key"
	value := []byte("sensitive-data-that-should-be-encrypted")

	// Set a value
	if err := sc.Set(ctx, key, value); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// The stored data should be encrypted (different from original)
	hashedKey := sc.hashKey(key)
	stored, ok, _ := cache.Get(ctx, hashedKey)
	if !ok {
		t.Fatal("Expected data to be stored in underlying cache")
	}
	if bytes.Equal(stored, value) {
		t.Error("Stored data should be encrypted (different from original)")
	}

	// Get should decrypt and return the original value
	retrieved, ok, err := sc.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if !ok {
		t.Error("Get() should return true for existing key")
	}
	if !bytes.Equal(retrieved, value) {
		t.Errorf("Get() = %s, want %s", retrieved, value)
	}
}

// TestDelete tests deletion of cached data.
func TestDelete(t *testing.T) {
	ctx := context.Background()
	cache := newMockCache()
	sc, err := New(Config{Cache: cache})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	key := "delete-key"
	value := []byte("delete-value")

	// Set and verify
	_ = sc.Set(ctx, key, value)
	if _, ok, _ := sc.Get(ctx, key); !ok {
		t.Error("Expected key to exist after Set()")
	}

	// Delete
	_ = sc.Delete(ctx, key)

	// Verify deletion
	if _, ok, _ := sc.Get(ctx, key); ok {
		t.Error("Expected key to not exist after Delete()")
	}

	// Verify underlying cache
	hashedKey := sc.hashKey(key)
	if _, ok, _ := cache.Get(ctx, hashedKey); ok {
		t.Error("Expected hashed key to not exist in underlying cache after Delete()")
	}
}

// TestMultipleKeysWithEncryption tests multiple keys with encryption.
func TestMultipleKeysWithEncryption(t *testing.T) {
	ctx := context.Background()
	cache := newMockCache()
	sc, err := New(Config{
		Cache:      cache,
		Passphrase: "multi-key-passphrase",
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	testCases := []struct {
		key   string
		value []byte
	}{
		{"key1", []byte("value1")},
		{"key2", []byte("value2-longer-data")},
		{"key3", []byte("value3-even-longer-data-with-special-chars-!@#$%")},
	}

	// Set all values
	for _, tc := range testCases {
		_ = sc.Set(ctx, tc.key, tc.value)
	}

	// Verify all values
	for _, tc := range testCases {
		retrieved, ok, _ := sc.Get(ctx, tc.key)
		if !ok {
			t.Errorf("Get(%s) should return true", tc.key)
			continue
		}
		if !bytes.Equal(retrieved, tc.value) {
			t.Errorf("Get(%s) = %s, want %s", tc.key, retrieved, tc.value)
		}
	}
}

// TestEmptyValue tests handling of empty values.
func TestEmptyValue(t *testing.T) {
	ctx := context.Background()
	cache := newMockCache()
	sc, err := New(Config{
		Cache:      cache,
		Passphrase: "empty-test-passphrase",
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	key := "empty-key"
	value := []byte("")

	_ = sc.Set(ctx, key, value)

	retrieved, ok, _ := sc.Get(ctx, key)
	if !ok {
		t.Error("Get() should return true for empty value")
	}
	if !bytes.Equal(retrieved, value) {
		t.Errorf("Get() = %v, want empty slice", retrieved)
	}
}

// TestLargeValue tests handling of large values.
func TestLargeValue(t *testing.T) {
	ctx := context.Background()
	cache := newMockCache()
	sc, err := New(Config{
		Cache:      cache,
		Passphrase: "large-value-passphrase",
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	key := "large-key"
	// Create a 1MB value
	value := make([]byte, 1024*1024)
	for i := range value {
		value[i] = byte(i % 256)
	}

	_ = sc.Set(ctx, key, value)

	retrieved, ok, _ := sc.Get(ctx, key)
	if !ok {
		t.Error("Get() should return true for large value")
	}
	if !bytes.Equal(retrieved, value) {
		t.Error("Retrieved large value does not match original")
	}
}

// TestCorruptedData tests handling of corrupted encrypted data.
func TestCorruptedData(t *testing.T) {
	ctx := context.Background()
	cache := newMockCache()
	sc, err := New(Config{
		Cache:      cache,
		Passphrase: "corruption-test-passphrase",
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	key := "corrupted-key"
	value := []byte("original-value")

	// Set a value
	_ = sc.Set(ctx, key, value)

	// Corrupt the underlying data
	hashedKey := sc.hashKey(key)
	stored, _, _ := cache.Get(ctx, hashedKey)
	if len(stored) > 20 {
		stored[20] ^= 0xFF // Flip bits to corrupt
		_ = cache.Set(ctx, hashedKey, stored)
	}

	// Get should fail gracefully
	_, ok, _ := sc.Get(ctx, key)
	if ok {
		t.Error("Get() should return false for corrupted data")
	}
}

// TestDifferentPassphrases tests that different passphrases cannot decrypt data.
func TestDifferentPassphrases(t *testing.T) {
	ctx := context.Background()
	cache := newMockCache()

	// Create cache with first passphrase
	sc1, err := New(Config{
		Cache:      cache,
		Passphrase: "passphrase-one",
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	key := "secret-key"
	value := []byte("secret-value")
	_ = sc1.Set(ctx, key, value)

	// Create cache with different passphrase
	sc2, err := New(Config{
		Cache:      cache,
		Passphrase: "passphrase-two",
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Should not be able to decrypt
	_, ok, _ := sc2.Get(ctx, key)
	if ok {
		t.Error("Get() with different passphrase should fail to decrypt")
	}
}

// TestHashKeyConsistency tests that hashKey produces consistent results.
func TestHashKeyConsistency(t *testing.T) {
	cache := newMockCache()
	sc, err := New(Config{Cache: cache})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	key := "consistency-test-key"
	hash1 := sc.hashKey(key)
	hash2 := sc.hashKey(key)

	if hash1 != hash2 {
		t.Errorf("hashKey() should produce consistent results, got %s and %s", hash1, hash2)
	}

	// Hash should be 64 characters (SHA-256 hex)
	if len(hash1) != 64 {
		t.Errorf("hashKey() should produce 64-character hex string, got %d characters", len(hash1))
	}
}

// TestIntegrationWithDiskCache tests integration with actual diskcache.
func TestIntegrationWithDiskCache(t *testing.T) {
	ctx := context.Background()
	sc, err := New(Config{
		Cache:      newMockCache(),
		Passphrase: "integration-test-passphrase",
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Test basic operations
	key := "integration-key"
	value := []byte("integration-value")

	_ = sc.Set(ctx, key, value)

	retrieved, ok, _ := sc.Get(ctx, key)
	if !ok {
		t.Error("Get() should return true")
	}
	if !bytes.Equal(retrieved, value) {
		t.Errorf("Get() = %s, want %s", retrieved, value)
	}

	_ = sc.Delete(ctx, key)

	if _, ok, _ := sc.Get(ctx, key); ok {
		t.Error("Get() should return false after Delete()")
	}
}
