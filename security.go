// Package httpcache provides a http.RoundTripper implementation that works as a
// mostly RFC 9111 compliant cache for HTTP responses.
package httpcache

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash"
	"io"
	"strconv"
	"sync"

	"github.com/cespare/xxhash/v2"
	"golang.org/x/crypto/scrypt"
)

// HashAlgorithm represents the hashing algorithm to use for cache keys.
type HashAlgorithm int

const (
	// HashAlgorithmSHA256 uses SHA-256 for hashing (default, backward compatible).
	// More secure but slower than xxHash.
	HashAlgorithmSHA256 HashAlgorithm = iota
	// HashAlgorithmXXHash uses xxHash for hashing (faster, ~10x than SHA-256).
	// Recommended for high-throughput scenarios where cryptographic security is not required.
	HashAlgorithmXXHash
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
	// saltSize is the size of the salt for random salt mode
	saltSize = 32
	// versionByte identifies the encryption format version
	versionByte byte = 0x01
)

// hashPool is a sync.Pool for reusing hash.Hash instances to reduce allocations.
var hashPool = sync.Pool{
	New: func() interface{} {
		return sha256.New()
	},
}

// securityConfig holds the security configuration for the Transport.
type securityConfig struct {
	gcm           cipher.AEAD
	passphrase    string
	useRandomSalt bool
	fixedSalt     []byte // Used when useRandomSalt is false for backward compatibility
}

// hashKey converts a cache key to its hash representation using SHA-256.
// This is always applied to cache keys before passing to the backend.
// Uses sync.Pool to reduce allocations and base64 for more compact output.
func hashKey(key string) string {
	// Get hash instance from pool
	h, ok := hashPool.Get().(hash.Hash)
	if !ok {
		// Fallback to creating new hash if pool returns wrong type
		h = sha256.New()
	}
	defer func() {
		h.Reset()
		hashPool.Put(h)
	}()

	// Write key to hash
	_, _ = h.Write([]byte(key)) // hash.Hash.Write never returns an error

	// Use base64.RawURLEncoding (no padding) for more compact and faster encoding
	// RawURLEncoding is URL-safe and produces shorter strings than hex encoding
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// hashKeyXXHash converts a cache key to its xxHash representation.
// xxHash is much faster than SHA-256 (~10x) but not cryptographically secure.
// Suitable for cache keys where speed is more important than cryptographic security.
// Returns a base36-encoded string for compactness.
func hashKeyXXHash(key string) string {
	// xxHash is extremely fast and produces 64-bit output
	h := xxhash.Sum64String(key)

	// Convert to base36 for compact representation (using 0-9, a-z)
	// Base36 is URL-safe and more compact than base64 for 64-bit values
	return strconv.FormatUint(h, 36)
}

// hashKeyWithAlgorithm converts a cache key using the specified algorithm.
func hashKeyWithAlgorithm(key string, algo HashAlgorithm) string {
	switch algo {
	case HashAlgorithmXXHash:
		return hashKeyXXHash(key)
	default:
		return hashKey(key)
	}
}

// initEncryption initializes the AES-256-GCM cipher using the passphrase.
// When useRandomSalt is false (default), uses a fixed salt for backward compatibility.
// When true, uses random salts stored with each encrypted value for improved security.
func initEncryption(passphrase string, useRandomSalt bool) (*securityConfig, error) {
	var fixedSalt []byte
	if !useRandomSalt {
		// Fixed salt for backward compatibility
		saltHash := sha256.Sum256([]byte("httpcache-securecache-salt-v1"))
		fixedSalt = saltHash[:]
	} else {
		// Random salt mode also needs a fixed salt for backward compatibility with legacy format
		saltHash := sha256.Sum256([]byte("httpcache-securecache-salt-v1"))
		fixedSalt = saltHash[:]
	}

	// Create GCM cipher with fixed salt for:
	// - Fixed salt mode: encrypt and decrypt operations
	// - Random salt mode: decrypt legacy format only
	key, err := scrypt.Key([]byte(passphrase), fixedSalt, scryptN, scryptR, scryptP, keyLength)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &securityConfig{
		gcm:           gcm,
		passphrase:    passphrase,
		useRandomSalt: useRandomSalt,
		fixedSalt:     fixedSalt,
	}, nil
}

// encrypt encrypts data using AES-256-GCM.
// Returns the encrypted data with version, salt (if random), and nonce.
func encrypt(cfg *securityConfig, data []byte) ([]byte, error) {
	if cfg == nil || (cfg.gcm == nil && !cfg.useRandomSalt) {
		return data, nil // No encryption configured
	}

	var gcm cipher.AEAD
	var salt []byte

	if cfg.useRandomSalt {
		// Generate random salt
		salt = make([]byte, saltSize)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, fmt.Errorf("failed to generate salt: %w", err)
		}

		// Derive key with random salt
		key, err := scrypt.Key([]byte(cfg.passphrase), salt, scryptN, scryptR, scryptP, keyLength)
		if err != nil {
			return nil, fmt.Errorf("failed to derive key: %w", err)
		}

		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("failed to create cipher: %w", err)
		}

		gcm, err = cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("failed to create GCM: %w", err)
		}
	} else {
		gcm = cfg.gcm
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the data
	// #nosec G407 -- nonce is randomly generated above using crypto/rand, not hardcoded
	ciphertext := gcm.Seal(nil, nonce, data, nil)

	// Build encrypted data structure
	if cfg.useRandomSalt {
		// Format: [version:1][salt:32][nonce:12][ciphertext:n]
		result := make([]byte, 1+saltSize+len(nonce)+len(ciphertext))
		result[0] = versionByte
		copy(result[1:], salt)
		copy(result[1+saltSize:], nonce)
		copy(result[1+saltSize+len(nonce):], ciphertext)
		return result, nil
	}
	// Legacy format: [nonce:12][ciphertext:n]
	result := make([]byte, len(nonce)+len(ciphertext))
	copy(result, nonce)
	copy(result[len(nonce):], ciphertext)
	return result, nil
}

