package vcsaccount

import "github.com/Strob0t/CodeForge/internal/crypto"

// DeriveKey derives a 32-byte AES-256 key using HKDF (RFC 5869) for VCS account encryption.
func DeriveKey(secret string, salt []byte, info string) ([]byte, error) {
	return crypto.DeriveKey(secret, salt, info)
}

// DeriveKeyLegacy derives a 32-byte key using plain SHA-256 (pre-HKDF migration).
//
// Deprecated: use DeriveKey for all new encryption.
func DeriveKeyLegacy(secret string) []byte {
	return crypto.DeriveKeyLegacy(secret)
}

// Encrypt encrypts plaintext with AES-256-GCM using the given key.
// The 12-byte nonce is prepended to the ciphertext.
func Encrypt(plaintext, key []byte) ([]byte, error) {
	return crypto.Encrypt(plaintext, key)
}

// Decrypt decrypts ciphertext produced by Encrypt (nonce || ciphertext).
func Decrypt(ciphertext, key []byte) ([]byte, error) {
	return crypto.Decrypt(ciphertext, key)
}
