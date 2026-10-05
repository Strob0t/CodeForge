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
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/user"
)

// KI-119: the first-admin setup needs the one-time token the Core creates
// on a first start without users (log line and <data>/setup_token).

const setupOtherTenantID = "11111111-1111-1111-1111-111111111111"

func newSetupTestService(t *testing.T, store *mockStore) (svc *AuthService, tokenFile string) {
	t.Helper()
	tokenFile = filepath.Join(t.TempDir(), "data", "setup_token")
	cfg := config.Auth{
		Enabled:            true,
		JWTSecret:          "test-secret-key-must-be-long-enough",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         4,
		DefaultAdminEmail:  "admin@test.com",
		SetupTokenFile:     tokenFile,
	}
	return NewAuthService(store, &cfg), tokenFile
}

func setupRequest(tenantID string) *user.CreateRequest {
	return &user.CreateRequest{
		Email:    "first@test.com",
		Name:     "First Admin",
		Password: "Password123",
		Role:     user.RoleAdmin,
		TenantID: tenantID,
	}
}

func TestPrepareSetupToken_NoUsers_WritesTokenFile(t *testing.T) {
	svc, tokenFile := newSetupTestService(t, &mockStore{})

	token, err := svc.PrepareSetupToken(context.Background(), testTenantID)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if len(token) < 32 {
		t.Fatalf("token %q is too short", token)
	}
	data, err := os.ReadFile(tokenFile) //nolint:gosec // test path from t.TempDir()
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if strings.TrimSpace(string(data)) != token {
		t.Fatalf("token file holds %q, want the returned token", data)
	}
	info, err := os.Stat(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("token file mode = %o, want 600", perm)
	}

	again, err := svc.PrepareSetupToken(context.Background(), testTenantID)
	if err != nil {
		t.Fatalf("prepare again: %v", err)
	}
	if again == token {
		t.Fatal("a new start must create a new token")
	}
	if _, err := svc.CompleteSetup(context.Background(), setupRequest(testTenantID), token); !errors.Is(err, user.ErrInvalidSetupToken) {
		t.Fatalf("the replaced token must be refused, got %v", err)
	}
}