// decrypt decrypts data using AES-256-GCM.
// Supports both legacy format (nonce+ciphertext) and new format (version+salt+nonce+ciphertext).
func decrypt(cfg *securityConfig, data []byte) ([]byte, error) {
	if cfg == nil || (cfg.gcm == nil && !cfg.useRandomSalt) {
		return data, nil // No decryption needed
	}

	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	var gcm cipher.AEAD
	var nonce, ciphertext []byte

	// Check if data starts with version byte (new format)
	if len(data) > 1+saltSize+nonceSize && data[0] == versionByte {
		// New format: [version:1][salt:32][nonce:12][ciphertext:n]
		if !cfg.useRandomSalt {
			// Try to decrypt with fixed salt if random salt mode is not enabled
			// This handles transition case
			gcm = cfg.gcm
			nonce = data[:nonceSize]
			ciphertext = data[nonceSize:]
		} else {
			salt := data[1 : 1+saltSize]
			nonce = data[1+saltSize : 1+saltSize+nonceSize]
			ciphertext = data[1+saltSize+nonceSize:]

			// Derive key with extracted salt
			key, err := scrypt.Key([]byte(cfg.passphrase), salt, scryptN, scryptR, scryptP, keyLength)
			if err != nil {
				return nil, fmt.Errorf("failed to derive key: %w", err)
			}

			block, err := aes.NewCipher(key)
			if err != nil {
				return nil, fmt.Errorf("failed to create cipher: %w", err)
			}

			gcm, err = cipher.NewGCM(block)
			if err != nil {
				return nil, fmt.Errorf("failed to create GCM: %w", err)
			}
		}
	} else {
		// Legacy format: [nonce:12][ciphertext:n]
		gcm = cfg.gcm
		if gcm == nil {
			return nil, fmt.Errorf("no cipher configured for legacy format")
		}
		nonce = data[:nonceSize]
		ciphertext = data[nonceSize:]
	}

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
