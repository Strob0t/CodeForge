package crypto //nolint:revive // intentional package name for encryption utilities

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

const nonceSize = 12 // standard GCM nonce length

// DeriveKey derives a 32-byte AES-256 key using HKDF (RFC 5869) with SHA-256.
//
// Parameters:
//   - secret: the input keying material (e.g. JWT secret or dedicated encryption key)
//   - salt: optional salt for HKDF Extract; nil uses a zero-filled salt per RFC 5869
//   - info: domain separation string (e.g. "codeforge/llmkey/v1")
//
// Returns a 32-byte key suitable for AES-256-GCM, or an error if derivation fails.
func DeriveKey(secret string, salt []byte, info string) ([]byte, error) {
	return hkdf.Key(sha256.New, []byte(secret), salt, info, 32)
}

// DeriveKeyLegacy derives a 32-byte AES-256 key from a secret using plain SHA-256.
// This function exists only to decrypt data encrypted before the HKDF migration.
//
// Deprecated: use DeriveKey (HKDF) for all new encryption.
func DeriveKeyLegacy(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// Encrypt encrypts plaintext with AES-256-GCM using the given key.
// The 12-byte nonce is prepended to the ciphertext.
func Encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher.NewGCM: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("rand nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts ciphertext produced by Encrypt (nonce || ciphertext).
func Decrypt(ciphertext, key []byte) ([]byte, error) {
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher.NewGCM: %w", err)
	}

	nonce := ciphertext[:nonceSize]
	ct := ciphertext[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("gcm.Open: %w", err)
	}

	return plaintext, nil
}
