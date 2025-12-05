// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

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

// securityConfig holds the security configuration for the Transport.
type securityConfig struct {
	gcm        cipher.AEAD
	passphrase string
}

// hashKey converts a cache key to its SHA-256 hash representation.
// This is always applied to cache keys before passing to the backend.
func hashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// initEncryption initializes the AES-256-GCM cipher using the passphrase.
func initEncryption(passphrase string) (cipher.AEAD, error) {
	// Derive a 32-byte key from the passphrase using scrypt
	// Using a fixed salt here - in production, consider storing a random salt
	salt := sha256.Sum256([]byte("httpcache-securecache-salt-v1"))
	key, err := scrypt.Key([]byte(passphrase), salt[:], scryptN, scryptR, scryptP, keyLength)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return gcm, nil
}

// encrypt encrypts data using AES-256-GCM.
// Returns the encrypted data with the nonce prepended.
func encrypt(gcm cipher.AEAD, data []byte) ([]byte, error) {
	if gcm == nil {
		return data, nil // No encryption configured
	}

	// Generate a random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the data
	// #nosec G407 -- nonce is randomly generated above using crypto/rand, not hardcoded
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// decrypt decrypts data using AES-256-GCM.
// Expects the nonce to be prepended to the ciphertext.
func decrypt(gcm cipher.AEAD, data []byte) ([]byte, error) {
	if gcm == nil {
		return data, nil // No decryption needed
	}

	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract nonce and ciphertext
	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	// Decrypt the data
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// IsEncryptionEnabled returns true if the Transport is configured with encryption.
func (t *Transport) IsEncryptionEnabled() bool {
	return t.security != nil && t.security.gcm != nil
}