func TestPrepareSetupToken_ReplacesLooseFilePermissions(t *testing.T) {
	svc, tokenFile := newSetupTestService(t, &mockStore{})
	if err := os.MkdirAll(filepath.Dir(tokenFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenFile, []byte("old\n"), 0o644); err != nil { //nolint:gosec // the loose mode is the test
		t.Fatal(err)
	}

	if _, err := svc.PrepareSetupToken(context.Background(), testTenantID); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	info, err := os.Stat(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("token file mode = %o, want 600", perm)
	}
}

func TestPrepareSetupToken_ExistingUsers_NoToken(t *testing.T) {
	store := &mockStore{}
	svc, tokenFile := newSetupTestService(t, store)
	registerAndLogin(t, svc, "existing@test.com", "Password123")
	// A token file left by an earlier start goes away once users exist.
	if err := os.MkdirAll(filepath.Dir(tokenFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenFile, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	token, err := svc.PrepareSetupToken(context.Background(), testTenantID)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if token != "" {
		t.Fatalf("token = %q, want none when users exist", token)
	}
	if _, err := os.Stat(tokenFile); !os.IsNotExist(err) {
		t.Fatalf("stale token file must be removed, stat err = %v", err)
	}
}

func TestPrepareSetupToken_EnvAdminNeedsNoToken(t *testing.T) {
	store := &mockStore{}
	svc, tokenFile := newSetupTestService(t, store)
	svc.cfg.DefaultAdminPass = "Adminpass123"
	if err := svc.BootstrapAdmin(context.Background(), testTenantID); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	token, err := svc.PrepareSetupToken(context.Background(), testTenantID)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if token != "" {
		t.Fatalf("token = %q, want none with the env admin", token)
	}
	if _, err := os.Stat(tokenFile); !os.IsNotExist(err) {
		t.Fatalf("no token file expected, stat err = %v", err)
	}
}

func TestPrepareSetupToken_NoFileConfigured(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store) // SetupTokenFile empty: log only
	token, err := svc.PrepareSetupToken(context.Background(), testTenantID)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if token == "" {
		t.Fatal("expected a token")
	}
	if _, err := svc.CompleteSetup(context.Background(), setupRequest(testTenantID), token); err != nil {
		t.Fatalf("complete setup: %v", err)
	}
}

func TestCompleteSetup_RefusesWrongToken(t *testing.T) {
	store := &mockStore{}
	svc, tokenFile := newSetupTestService(t, store)
	token, err := svc.PrepareSetupToken(context.Background(), testTenantID)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	tests := []struct {
		name     string
		token    string
		tenantID string
	}{
		{"empty", "", testTenantID},
		{"wrong", strings.Repeat("a", len(token)), testTenantID},
		{"prefix", token[:len(token)-1], testTenantID},
		{"longer", token + "0", testTenantID},
		{"padded", " " + token + "\n", testTenantID},
		{"upper case", strings.ToUpper(token), testTenantID},
		{"other tenant", token, setupOtherTenantID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CompleteSetup(context.Background(), setupRequest(tt.tenantID), tt.token)
			if !errors.Is(err, user.ErrInvalidSetupToken) {
				t.Fatalf("err = %v, want ErrInvalidSetupToken", err)
			}
		})
	}
	if len(store.users) != 0 {
		t.Fatalf("no user may be created, got %d", len(store.users))
	}
	if _, err := os.Stat(tokenFile); err != nil {
		t.Fatalf("the token file must stay until a setup succeeds: %v", err)
	}
}

func TestCompleteSetup_NoTokenArmed(t *testing.T) {
	store := &mockStore{}
	svc, _ := newSetupTestService(t, store)
	_, err := svc.CompleteSetup(context.Background(), setupRequest(testTenantID), "")
	if !errors.Is(err, user.ErrInvalidSetupToken) {
		t.Fatalf("err = %v, want ErrInvalidSetupToken", err)
	}
	if len(store.users) != 0 {
		t.Fatalf("no user may be created, got %d", len(store.users))
	}
}

func TestCompleteSetup_RightTokenCreatesAdminAndRemovesToken(t *testing.T) {
	store := &mockStore{}
	svc, tokenFile := newSetupTestService(t, store)
	token, err := svc.PrepareSetupToken(context.Background(), testTenantID)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	u, err := svc.CompleteSetup(context.Background(), setupRequest(testTenantID), token)
	if err != nil {
		t.Fatalf("complete setup: %v", err)
	}
	if u.Role != user.RoleAdmin || u.TenantID != testTenantID {
		t.Fatalf("user = %+v, want an admin of the tenant", u)
	}
	if _, err := os.Stat(tokenFile); !os.IsNotExist(err) {
		t.Fatalf("token file must be removed after the setup, stat err = %v", err)
	}

	// The token is used up: the next call reports the finished setup.
	second := setupRequest(testTenantID)
	second.Email = "second@test.com"
	if _, err := svc.CompleteSetup(context.Background(), second, token); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second setup err = %v, want ErrConflict", err)
	}
	if len(store.users) != 1 {
		t.Fatalf("users = %d, want 1", len(store.users))
	}
}

func TestCompleteSetup_ValidatesBeforeTheToken(t *testing.T) {
	svc, _ := newSetupTestService(t, &mockStore{})
	req := setupRequest(testTenantID)
	req.Password = "short"
	if _, err := svc.CompleteSetup(context.Background(), req, ""); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

func TestPrepareSetupToken_UnwritableFileStillArmsTheLoggedToken(t *testing.T) {
	svc, _ := newSetupTestService(t, &mockStore{})
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	svc.cfg.SetupTokenFile = filepath.Join(blocker, "setup_token")

	token, err := svc.PrepareSetupToken(context.Background(), testTenantID)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if _, err := svc.CompleteSetup(context.Background(), setupRequest(testTenantID), token); err != nil {
		t.Fatalf("the logged token must work: %v", err)
	}
}
