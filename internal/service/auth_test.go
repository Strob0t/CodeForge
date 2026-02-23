package service

import (
	"context"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/user"
)

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

func TestAuthService_RegisterAndLogin(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	// Register
	u, err := svc.Register(ctx, &user.CreateRequest{
		Email:    "test@example.com",
		Name:     "Test User",
		Password: "Password123",
		Role:     user.RoleEditor,
		TenantID: "00000000-0000-0000-0000-000000000000",
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

	// Login
	resp, rawRefresh, err := svc.Login(ctx, user.LoginRequest{
		Email:    "test@example.com",
		Password: "Password123",
	}, "00000000-0000-0000-0000-000000000000")
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

	// Register
	_, err := svc.Register(ctx, &user.CreateRequest{
		Email:    "test@example.com",
		Name:     "Test",
		Password: "Password123",
		Role:     user.RoleViewer,
		TenantID: "00000000-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Wrong password
	_, _, err = svc.Login(ctx, user.LoginRequest{
		Email:    "test@example.com",
		Password: "wrongpassword",
	}, "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}

	// Non-existent user
	_, _, err = svc.Login(ctx, user.LoginRequest{
		Email:    "nobody@example.com",
		Password: "Password123",
	}, "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestAuthService_JWTSignAndVerify(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	// Register and login to get a token
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

	// Verify token
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

	// Register a user
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

	// Create API key
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

	// Validate API key
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

	// List keys
	keys, err := svc.ListAPIKeys(ctx, u.ID)
	if err != nil {
		t.Fatalf("list api keys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("got %d keys, want 1", len(keys))
	}

	// Delete key
	if err := svc.DeleteAPIKey(ctx, resp.APIKey.ID, u.ID); err != nil {
		t.Fatalf("delete api key: %v", err)
	}
}

func TestAuthService_SeedDefaultAdmin(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	ctx := context.Background()

	err := svc.SeedDefaultAdmin(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Second call should be a no-op
	err = svc.SeedDefaultAdmin(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("seed second: %v", err)
	}
}
