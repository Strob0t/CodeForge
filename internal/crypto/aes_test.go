package crypto //nolint:revive // intentional package name for encryption utilities

import (
	"bytes"
	"testing"
)

func TestDeriveKey_Deterministic(t *testing.T) {
	k1, err := DeriveKey("my-secret", nil, "test/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	k2, err := DeriveKey("my-secret", nil, "test/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	if len(k1) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(k1))
	}
	if !bytes.Equal(k1, k2) {
		t.Fatal("same input must produce same key")
	}
}

func TestDeriveKey_DifferentInputs(t *testing.T) {
	k1, err := DeriveKey("secret-a", nil, "test/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	k2, err := DeriveKey("secret-b", nil, "test/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	if bytes.Equal(k1, k2) {
		t.Fatal("different inputs must produce different keys")
	}
}

func TestDeriveKey_DifferentInfo(t *testing.T) {
	k1, err := DeriveKey("same-secret", nil, "codeforge/llmkey/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	k2, err := DeriveKey("same-secret", nil, "codeforge/vcsaccount/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	if bytes.Equal(k1, k2) {
		t.Fatal("different info strings must produce different keys (domain separation)")
	}
}

func TestDeriveKey_WithSalt(t *testing.T) {
	salt := []byte("random-salt-value")
	k1, err := DeriveKey("secret", salt, "test/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	k2, err := DeriveKey("secret", nil, "test/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	if bytes.Equal(k1, k2) {
		t.Fatal("different salts must produce different keys")
	}
}

func TestDeriveKeyLegacy_Deterministic(t *testing.T) {
	k1 := DeriveKeyLegacy("my-secret")
	k2 := DeriveKeyLegacy("my-secret")
	if len(k1) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(k1))
	}
	if !bytes.Equal(k1, k2) {
		t.Fatal("same input must produce same key")
	}
}

func TestDeriveKey_DiffersFromLegacy(t *testing.T) {
	hkdfKey, err := DeriveKey("my-secret", nil, "test/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	legacyKey := DeriveKeyLegacy("my-secret")
	if bytes.Equal(hkdfKey, legacyKey) {
		t.Fatal("HKDF key must differ from legacy SHA-256 key")
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key, err := DeriveKey("test-jwt-secret", nil, "test/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	plaintext := []byte("sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(ct) <= len(plaintext) {
		t.Fatal("ciphertext should be longer than plaintext")
	}

	got, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1, err := DeriveKey("secret-1", nil, "test/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	key2, err := DeriveKey("secret-2", nil, "test/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}

	ct, err := Encrypt([]byte("token"), key1)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err = Decrypt(ct, key2); err == nil {
		t.Fatal("expected error when decrypting with wrong key")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	key, err := DeriveKey("secret", nil, "test/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	if _, err := Decrypt([]byte("short"), key); err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}

func TestEncrypt_UniqueCiphertexts(t *testing.T) {
	key, err := DeriveKey("secret", nil, "test/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	plaintext := []byte("same-token")

	ct1, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt 1: %v", err)
	}
	ct2, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt 2: %v", err)
	}
	if bytes.Equal(ct1, ct2) {
		t.Fatal("encrypting same plaintext twice should produce different ciphertexts")
	}
}

func TestEncryptDecrypt_EmptyPlaintext(t *testing.T) {
	key, err := DeriveKey("secret", nil, "test/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	ct, err := Encrypt([]byte{}, key)
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
	got, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty plaintext, got %d bytes", len(got))
	}
}
