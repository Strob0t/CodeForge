package vcsaccount

import "testing"

func TestCipherInterface(t *testing.T) {
	// Verify that the Cipher interface is well-formed by asserting it
	// can be used as a variable type. The concrete implementation lives
	// in internal/crypto and is tested there (aes_test.go, crypto_test.go).
	var _ Cipher = nil
	_ = t // avoid unused
}
