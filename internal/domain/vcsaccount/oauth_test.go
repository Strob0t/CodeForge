package vcsaccount

import (
	"encoding/hex"
	"testing"
	"time"
)

func TestNewOAuthState(t *testing.T) {
	state, err := NewOAuthState("github", "tenant-1")
	if err != nil {
		t.Fatalf("NewOAuthState: %v", err)
	}

	// State must be 64 hex characters (32 bytes hex-encoded).
	if len(state.State) != 64 {
		t.Fatalf("expected state length 64, got %d", len(state.State))
	}

	// State must be valid hex.
	if _, err := hex.DecodeString(state.State); err != nil {
		t.Fatalf("state is not valid hex: %v", err)
	}

	// Provider and TenantID must be set.
	if state.Provider != "github" {
		t.Fatalf("expected provider %q, got %q", "github", state.Provider)
	}
	if state.TenantID != "tenant-1" {
		t.Fatalf("expected tenant_id %q, got %q", "tenant-1", state.TenantID)
	}

	// ExpiresAt must be roughly 10 minutes from now (allow 5 seconds tolerance).
	expectedExpiry := time.Now().Add(10 * time.Minute)
	diff := state.ExpiresAt.Sub(expectedExpiry)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Fatalf("expiry too far from expected: got %v, want ~%v", state.ExpiresAt, expectedExpiry)
	}

	// CreatedAt must be recent (within 5 seconds of now).
	sinceCreated := time.Since(state.CreatedAt)
	if sinceCreated < 0 || sinceCreated > 5*time.Second {
		t.Fatalf("created_at not recent: %v ago", sinceCreated)
	}
}

func TestNewOAuthState_Uniqueness(t *testing.T) {
	s1, err := NewOAuthState("github", "tenant-1")
	if err != nil {
		t.Fatalf("NewOAuthState 1: %v", err)
	}

	s2, err := NewOAuthState("github", "tenant-1")
	if err != nil {
		t.Fatalf("NewOAuthState 2: %v", err)
	}

	if s1.State == s2.State {
		t.Fatal("two OAuthState values must have different State strings")
	}
}

func TestOAuthState_IsExpired_Fresh(t *testing.T) {
	state, err := NewOAuthState("github", "tenant-1")
	if err != nil {
		t.Fatalf("NewOAuthState: %v", err)
	}

	if state.IsExpired() {
		t.Fatal("freshly created OAuthState should not be expired")
	}
}

func TestOAuthState_IsExpired_Past(t *testing.T) {
	state := OAuthState{
		State:     "deadbeef",
		Provider:  "github",
		TenantID:  "tenant-1",
		ExpiresAt: time.Now().Add(-1 * time.Minute),
		CreatedAt: time.Now().Add(-11 * time.Minute),
	}

	if !state.IsExpired() {
		t.Fatal("OAuthState with ExpiresAt in the past should be expired")
	}
}
