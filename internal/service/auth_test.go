package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/user"
)

const testTenantID = "00000000-0000-0000-0000-000000000000"

func newTestAuthService(store *mockStore) *AuthService {
	cfg := config.Auth{
		Enabled:            true,
		JWTSecret:          "test-secret-key-must-be-long-enough",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         4, // low cost for fast tests
		DefaultAdminEmail:  "admin@test.com",
		DefaultAdminPass:   "Adminpass123",
	}
	return NewAuthService(store, &cfg)
}

// registerAndLogin is a test helper that registers a user and logs them in,
// returning the user, login response, and raw refresh token.
func registerAndLogin(t *testing.T, svc *AuthService, email, password string) (*user.User, *user.LoginResponse, string) { //nolint:unparam // password varies per test but linter sees constant "Password123"
	t.Helper()
	ctx := context.Background()

	u, err := svc.Register(ctx, &user.CreateRequest{
		Email:    email,
		Name:     "Test User",
		Password: password,
		Role:     user.RoleEditor,
		TenantID: testTenantID,
	})
	if err != nil {
		t.Fatalf("register %s: %v", email, err)
	}

	resp, rawRefresh, err := svc.Login(ctx, user.LoginRequest{
		Email:    email,
		Password: password,
	}, testTenantID)
	if err != nil {
		t.Fatalf("login %s: %v", email, err)
	}

	return u, resp, rawRefresh
}

// --- Existing tests (preserved) ---

