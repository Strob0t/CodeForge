package vcsaccount

import "github.com/Strob0t/CodeForge/internal/crypto"

// DeriveKey derives a 32-byte AES-256 key from a JWT secret using SHA-256.
func DeriveKey(jwtSecret string) []byte {
	return crypto.DeriveKey(jwtSecret)
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
