// Package securecache provides a security wrapper for httpcache.Cache implementations.
// It adds SHA-256 key hashing (always enabled) and optional AES-256-GCM encryption for cached data.
package securecache

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/sandrolain/httpcache"
	"golang.org/x/crypto/scrypt"
)

const (
	// scryptN is the CPU/memory cost parameter for scrypt key derivation
	scryptN = 32768
	// scryptR is the block size parameter for scrypt
	scryptR = 8
	// scryptP is the parallelization parameter for scrypt
	scryptP = 1
	// keyLength is the desired key length for AES-256
	keyLength = 32
	// nonceSize is the size of the GCM nonce
	nonceSize = 12
)

// SecureCache wraps an existing cache implementation to add security features:
// - SHA-256 hashing of all cache keys (always enabled)
// - Optional AES-256-GCM encryption of cached data (when passphrase is provided)
type SecureCache struct {
	cache      httpcache.Cache
	gcm        cipher.AEAD
	passphrase string
}

// Config holds the configuration for creating a SecureCache.
type Config struct {
	// Cache is the underlying cache implementation to wrap.
	Cache httpcache.Cache

	// Passphrase is the secret used to encrypt/decrypt cached data.
	// If empty, only key hashing is performed (no encryption).
	// Must be kept secret and consistent across application restarts.
	Passphrase string
}

// New creates a new SecureCache that wraps the provided cache.
// Keys are always hashed with SHA-256.
// If a passphrase is provided, cached data is encrypted with AES-256-GCM.
func New(config Config) (*SecureCache, error) {
	if config.Cache == nil {
		return nil, fmt.Errorf("cache cannot be nil")
	}

	sc := &SecureCache{
		cache:      config.Cache,
		passphrase: config.Passphrase,
	}

	// If passphrase is provided, initialize encryption
	if config.Passphrase != "" {
		if err := sc.initEncryption(); err != nil {
			return nil, fmt.Errorf("failed to initialize encryption: %w", err)
		}
	}

	return sc, nil
}

// initEncryption initializes the AES-256-GCM cipher using the passphrase.
func (sc *SecureCache) initEncryption() error {
	// Derive a 32-byte key from the passphrase using scrypt
	// Using a fixed salt here - in production, consider storing a random salt
	salt := sha256.Sum256([]byte("httpcache-securecache-salt-v1"))
	key, err := scrypt.Key([]byte(sc.passphrase), salt[:], scryptN, scryptR, scryptP, keyLength)
	if err != nil {
		return fmt.Errorf("failed to derive key: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	sc.gcm = gcm
	return nil
}

// hashKey converts a cache key to its SHA-256 hash representation.
func (sc *SecureCache) hashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// encrypt encrypts data using AES-256-GCM.
// Returns the encrypted data with the nonce prepended.
func (sc *SecureCache) encrypt(data []byte) ([]byte, error) {
	if sc.gcm == nil {
		return data, nil // No encryption configured
	}

	// Generate a random nonce
	nonce := make([]byte, sc.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the data
	// #nosec G407 -- nonce is randomly generated above using crypto/rand, not hardcoded
	ciphertext := sc.gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// decrypt decrypts data using AES-256-GCM.
// Expects the nonce to be prepended to the ciphertext.
func (sc *SecureCache) decrypt(data []byte) ([]byte, error) {
	if sc.gcm == nil {
		return data, nil // No decryption needed
	}

	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract nonce and ciphertext
	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	// Decrypt the data
	plaintext, err := sc.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// Get retrieves a cached response.
// The key is hashed with SHA-256 before lookup.
// The data is decrypted if encryption is enabled.
// Uses the provided context for cache operations.
func (sc *SecureCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	hashedKey := sc.hashKey(key)
	data, ok, err := sc.cache.Get(ctx, hashedKey)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	// Decrypt if encryption is enabled
	if sc.gcm != nil {
		plaintext, err := sc.decrypt(data)
		if err != nil {
			// Log error but don't expose it to caller
			httpcache.GetLogger().Warn("failed to decrypt cached data", "key", hashedKey, "error", err)
			return nil, false, err
		}
		return plaintext, true, nil
	}

	return data, true, nil
}

// Set stores a response in the cache.
// The key is hashed with SHA-256 before storage.
// The data is encrypted if encryption is enabled.
// Uses the provided context for cache operations.
func (sc *SecureCache) Set(ctx context.Context, key string, data []byte) error {
	hashedKey := sc.hashKey(key)

	// Encrypt if encryption is enabled
	var toStore []byte
	if sc.gcm != nil {
		encrypted, err := sc.encrypt(data)
		if err != nil {
			httpcache.GetLogger().Warn("failed to encrypt data", "key", hashedKey, "error", err)
			return err
		}
		toStore = encrypted
	} else {
		toStore = data
	}

	return sc.cache.Set(ctx, hashedKey, toStore)
}

// Delete removes a response from the cache.
// The key is hashed with SHA-256 before deletion.
// Uses the provided context for cache operations.
func (sc *SecureCache) Delete(ctx context.Context, key string) error {
	hashedKey := sc.hashKey(key)
	return sc.cache.Delete(ctx, hashedKey)
}

// IsEncrypted returns true if the cache is configured with encryption.
func (sc *SecureCache) IsEncrypted() bool {
	return sc.gcm != nil
}
