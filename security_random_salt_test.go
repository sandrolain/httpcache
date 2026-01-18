package httpcache

import (
	"bytes"
	"crypto/rand"
	"testing"
)

// TestInitEncryptionFixedSalt verifies fixed salt mode (default)
func TestInitEncryptionFixedSalt(t *testing.T) {
	cfg, err := initEncryption("test-passphrase", false)
	if err != nil {
		t.Fatalf("initEncryption failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil securityConfig")
	}
	if cfg.gcm == nil {
		t.Fatal("expected non-nil GCM")
	}
	if cfg.useRandomSalt {
		t.Error("expected useRandomSalt to be false")
	}
	if cfg.fixedSalt == nil {
		t.Error("expected non-nil fixedSalt")
	}
	if len(cfg.fixedSalt) != 32 {
		t.Errorf("expected fixedSalt length 32, got %d", len(cfg.fixedSalt))
	}
}

// TestInitEncryptionRandomSalt verifies random salt mode
func TestInitEncryptionRandomSalt(t *testing.T) {
	cfg, err := initEncryption("test-passphrase", true)
	if err != nil {
		t.Fatalf("initEncryption failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil securityConfig")
	}
	if cfg.gcm == nil {
		t.Fatal("expected non-nil GCM")
	}
	if !cfg.useRandomSalt {
		t.Error("expected useRandomSalt to be true")
	}
	// fixedSalt is kept for backward compatibility with legacy format
	if cfg.fixedSalt == nil {
		t.Error("expected non-nil fixedSalt for backward compatibility")
	}
}

// TestEncryptDecryptFixedSalt verifies encryption/decryption with fixed salt
func TestEncryptDecryptFixedSalt(t *testing.T) {
	cfg, err := initEncryption("test-passphrase", false)
	if err != nil {
		t.Fatalf("initEncryption failed: %v", err)
	}

	plaintext := []byte("test data for encryption")
	encrypted, err := encrypt(cfg, plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// Fixed salt mode uses legacy format (no version byte)
	if len(encrypted) < 1 {
		t.Fatal("encrypted data too short")
	}

	// Decrypt
	decrypted, err := decrypt(cfg, encrypted)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decrypted data mismatch\ngot:  %q\nwant: %q", decrypted, plaintext)
	}
}

// TestEncryptDecryptRandomSalt verifies encryption/decryption with random salt
func TestEncryptDecryptRandomSalt(t *testing.T) {
	cfg, err := initEncryption("test-passphrase", true)
	if err != nil {
		t.Fatalf("initEncryption failed: %v", err)
	}

	plaintext := []byte("test data for encryption with random salt")
	encrypted, err := encrypt(cfg, plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// Random salt mode uses new format with version byte
	if len(encrypted) < 34 { // 1 version + 32 salt + 1 minimum for nonce+ciphertext
		t.Fatal("encrypted data too short for random salt format")
	}
	if encrypted[0] != 1 {
		t.Errorf("expected version byte 1, got %d", encrypted[0])
	}

	// Decrypt
	decrypted, err := decrypt(cfg, encrypted)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decrypted data mismatch\ngot:  %q\nwant: %q", decrypted, plaintext)
	}
}

