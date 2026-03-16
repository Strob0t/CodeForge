package crypto //nolint:revive // intentional package name for encryption utilities

import (
	"strings"
	"testing"
)

func TestGenerateRequestID(t *testing.T) {
	id := GenerateRequestID()
	if len(id) != 32 {
		t.Fatalf("expected 32 hex chars, got %d: %q", len(id), id)
	}

	// Must produce unique values.
	id2 := GenerateRequestID()
	if id == id2 {
		t.Fatal("two calls should produce different IDs")
	}
}

func TestGenerateUUIDv4(t *testing.T) {
	id := GenerateUUIDv4()

	// UUID v4 format: 8-4-4-4-12 hex chars (with dashes).
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 dash-separated parts, got %d: %q", len(parts), id)
	}

	// Version nibble (first char of 3rd segment) must be '4'.
	if parts[2][0] != '4' {
		t.Fatalf("expected version 4, got %q in %q", parts[2][0:1], id)
	}

	// Variant nibble (first char of 4th segment) must be 8, 9, a, or b.
	v := parts[3][0]
	if v != '8' && v != '9' && v != 'a' && v != 'b' {
		t.Fatalf("expected variant 10xx, got %q in %q", string(v), id)
	}

	// Uniqueness.
	id2 := GenerateUUIDv4()
	if id == id2 {
		t.Fatal("two UUIDs should differ")
	}
}

func TestGenerateRandomToken(t *testing.T) {
	tok, err := GenerateRandomToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tok) != 64 {
		t.Fatalf("expected 64 hex chars, got %d: %q", len(tok), tok)
	}

	tok2, err := GenerateRandomToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok == tok2 {
		t.Fatal("two tokens should differ")
	}
}

func TestGenerateRandomPassword(t *testing.T) {
	pw, err := GenerateRandomPassword(24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pw) != 24 {
		t.Fatalf("expected 24 chars, got %d: %q", len(pw), pw)
	}

	// First char uppercase, second lowercase, third digit.
	if pw[0] < 'A' || pw[0] > 'Z' {
		t.Fatalf("first char should be uppercase, got %q", string(pw[0]))
	}
	if pw[1] < 'a' || pw[1] > 'z' {
		t.Fatalf("second char should be lowercase, got %q", string(pw[1]))
	}
	if pw[2] < '0' || pw[2] > '9' {
		t.Fatalf("third char should be digit, got %q", string(pw[2]))
	}
}

func TestHashSHA256(t *testing.T) {
	h := HashSHA256("hello")
	expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if h != expected {
		t.Fatalf("hash mismatch:\ngot  %s\nwant %s", h, expected)
	}

	// Different input produces different hash.
	h2 := HashSHA256("world")
	if h == h2 {
		t.Fatal("different inputs should produce different hashes")
	}
}
