package vcsaccount

// Cipher abstracts symmetric encryption for VCS token storage.
// The service layer provides the concrete implementation (AES-256-GCM).
type Cipher interface {
	Encrypt(plaintext, key []byte) ([]byte, error)
	Decrypt(ciphertext, key []byte) ([]byte, error)
}
