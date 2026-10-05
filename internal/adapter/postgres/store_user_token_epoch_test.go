package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/user"
)

// KI-143: users.token_epoch (migration 126) is read by token validation in
// the tenant the token names and raised in the caller's tenant only.
func TestStore_UserTokenEpoch(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	otherTenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	u := &user.User{
		ID:           uuid.New().String(),
		Email:        "epoch-" + uuid.New().String()[:8] + "@example.com",
		Name:         "Epoch User",
		PasswordHash: "$2a$10$dummyhashforintegrationtest000000000000000000000000",
		Role:         user.RoleEditor,
		TenantID:     tenantID,
		Enabled:      true,
	}
	if err := store.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteUser(ctx, u.ID) })

	epoch := func(want int64) {
		t.Helper()
		got, err := store.GetUserTokenEpoch(context.Background(), u.ID, tenantID)
		if err != nil {
			t.Fatalf("GetUserTokenEpoch: %v", err)
		}
		if got != want {
			t.Fatalf("token epoch = %d, want %d", got, want)
		}
		byID, err := store.GetUser(context.Background(), u.ID)
		if err != nil {
			t.Fatalf("GetUser: %v", err)
		}
		byEmail, err := store.GetUserByEmail(context.Background(), u.Email, tenantID)
		if err != nil {
			t.Fatalf("GetUserByEmail: %v", err)
		}
		if byID.TokenEpoch != want || byEmail.TokenEpoch != want {
			t.Fatalf("users read with epochs %d/%d, want %d", byID.TokenEpoch, byEmail.TokenEpoch, want)
		}
	}

	epoch(0)
	if err := store.RaiseUserTokenEpoch(ctx, u.ID); err != nil {
		t.Fatalf("RaiseUserTokenEpoch: %v", err)
	}
	epoch(1)

	// Another tenant neither reads nor raises it.
	if _, err := store.GetUserTokenEpoch(context.Background(), u.ID, otherTenantID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant read: err = %v, want ErrNotFound", err)
	}
	if err := store.RaiseUserTokenEpoch(ctxWithTenant(t, otherTenantID), u.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant raise: err = %v, want ErrNotFound", err)
	}
	epoch(1)

	// UpdateUser with a struct read before the raise keeps the epoch.
	u.Name = "Renamed"
	if err := store.UpdateUser(ctx, u); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	epoch(1)

	if err := store.DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := store.GetUserTokenEpoch(context.Background(), u.ID, tenantID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted user: err = %v, want ErrNotFound", err)
	}
}
