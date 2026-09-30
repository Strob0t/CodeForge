package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/vcsaccount"
)

func TestOAuthState_CreateAndConsume(t *testing.T) {
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

	got, err := store.ConsumeOAuthState(ctx, state.State)
	if err != nil {
		t.Fatalf("ConsumeOAuthState: %v", err)
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

	// Single use.
	if _, err := store.ConsumeOAuthState(ctx, state.State); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second ConsumeOAuthState = %v, want ErrNotFound", err)
	}
}

func TestOAuthState_ConsumeNotFound(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	_, err := store.ConsumeOAuthState(ctx, "nonexistent-state-token")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ConsumeOAuthState = %v, want ErrNotFound", err)
	}
}

func TestOAuthState_ConsumeExpired(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	// A random state token keeps the test independent of rows left in a shared
	// database by earlier runs (state is the primary key).
	state, err := vcsaccount.NewOAuthState("github", tenantID)
	if err != nil {
		t.Fatalf("NewOAuthState: %v", err)
	}
	state.ExpiresAt = time.Now().Add(-1 * time.Minute) // already expired
	state.CreatedAt = time.Now().Add(-11 * time.Minute)
	if err := store.CreateOAuthState(ctx, state); err != nil {
		t.Fatalf("CreateOAuthState: %v", err)
	}

	// Expired states are not returned.
	if _, err := store.ConsumeOAuthState(ctx, state.State); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ConsumeOAuthState = %v, want ErrNotFound", err)
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

	if _, err := store.ConsumeOAuthState(ctx, state.State); err == nil {
		t.Fatal("expected error after delete")
	}
}

// KI-55: the callback request has no session (default tenant); consuming
// the state returns the tenant that started the flow.
func TestOAuthState_ConsumeReturnsTheStartingTenant(t *testing.T) {
	store := setupStore(t)
	tenant1 := createTestTenant(t, store)
	ctx1 := ctxWithTenant(t, tenant1)

	state, err := vcsaccount.NewOAuthState("github", tenant1)
	if err != nil {
		t.Fatalf("NewOAuthState: %v", err)
	}
	if err := store.CreateOAuthState(ctx1, state); err != nil {
		t.Fatalf("CreateOAuthState: %v", err)
	}

	got, err := store.ConsumeOAuthState(context.Background(), state.State)
	if err != nil {
		t.Fatalf("ConsumeOAuthState without a tenant: %v", err)
	}
	if got.TenantID != tenant1 {
		t.Fatalf("TenantID = %q, want %q", got.TenantID, tenant1)
	}
}

func TestConsumeOAuthState_IntentionallyCrossTenant(t *testing.T) {
	src := readStoreSource(t, "store_oauth_state.go")
	if !strings.Contains(src, "INTENTIONALLY CROSS-TENANT") {
		t.Fatal("ConsumeOAuthState must document why it is cross-tenant")
	}
}
