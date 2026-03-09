package postgres_test

import (
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/vcsaccount"
)

func TestOAuthState_CreateAndGet(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	state, err := vcsaccount.NewOAuthState("github", tenantID)
	if err != nil {
		t.Fatalf("NewOAuthState: %v", err)
	}

	if err := store.CreateOAuthState(ctx, state); err != nil {
		t.Fatalf("CreateOAuthState: %v", err)
	}

	got, err := store.GetOAuthState(ctx, state.State)
	if err != nil {
		t.Fatalf("GetOAuthState: %v", err)
	}
	if got.State != state.State {
		t.Errorf("State = %q, want %q", got.State, state.State)
	}
	if got.Provider != "github" {
		t.Errorf("Provider = %q, want %q", got.Provider, "github")
	}
	if got.TenantID != tenantID {
		t.Errorf("TenantID = %q, want %q", got.TenantID, tenantID)
	}
}

func TestOAuthState_GetNotFound(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	_, err := store.GetOAuthState(ctx, "nonexistent-state-token")
	if err == nil {
		t.Fatal("expected error for nonexistent state")
	}
}

func TestOAuthState_GetExpired(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	state := &vcsaccount.OAuthState{
		State:     "expired-state-token-aaaaaabbbbbbccccccddddddeeeeeeee",
		Provider:  "github",
		TenantID:  tenantID,
		ExpiresAt: time.Now().Add(-1 * time.Minute), // already expired
		CreatedAt: time.Now().Add(-11 * time.Minute),
	}
	if err := store.CreateOAuthState(ctx, state); err != nil {
		t.Fatalf("CreateOAuthState: %v", err)
	}

	// GetOAuthState filters by expires_at > now(), so expired states are not found.
	_, err := store.GetOAuthState(ctx, state.State)
	if err == nil {
		t.Fatal("expected error for expired state")
	}
}

func TestOAuthState_Delete(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	state, err := vcsaccount.NewOAuthState("github", tenantID)
	if err != nil {
		t.Fatalf("NewOAuthState: %v", err)
	}
	if err := store.CreateOAuthState(ctx, state); err != nil {
		t.Fatalf("CreateOAuthState: %v", err)
	}

	if err := store.DeleteOAuthState(ctx, state.State); err != nil {
		t.Fatalf("DeleteOAuthState: %v", err)
	}

	_, err = store.GetOAuthState(ctx, state.State)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestOAuthState_TenantIsolation(t *testing.T) {
	store := setupStore(t)
	tenant1 := createTestTenant(t, store)
	tenant2 := createTestTenant(t, store)
	ctx1 := ctxWithTenant(t, tenant1)
	ctx2 := ctxWithTenant(t, tenant2)

	state, err := vcsaccount.NewOAuthState("github", tenant1)
	if err != nil {
		t.Fatalf("NewOAuthState: %v", err)
	}
	if err := store.CreateOAuthState(ctx1, state); err != nil {
		t.Fatalf("CreateOAuthState: %v", err)
	}

	// Tenant 2 should not see tenant 1's state.
	_, err = store.GetOAuthState(ctx2, state.State)
	if err == nil {
		t.Fatal("expected error: tenant 2 should not see tenant 1's state")
	}
}