// TestRandomSaltUniqueness verifies each encryption generates a unique salt
func TestRandomSaltUniqueness(t *testing.T) {
	cfg, err := initEncryption("test-passphrase", true)
	if err != nil {
		t.Fatalf("initEncryption failed: %v", err)
	}

	plaintext := []byte("same plaintext")
	encrypted1, err := encrypt(cfg, plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	encrypted2, err := encrypt(cfg, plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// Extract salts (bytes 1-33)
	salt1 := encrypted1[1:33]
	salt2 := encrypted2[1:33]

	if bytes.Equal(salt1, salt2) {
		t.Error("expected different salts for each encryption")
	}

	// Verify both decrypt correctly
	decrypted1, err := decrypt(cfg, encrypted1)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	decrypted2, err := decrypt(cfg, encrypted2)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted1) || !bytes.Equal(plaintext, decrypted2) {
		t.Error("decrypted data mismatch")
	}
}

// TestBackwardCompatibility verifies random salt config can decrypt fixed salt data
func TestBackwardCompatibility(t *testing.T) {
	// Encrypt with fixed salt
	cfgFixed, err := initEncryption("test-passphrase", false)
	if err != nil {
		t.Fatalf("initEncryption fixed failed: %v", err)
	}
	plaintext := []byte("backward compatibility test")
	encryptedFixed, err := encrypt(cfgFixed, plaintext)
	if err != nil {
		t.Fatalf("encrypt fixed failed: %v", err)
	}

	// Decrypt with random salt config (should auto-detect format)
	cfgRandom, err := initEncryption("test-passphrase", true)
	if err != nil {
		t.Fatalf("initEncryption random failed: %v", err)
	}
	decrypted, err := decrypt(cfgRandom, encryptedFixed)
	if err != nil {
		t.Fatalf("decrypt with random salt config failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decrypted data mismatch\ngot:  %q\nwant: %q", decrypted, plaintext)
	}
}

// TestEncryptEmptyData verifies encryption of empty data
func TestEncryptEmptyData(t *testing.T) {
	cfgFixed, err := initEncryption("test-passphrase", false)
	if err != nil {
		t.Fatalf("initEncryption failed: %v", err)
	}
	cfgRandom, err := initEncryption("test-passphrase", true)
	if err != nil {
		t.Fatalf("initEncryption failed: %v", err)
	}

	plaintext := []byte{}

	// Fixed salt
	encryptedFixed, err := encrypt(cfgFixed, plaintext)
	if err != nil {
		t.Fatalf("encrypt fixed failed: %v", err)
	}
	decryptedFixed, err := decrypt(cfgFixed, encryptedFixed)
	if err != nil {
		t.Fatalf("decrypt fixed failed: %v", err)
	}
	if !bytes.Equal(plaintext, decryptedFixed) {
		t.Error("decrypted empty data mismatch (fixed)")
	}

	// Random salt
	encryptedRandom, err := encrypt(cfgRandom, plaintext)
	if err != nil {
		t.Fatalf("encrypt random failed: %v", err)
	}
	decryptedRandom, err := decrypt(cfgRandom, encryptedRandom)
	if err != nil {
		t.Fatalf("decrypt random failed: %v", err)
	}
	if !bytes.Equal(plaintext, decryptedRandom) {
		t.Error("decrypted empty data mismatch (random)")
	}
}

// TestEncryptLargeData verifies encryption of large data
func TestEncryptLargeData(t *testing.T) {
	cfg, err := initEncryption("test-passphrase", true)
	if err != nil {
		t.Fatalf("initEncryption failed: %v", err)
	}

	// Create 1MB of random data
	plaintext := make([]byte, 1024*1024)
	if _, err := rand.Read(plaintext); err != nil {
		t.Fatalf("failed to generate random data: %v", err)
	}

	encrypted, err := encrypt(cfg, plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	decrypted, err := decrypt(cfg, encrypted)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Error("decrypted large data mismatch")
	}
}

// TestDecryptInvalidData verifies proper error handling
func TestDecryptInvalidData(t *testing.T) {
	cfg, err := initEncryption("test-passphrase", true)
	if err != nil {
		t.Fatalf("initEncryption failed: %v", err)
	}

	tests := []struct {
		name string
		data []byte
	}{
		{"empty data", []byte{}},
		{"too short", []byte{1, 2, 3}},
		{"invalid version", append([]byte{99}, make([]byte, 50)...)},
		{"corrupted data", make([]byte, 100)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decrypt(cfg, tt.data)
			if err == nil {
				t.Error("expected error for invalid data")
			}
		})
	}
}

// TestWithRandomSaltEncryptionOption verifies the TransportOption
func TestWithRandomSaltEncryptionOption(t *testing.T) {
	tr := &Transport{}
	opt := WithRandomSaltEncryption("test-passphrase")
	if err := opt(tr); err != nil {
		t.Fatalf("WithRandomSaltEncryption failed: %v", err)
	}

	if tr.security == nil {
		t.Fatal("expected non-nil security config")
	}
	if tr.security.gcm == nil {
		t.Fatal("expected non-nil GCM")
	}
	if !tr.security.useRandomSalt {
		t.Error("expected useRandomSalt to be true")
	}
}

// TestWithEncryptionOption verifies the default fixed salt option
func TestWithEncryptionOption(t *testing.T) {
	tr := &Transport{}
	opt := WithEncryption("test-passphrase")
	if err := opt(tr); err != nil {
		t.Fatalf("WithEncryption failed: %v", err)
	}

	if tr.security == nil {
		t.Fatal("expected non-nil security config")
	}
	if tr.security.gcm == nil {
		t.Fatal("expected non-nil GCM")
	}
	if tr.security.useRandomSalt {
		t.Error("expected useRandomSalt to be false")
	}
	if tr.security.fixedSalt == nil {
		t.Error("expected non-nil fixedSalt")
	}
}
