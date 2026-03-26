package crypto //nolint:revive // intentional package name for encryption utilities

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
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
// Uses crypto/rand.Int for uniform distribution (no modular bias).
func GenerateRandomPassword(length int) (string, error) {
	const (
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		upper   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		lower   = "abcdefghijklmnopqrstuvwxyz"
		digits  = "0123456789"
	)
	charsetLen := big.NewInt(int64(len(charset)))

	result := make([]byte, length)
	for i := range result {
		idx, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", fmt.Errorf("generate random index: %w", err)
		}
		result[i] = charset[idx.Int64()]
	}

	// Ensure at least one of each required character class at random positions.
	classes := []string{upper, lower, digits}
	for _, class := range classes {
		pos, err := rand.Int(rand.Reader, big.NewInt(int64(length)))
		if err != nil {
			return "", fmt.Errorf("generate random position: %w", err)
		}
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(class))))
		if err != nil {
			return "", fmt.Errorf("generate random class char: %w", err)
		}
		result[pos.Int64()] = class[idx.Int64()]
	}

	return string(result), nil
}

// HashSHA256 returns the hex-encoded SHA-256 hash of the given string.
func HashSHA256(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}
