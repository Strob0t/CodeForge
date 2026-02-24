package vcsaccount

import (
	"bytes"
	"testing"
)

func TestDeriveKey_Deterministic(t *testing.T) {
	k1 := DeriveKey("my-secret")
	k2 := DeriveKey("my-secret")
	if len(k1) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(k1))
	}
	for i := range k1 {
		if k1[i] != k2[i] {
			t.Fatal("same input must produce same key")
		}
	}
}

func TestDeriveKey_DifferentInputs(t *testing.T) {
	k1 := DeriveKey("secret-a")
	k2 := DeriveKey("secret-b")
	same := true
	for i := range k1 {
		if k1[i] != k2[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different inputs must produce different keys")
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key := DeriveKey("test-jwt-secret")
	plaintext := []byte("ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Ciphertext must be longer than plaintext (nonce + auth tag)
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
	key1 := DeriveKey("secret-1")
	key2 := DeriveKey("secret-2")

	ct, err := Encrypt([]byte("token"), key1)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = Decrypt(ct, key2)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	key := DeriveKey("secret")
	_, err := Decrypt([]byte("short"), key)
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}

func TestEncrypt_UniqueCiphertexts(t *testing.T) {
	key := DeriveKey("secret")
	plaintext := []byte("same-token")

	ct1, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt 1: %v", err)
	}

	ct2, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt 2: %v", err)
	}

	// Due to random nonce, ciphertexts must differ
	if bytes.Equal(ct1, ct2) {
		t.Fatal("encrypting same plaintext twice should produce different ciphertexts")
	}
}
