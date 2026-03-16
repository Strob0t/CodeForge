package crypto //nolint:revive // intentional package name for encryption utilities

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// GenerateRequestID returns a 16-byte random hex string (32 chars).
func GenerateRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GenerateUUIDv4 produces a UUID v4 string using crypto/rand.
func GenerateUUIDv4() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// GenerateRandomToken generates a 32-byte hex token using crypto/rand.
func GenerateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GenerateRandomPassword creates a random password of the given length
// containing uppercase, lowercase, and digits.
func GenerateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	// Ensure at least one of each required character class
	b[0] = 'A' + b[0]%26 // uppercase
	b[1] = 'a' + b[1]%26 // lowercase
	b[2] = '0' + b[2]%10 // digit
	return string(b), nil
}

// HashSHA256 returns the hex-encoded SHA-256 hash of the given string.
func HashSHA256(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}