func TestAuthService_RegisterAndLogin(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	u, err := svc.Register(ctx, &user.CreateRequest{
		Email:    "test@example.com",
		Name:     "Test User",
		Password: "Password123",
		Role:     user.RoleEditor,
		TenantID: testTenantID,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if u.Email != "test@example.com" {
		t.Errorf("email = %q, want test@example.com", u.Email)
	}
	if u.Role != user.RoleEditor {
		t.Errorf("role = %q, want editor", u.Role)
	}

	resp, rawRefresh, err := svc.Login(ctx, user.LoginRequest{
		Email:    "test@example.com",
		Password: "Password123",
	}, testTenantID)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("access token is empty")
	}
	if rawRefresh == "" {
		t.Error("refresh token is empty")
	}
	if resp.User.Email != "test@example.com" {
		t.Errorf("user email = %q, want test@example.com", resp.User.Email)
	}
}

func TestAuthService_InvalidLogin(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	_, err := svc.Register(ctx, &user.CreateRequest{
		Email:    "test@example.com",
		Name:     "Test",
		Password: "Password123",
		Role:     user.RoleViewer,
		TenantID: testTenantID,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	_, _, err = svc.Login(ctx, user.LoginRequest{
		Email:    "test@example.com",
		Password: "wrongpassword",
	}, testTenantID)
	if err == nil {
		t.Fatal("expected error for wrong password")
	}

	_, _, err = svc.Login(ctx, user.LoginRequest{
		Email:    "nobody@example.com",
		Password: "Password123",
	}, testTenantID)
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestAuthService_JWTSignAndVerify(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	_, err := svc.Register(ctx, &user.CreateRequest{
		Email:    "jwt@test.com",
		Name:     "JWT User",
		Password: "Jwtpass1234",
		Role:     user.RoleAdmin,
		TenantID: "tid-1",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	resp, _, err := svc.Login(ctx, user.LoginRequest{
		Email:    "jwt@test.com",
		Password: "Jwtpass1234",
	}, "tid-1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	claims, err := svc.ValidateAccessToken(resp.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.Email != "jwt@test.com" {
		t.Errorf("email = %q, want jwt@test.com", claims.Email)
	}
	if claims.Role != user.RoleAdmin {
		t.Errorf("role = %q, want admin", claims.Role)
	}
	if claims.TenantID != "tid-1" {
		t.Errorf("tenant = %q, want tid-1", claims.TenantID)
	}
}

func TestAuthService_InvalidToken(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)

	_, err := svc.ValidateAccessToken("garbage.token.here")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}

	_, err = svc.ValidateAccessToken("not-even-three-parts")
	if err == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestAuthService_APIKey(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	u, err := svc.Register(ctx, &user.CreateRequest{
		Email:    "apikey@test.com",
		Name:     "API Key User",
		Password: "Password123",
		Role:     user.RoleEditor,
		TenantID: "tid-1",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	resp, err := svc.CreateAPIKey(ctx, u.ID, user.CreateAPIKeyRequest{Name: "ci-key"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if resp.PlainKey == "" {
		t.Error("plain key is empty")
	}
	if resp.APIKey.Name != "ci-key" {
		t.Errorf("name = %q, want ci-key", resp.APIKey.Name)
	}

	validatedUser, validatedKey, err := svc.ValidateAPIKey(ctx, resp.PlainKey)
	if err != nil {
		t.Fatalf("validate api key: %v", err)
	}
	if validatedUser.ID != u.ID {
		t.Errorf("user id = %q, want %q", validatedUser.ID, u.ID)
	}
	if validatedKey.Name != "ci-key" {
		t.Errorf("api key name = %q, want ci-key", validatedKey.Name)
	}

	keys, err := svc.ListAPIKeys(ctx, u.ID)
	if err != nil {
		t.Fatalf("list api keys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("got %d keys, want 1", len(keys))
	}

	if err := svc.DeleteAPIKey(ctx, resp.APIKey.ID, u.ID); err != nil {
		t.Fatalf("delete api key: %v", err)
	}
}

func TestAuthService_SeedDefaultAdmin(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	err := svc.SeedDefaultAdmin(ctx, testTenantID)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	err = svc.SeedDefaultAdmin(ctx, testTenantID)
	if err != nil {
		t.Fatalf("seed second: %v", err)
	}
}

// --- Priority 1: Security-critical tests ---

func TestAuthService_RefreshTokens(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	_, resp, rawRefresh := registerAndLogin(t, svc, "refresh@test.com", "Password123")
	oldAccessToken := resp.AccessToken

	// Refresh: should get new access + refresh tokens
	newResp, newRawRefresh, err := svc.RefreshTokens(ctx, rawRefresh)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if newResp.AccessToken == "" {
		t.Error("new access token is empty")
	}
	if newResp.AccessToken == oldAccessToken {
		t.Error("new access token should differ from old")
	}
	if newRawRefresh == "" {
		t.Error("new refresh token is empty")
	}
	if newRawRefresh == rawRefresh {
		t.Error("new refresh token should differ from old (rotation)")
	}

	// Old refresh token should be invalidated
	_, _, err = svc.RefreshTokens(ctx, rawRefresh)
	if err == nil {
		t.Fatal("expected error for old refresh token after rotation")
	}
}

func TestAuthService_RefreshTokens_Expired(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)

	_, _, rawRefresh := registerAndLogin(t, svc, "expire@test.com", "Password123")

	// Manually expire the refresh token in the store
	for i := range store.refreshTokens {
		store.refreshTokens[i].ExpiresAt = time.Now().Add(-1 * time.Hour)
	}

	_, _, err := svc.RefreshTokens(context.Background(), rawRefresh)
	if err == nil {
		t.Fatal("expected error for expired refresh token")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error = %q, want to contain 'expired'", err.Error())
	}
}

func TestAuthService_RefreshTokens_DisabledAccount(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)

	u, _, rawRefresh := registerAndLogin(t, svc, "disabled@test.com", "Password123")

	// Disable the account
	u.Enabled = false
	if err := store.UpdateUser(context.Background(), u); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	_, _, err := svc.RefreshTokens(context.Background(), rawRefresh)
	if err == nil {
		t.Fatal("expected error for disabled account refresh")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("error = %q, want to contain 'disabled'", err.Error())
	}
}

func TestAuthService_RefreshTokens_InvalidToken(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)

	_, _, err := svc.RefreshTokens(context.Background(), "nonexistent-token")
	if err == nil {
		t.Fatal("expected error for unknown refresh token")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error = %q, want to contain 'invalid'", err.Error())
	}
}

func TestAuthService_Logout(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	u, resp, _ := registerAndLogin(t, svc, "logout@test.com", "Password123")

	// Parse the access token to get the JTI
	claims, err := svc.ValidateAccessToken(resp.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	tokenExpiry := time.Unix(claims.Expiry, 0)
	if err := svc.Logout(ctx, u.ID, claims.JTI, tokenExpiry); err != nil {
		t.Fatalf("logout: %v", err)
	}

	// Access token should be revoked
	_, err = svc.ValidateAccessToken(resp.AccessToken)
	if err == nil {
		t.Fatal("expected error for revoked access token")
	}
	if !strings.Contains(err.Error(), "revoked") {
		t.Errorf("error = %q, want to contain 'revoked'", err.Error())
	}

	// Refresh tokens should be cleared
	if len(store.refreshTokens) != 0 {
		t.Errorf("expected 0 refresh tokens after logout, got %d", len(store.refreshTokens))
	}
}

func TestAuthService_Logout_EmptyJTI(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)

	u, _, _ := registerAndLogin(t, svc, "logoutjti@test.com", "Password123")

	// Logout with empty JTI: should clear refresh tokens without revocation entry
	if err := svc.Logout(context.Background(), u.ID, "", time.Time{}); err != nil {
		t.Fatalf("logout: %v", err)
	}

	if len(store.refreshTokens) != 0 {
		t.Errorf("expected 0 refresh tokens, got %d", len(store.refreshTokens))
	}
	if len(store.revokedTokens) != 0 {
		t.Errorf("expected 0 revoked tokens with empty JTI, got %d", len(store.revokedTokens))
	}
}

func TestAuthService_ValidateAccessToken_Revoked(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	_, resp, _ := registerAndLogin(t, svc, "revoke@test.com", "Password123")

	claims, err := svc.ValidateAccessToken(resp.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	// Revoke the token
	if err := svc.RevokeAccessToken(ctx, claims.JTI, time.Unix(claims.Expiry, 0)); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// Validation should now fail
	_, err = svc.ValidateAccessToken(resp.AccessToken)
	if err == nil {
		t.Fatal("expected error for revoked token")
	}
	if !strings.Contains(err.Error(), "revoked") {
		t.Errorf("error = %q, want to contain 'revoked'", err.Error())
	}
}

func TestAuthService_ValidateAccessToken_DBError_FailClosed(t *testing.T) {
	store := &mockStore{
		isTokenRevokedErr: errors.New("database connection lost"),
	}
	svc := newTestAuthService(store)

	_, resp, _ := registerAndLogin(t, svc, "failclosed@test.com", "Password123")

	// Validation should fail-closed when DB is down
	_, err := svc.ValidateAccessToken(resp.AccessToken)
	if err == nil {
		t.Fatal("expected error when DB check fails (fail-closed)")
	}
	if !strings.Contains(err.Error(), "unable to verify") {
		t.Errorf("error = %q, want to contain 'unable to verify'", err.Error())
	}
}

func TestAuthService_AccountLockout(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	registerAndLogin(t, svc, "lockout@test.com", "Password123")

	// Fail 5 times to trigger lockout
	for i := 0; i < user.MaxFailedAttempts; i++ {
		_, _, err := svc.Login(ctx, user.LoginRequest{
			Email:    "lockout@test.com",
			Password: "WrongPass123",
		}, testTenantID)
		if err == nil {
			t.Fatalf("attempt %d: expected error for wrong password", i+1)
		}
	}

	// Account should be locked — even correct password should fail
	_, _, err := svc.Login(ctx, user.LoginRequest{
		Email:    "lockout@test.com",
		Password: "Password123",
	}, testTenantID)
	if err == nil {
		t.Fatal("expected error for locked account")
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Errorf("error = %q, want to contain 'locked'", err.Error())
	}

	// Simulate lockout expiry
	for i := range store.users {
		if store.users[i].Email == "lockout@test.com" {
			store.users[i].LockedUntil = time.Now().Add(-1 * time.Minute)
			break
		}
	}

	// Should succeed after lockout expires
	_, _, err = svc.Login(ctx, user.LoginRequest{
		Email:    "lockout@test.com",
		Password: "Password123",
	}, testTenantID)
	if err != nil {
		t.Fatalf("expected success after lockout expiry, got: %v", err)
	}
}

func TestAuthService_DisabledAccount(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)

	u, _, _ := registerAndLogin(t, svc, "disabled2@test.com", "Password123")

	u.Enabled = false
	if err := store.UpdateUser(context.Background(), u); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	_, _, err := svc.Login(context.Background(), user.LoginRequest{
		Email:    "disabled2@test.com",
		Password: "Password123",
	}, testTenantID)
	if err == nil {
		t.Fatal("expected error for disabled account login")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("error = %q, want to contain 'disabled'", err.Error())
	}
}

func TestAuthService_ChangePassword(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	u, _, _ := registerAndLogin(t, svc, "changepw@test.com", "Password123")

	// Set MustChangePassword to verify it gets cleared
	u.MustChangePassword = true
	if err := store.UpdateUser(ctx, u); err != nil {
		t.Fatalf("set must_change: %v", err)
	}

	// Change password
	err := svc.ChangePassword(ctx, u.ID, user.ChangePasswordRequest{
		OldPassword: "Password123",
		NewPassword: "NewSecure456",
	})
	if err != nil {
		t.Fatalf("change password: %v", err)
	}

	// MustChangePassword should be cleared
	updated, _ := store.GetUser(ctx, u.ID)
	if updated.MustChangePassword {
		t.Error("MustChangePassword should be false after change")
	}

	// Login with new password should work
	_, _, err = svc.Login(ctx, user.LoginRequest{
		Email:    "changepw@test.com",
		Password: "NewSecure456",
	}, testTenantID)
	if err != nil {
		t.Fatalf("login with new password: %v", err)
	}

	// Login with old password should fail
	_, _, err = svc.Login(ctx, user.LoginRequest{
		Email:    "changepw@test.com",
		Password: "Password123",
	}, testTenantID)
	if err == nil {
		t.Fatal("expected error for old password after change")
	}
}

func TestAuthService_ChangePassword_WrongOld(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)

	u, _, _ := registerAndLogin(t, svc, "wrongold@test.com", "Password123")

	err := svc.ChangePassword(context.Background(), u.ID, user.ChangePasswordRequest{
		OldPassword: "WrongOldPass1",
		NewPassword: "NewSecure456",
	})
	if err == nil {
		t.Fatal("expected error for wrong old password")
	}
	if !strings.Contains(err.Error(), "incorrect") {
		t.Errorf("error = %q, want to contain 'incorrect'", err.Error())
	}
}

func TestAuthService_AdminResetPassword(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	u, _, _ := registerAndLogin(t, svc, "adminreset@test.com", "Password123")

	// Lock the account and add failed attempts
	u.FailedAttempts = 5
	u.LockedUntil = time.Now().Add(15 * time.Minute)
	u.MustChangePassword = true
	if err := store.UpdateUser(ctx, u); err != nil {
		t.Fatalf("update user: %v", err)
	}

	// Admin reset
	err := svc.AdminResetPassword(ctx, "adminreset@test.com", testTenantID, "ResetPass1234")
	if err != nil {
		t.Fatalf("admin reset: %v", err)
	}

	// Verify lockout cleared
	updated, _ := store.GetUser(ctx, u.ID)
	if updated.FailedAttempts != 0 {
		t.Errorf("failed attempts = %d, want 0", updated.FailedAttempts)
	}
	if !updated.LockedUntil.IsZero() {
		t.Error("locked_until should be zero after admin reset")
	}
	if updated.MustChangePassword {
		t.Error("must_change_password should be false after admin reset")
	}

	// Sessions should be invalidated (refresh tokens cleared) — check before new login
	for _, rt := range store.refreshTokens {
		if rt.UserID == u.ID {
			t.Error("refresh tokens should be cleared after admin reset")
			break
		}
	}

	// Login with new password should work
	_, _, err = svc.Login(ctx, user.LoginRequest{
		Email:    "adminreset@test.com",
		Password: "ResetPass1234",
	}, testTenantID)
	if err != nil {
		t.Fatalf("login after admin reset: %v", err)
	}
}

func TestAuthService_AdminResetPassword_WeakPassword(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)

	registerAndLogin(t, svc, "weakadmin@test.com", "Password123")

	err := svc.AdminResetPassword(context.Background(), "weakadmin@test.com", testTenantID, "short")
	if err == nil {
		t.Fatal("expected error for weak password")
	}
}

func TestAuthService_RequestPasswordReset(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	registerAndLogin(t, svc, "resetreq@test.com", "Password123")

	token, err := svc.RequestPasswordReset(ctx, "resetreq@test.com", testTenantID)
	if err != nil {
		t.Fatalf("request reset: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty reset token")
	}

	// Token should be stored
	if len(store.passwordResetTokens) != 1 {
		t.Fatalf("expected 1 password reset token, got %d", len(store.passwordResetTokens))
	}
}

func TestAuthService_RequestPasswordReset_UnknownEmail(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)

	// Unknown email should return empty token and nil error (enumeration prevention)
	token, err := svc.RequestPasswordReset(context.Background(), "unknown@test.com", testTenantID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token for unknown email, got %q", token)
	}
}

func TestAuthService_ConfirmPasswordReset(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	u, _, _ := registerAndLogin(t, svc, "confirmreset@test.com", "Password123")

	rawToken, err := svc.RequestPasswordReset(ctx, "confirmreset@test.com", testTenantID)
	if err != nil {
		t.Fatalf("request reset: %v", err)
	}

	// Confirm the reset
	err = svc.ConfirmPasswordReset(ctx, rawToken, "NewReset12345")
	if err != nil {
		t.Fatalf("confirm reset: %v", err)
	}

	// Token should be marked as used
	for _, prt := range store.passwordResetTokens {
		if prt.UserID == u.ID && !prt.Used {
			t.Error("password reset token should be marked as used")
		}
	}

	// Sessions should be invalidated — check before new login
	for _, rt := range store.refreshTokens {
		if rt.UserID == u.ID {
			t.Error("refresh tokens should be cleared after password reset")
			break
		}
	}

	// Login with new password should work
	_, _, err = svc.Login(ctx, user.LoginRequest{
		Email:    "confirmreset@test.com",
		Password: "NewReset12345",
	}, testTenantID)
	if err != nil {
		t.Fatalf("login after reset: %v", err)
	}
}

func TestAuthService_ConfirmPasswordReset_UsedToken(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	registerAndLogin(t, svc, "usedtoken@test.com", "Password123")

	rawToken, err := svc.RequestPasswordReset(ctx, "usedtoken@test.com", testTenantID)
	if err != nil {
		t.Fatalf("request reset: %v", err)
	}

	// Use the token once
	if err := svc.ConfirmPasswordReset(ctx, rawToken, "NewReset12345"); err != nil {
		t.Fatalf("first confirm: %v", err)
	}

	// Second use should fail
	err = svc.ConfirmPasswordReset(ctx, rawToken, "AnotherPass123")
	if err == nil {
		t.Fatal("expected error for already-used token")
	}
	if !strings.Contains(err.Error(), "already been used") {
		t.Errorf("error = %q, want to contain 'already been used'", err.Error())
	}
}

// --- Priority 2: Edge cases ---

func TestAuthService_ConfirmPasswordReset_ExpiredToken(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	registerAndLogin(t, svc, "expiredprt@test.com", "Password123")

	rawToken, err := svc.RequestPasswordReset(ctx, "expiredprt@test.com", testTenantID)
	if err != nil {
		t.Fatalf("request reset: %v", err)
	}

	// Manually expire the token
	for i := range store.passwordResetTokens {
		store.passwordResetTokens[i].ExpiresAt = time.Now().Add(-1 * time.Hour)
	}

	err = svc.ConfirmPasswordReset(ctx, rawToken, "NewReset12345")
	if err == nil {
		t.Fatal("expected error for expired reset token")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error = %q, want to contain 'expired'", err.Error())
	}
}

func TestAuthService_ConfirmPasswordReset_InvalidToken(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)

	err := svc.ConfirmPasswordReset(context.Background(), "nonexistent-token", "NewReset12345")
	if err == nil {
		t.Fatal("expected error for unknown reset token")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error = %q, want to contain 'invalid'", err.Error())
	}
}

func TestAuthService_ValidateAPIKey_Expired(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	u, _, _ := registerAndLogin(t, svc, "expiredkey@test.com", "Password123")

	// Create API key with expiry
	resp, err := svc.CreateAPIKey(ctx, u.ID, user.CreateAPIKeyRequest{
		Name:      "exp-key",
		ExpiresIn: 1, // 1 second
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Manually expire the key
	for i := range store.apiKeys {
		if store.apiKeys[i].ID == resp.APIKey.ID {
			store.apiKeys[i].ExpiresAt = time.Now().Add(-1 * time.Hour)
		}
	}

	_, _, err = svc.ValidateAPIKey(ctx, resp.PlainKey)
	if err == nil {
		t.Fatal("expected error for expired API key")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error = %q, want to contain 'expired'", err.Error())
	}
}

func TestAuthService_GetSetupStatus(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	// No users: needs setup
	status, err := svc.GetSetupStatus(ctx, testTenantID)
	if err != nil {
		t.Fatalf("get setup status: %v", err)
	}
	if !status.NeedsSetup {
		t.Error("expected NeedsSetup=true with no users")
	}

	// Register a user
	registerAndLogin(t, svc, "setup@test.com", "Password123")

	// With users: no setup needed
	status, err = svc.GetSetupStatus(ctx, testTenantID)
	if err != nil {
		t.Fatalf("get setup status: %v", err)
	}
	if status.NeedsSetup {
		t.Error("expected NeedsSetup=false with existing users")
	}
}

func TestAuthService_BootstrapAdmin_AutoGenerate(t *testing.T) {
	tmpDir := t.TempDir()
	pwFile := filepath.Join(tmpDir, "initial_password")

	store := &mockStore{}
	cfg := config.Auth{
		Enabled:                     true,
		JWTSecret:                   "test-secret-key-must-be-long-enough",
		AccessTokenExpiry:           15 * time.Minute,
		RefreshTokenExpiry:          7 * 24 * time.Hour,
		BcryptCost:                  4,
		DefaultAdminEmail:           "admin@autogen.com",
		AutoGenerateInitialPassword: true,
		InitialPasswordFile:         pwFile,
	}
	svc := NewAuthService(store, &cfg)
	ctx := context.Background()

	if err := svc.BootstrapAdmin(ctx, testTenantID); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	// Password file should exist
	data, err := os.ReadFile(pwFile) //nolint:gosec // test file path from t.TempDir()
	if err != nil {
		t.Fatalf("read password file: %v", err)
	}
	password := strings.TrimSpace(string(data))
	if password == "" {
		t.Fatal("password file is empty")
	}

	// User should have MustChangePassword set
	if len(store.users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(store.users))
	}
	if !store.users[0].MustChangePassword {
		t.Error("expected MustChangePassword=true for auto-generated password")
	}

	// Login with generated password should work
	_, _, err = svc.Login(ctx, user.LoginRequest{
		Email:    "admin@autogen.com",
		Password: password,
	}, testTenantID)
	if err != nil {
		t.Fatalf("login with generated password: %v", err)
	}
}

func TestAuthService_BootstrapAdmin_WizardWait(t *testing.T) {
	store := &mockStore{}
	cfg := config.Auth{
		Enabled:            true,
		JWTSecret:          "test-secret-key-must-be-long-enough",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         4,
		DefaultAdminEmail:  "admin@wizard.com",
		// No password configured, AutoGenerate=false
	}
	svc := NewAuthService(store, &cfg)

	if err := svc.BootstrapAdmin(context.Background(), testTenantID); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	// No user should be created
	if len(store.users) != 0 {
		t.Fatalf("expected 0 users (wizard wait), got %d", len(store.users))
	}
}

func TestAuthService_SyncAdminPassword(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	// Bootstrap the admin first
	if err := svc.BootstrapAdmin(ctx, testTenantID); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	// Change the configured password
	svc2 := NewAuthService(store, &config.Auth{
		Enabled:            true,
		JWTSecret:          "test-secret-key-must-be-long-enough",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         4,
		DefaultAdminEmail:  "admin@test.com",
		DefaultAdminPass:   "NewAdmin12345",
	})

	// BootstrapAdmin should sync the password since admin already exists
	if err := svc2.BootstrapAdmin(ctx, testTenantID); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Login with the new password should work
	_, _, err := svc2.Login(ctx, user.LoginRequest{
		Email:    "admin@test.com",
		Password: "NewAdmin12345",
	}, testTenantID)
	if err != nil {
		t.Fatalf("login with synced password: %v", err)
	}
}

func TestAuthService_UpdateUser(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	u, _, _ := registerAndLogin(t, svc, "update@test.com", "Password123")

	// Update name and role
	enabled := false
	updated, err := svc.UpdateUser(ctx, u.ID, user.UpdateRequest{
		Name:    "New Name",
		Role:    user.RoleAdmin,
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("update user: %v", err)
	}
	if updated.Name != "New Name" {
		t.Errorf("name = %q, want 'New Name'", updated.Name)
	}
	if updated.Role != user.RoleAdmin {
		t.Errorf("role = %q, want admin", updated.Role)
	}
	if updated.Enabled {
		t.Error("expected Enabled=false")
	}

	// Invalid role should fail
	_, err = svc.UpdateUser(ctx, u.ID, user.UpdateRequest{Role: "superadmin"})
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
	if !strings.Contains(err.Error(), "invalid role") {
		t.Errorf("error = %q, want to contain 'invalid role'", err.Error())
	}
}

// --- Priority 3: Login edge cases ---

func TestAuthService_Login_ResetsFailedAttempts(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	registerAndLogin(t, svc, "resetfail@test.com", "Password123")

	// Fail a few times (but not enough to lock)
	for i := 0; i < 3; i++ {
		_, _, _ = svc.Login(ctx, user.LoginRequest{
			Email:    "resetfail@test.com",
			Password: "WrongPass123",
		}, testTenantID)
	}

	// Verify failed attempts > 0
	for _, u := range store.users {
		if u.Email == "resetfail@test.com" && u.FailedAttempts == 0 {
			t.Fatal("expected non-zero failed attempts")
		}
	}

	// Successful login should reset the counter
	_, _, err := svc.Login(ctx, user.LoginRequest{
		Email:    "resetfail@test.com",
		Password: "Password123",
	}, testTenantID)
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	for _, u := range store.users {
		if u.Email == "resetfail@test.com" && u.FailedAttempts != 0 {
			t.Errorf("failed attempts = %d, want 0 after successful login", u.FailedAttempts)
		}
	}
}

func TestAuthService_Register_DefaultRole(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)

	u, err := svc.Register(context.Background(), &user.CreateRequest{
		Email:    "norole@test.com",
		Name:     "No Role",
		Password: "Password123",
		TenantID: testTenantID,
		// Role left empty
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if u.Role != user.RoleViewer {
		t.Errorf("role = %q, want viewer (default)", u.Role)
	}
}

func TestAuthService_DeleteUser(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	u, _, _ := registerAndLogin(t, svc, "delete@test.com", "Password123")

	if err := svc.DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	_, err := svc.GetUser(ctx, u.ID)
	if err == nil {
		t.Fatal("expected error after deleting user")
	}
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	// First registration succeeds.
	_, err := svc.Register(ctx, &user.CreateRequest{
		Email:    "dupe@test.com",
		Name:     "First",
		Password: "Password123",
		Role:     user.RoleEditor,
		TenantID: testTenantID,
	})
	if err != nil {
		t.Fatalf("first register: %v", err)
	}

	// Same email + same tenant should fail.
	_, err = svc.Register(ctx, &user.CreateRequest{
		Email:    "dupe@test.com",
		Name:     "Second",
		Password: "Password123",
		Role:     user.RoleViewer,
		TenantID: testTenantID,
	})
	if err == nil {
		t.Fatal("expected error for duplicate email in same tenant")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %q, want to contain 'duplicate'", err.Error())
	}

	// Same email in a different tenant should succeed (multi-tenant isolation).
	_, err = svc.Register(ctx, &user.CreateRequest{
		Email:    "dupe@test.com",
		Name:     "Other Tenant",
		Password: "Password123",
		Role:     user.RoleEditor,
		TenantID: "different-tenant-id",
	})
	if err != nil {
		t.Fatalf("register in different tenant: %v", err)
	}
}

func TestAuthService_ChangePassword_WeakNew(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)

	u, _, _ := registerAndLogin(t, svc, "weaknew@test.com", "Password123")

	err := svc.ChangePassword(context.Background(), u.ID, user.ChangePasswordRequest{
		OldPassword: "Password123",
		NewPassword: "short",
	})
	if err == nil {
		t.Fatal("expected error for weak new password")
	}
}
