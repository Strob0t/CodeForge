package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const gdprOtherTenant = "11111111-2222-3333-4444-555555555555"

// recordingInvalidator records whose sessions were ended.
type recordingInvalidator struct{ ended []string }

func (r *recordingInvalidator) EndUserSessions(userID string) { r.ended = append(r.ended, userID) }

// KI-176: the admin export reads only users of the caller's tenant; another
// tenant's user is not found.
func TestExportUserData_OtherTenantIsNotFound(t *testing.T) {
	store := &gdprMockStore{user: &user.User{ID: "u2", Email: "b@example.com", TenantID: gdprOtherTenant}}
	svc := NewGDPRService(store)

	_, err := svc.ExportUserData(tenantctx.WithTenant(context.Background(), tenantctx.DefaultTenantID), "u2")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, err := svc.ExportUserData(tenantctx.WithTenant(context.Background(), gdprOtherTenant), "u2"); err != nil {
		t.Fatalf("own tenant: %v", err)
	}
}

// An erasure of another tenant's user runs no step and ends no session;
// sessions end only once the user is erased.
func TestDeleteUserData_OtherTenantHasNoSideEffect(t *testing.T) {
	store := &gdprMockStore{user: &user.User{ID: "u2", TenantID: gdprOtherTenant}}
	svc := NewGDPRService(store)
	inv := &recordingInvalidator{}
	svc.SetTokenInvalidator(inv)

	err := svc.DeleteUserData(tenantctx.WithTenant(context.Background(), tenantctx.DefaultTenantID), "u2")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if len(store.steps) != 0 || len(inv.ended) != 0 {
		t.Fatalf("steps = %v, sessions ended = %v: want nothing", store.steps, inv.ended)
	}

	if err := svc.DeleteUserData(tenantctx.WithTenant(context.Background(), gdprOtherTenant), "u2"); err != nil {
		t.Fatalf("own tenant: %v", err)
	}
	if !store.deleteCalled || len(inv.ended) != 1 || inv.ended[0] != "u2" {
		t.Fatalf("deleted = %v, sessions ended = %v", store.deleteCalled, inv.ended)
	}
}

// A failed erasure keeps the user's sessions too.
func TestDeleteUserData_FailedErasureKeepsSessions(t *testing.T) {
	store := &gdprMockStore{user: &user.User{ID: "u1", TenantID: tenantctx.DefaultTenantID}, deleteUserErr: errors.New("fk violation")}
	svc := NewGDPRService(store)
	inv := &recordingInvalidator{}
	svc.SetTokenInvalidator(inv)
	if err := svc.DeleteUserData(context.Background(), "u1"); err == nil {
		t.Fatal("expected the delete error")
	}
	if len(inv.ended) != 0 {
		t.Fatalf("sessions ended = %v, want none", inv.ended)
	}
}

// DELETE /users/{id} through the AuthService: a foreign user's WebSocket
// connections stay open (nothing was deleted), an erased user's are closed.
func TestAuthDeleteUser_EndsSessionsOnlyAfterTheErasure(t *testing.T) {
	store := &gdprMockStore{user: &user.User{ID: "u2", TenantID: gdprOtherTenant}}
	svc := NewAuthService(store, &config.Auth{JWTSecret: "test-secret-key-must-be-long-enough"})
	dropper := &recordingDropper{}
	svc.SetConnectionDropper(dropper)

	err := svc.DeleteUser(tenantctx.WithTenant(context.Background(), tenantctx.DefaultTenantID), "u2")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if len(dropper.users) != 0 || store.deleteCalled {
		t.Fatalf("dropped = %v, deleted = %v: want nothing", dropper.users, store.deleteCalled)
	}
	if err := svc.DeleteUser(tenantctx.WithTenant(context.Background(), gdprOtherTenant), "u2"); err != nil {
		t.Fatalf("own tenant: %v", err)
	}
	if len(dropper.users) != 1 || dropper.users[0] != "u2" {
		t.Fatalf("dropped = %v, want u2", dropper.users)
	}
}
