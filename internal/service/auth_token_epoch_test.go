package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/user"
)

// KI-143: an access token carries its user's token epoch; deleting, erasing
// or disabling the user and a role change raise the epoch, so the user's
// earlier access tokens stop working before they expire.

// loginToken registers an editor and returns the user and an access token
// that has passed validation once (so its epoch is cached).
func loginToken(t *testing.T, svc *AuthService, email string) (u *user.User, token string) {
	t.Helper()
	u, resp, _ := registerAndLogin(t, svc, email, "Password123")
	if _, err := svc.ValidateAccessToken(resp.AccessToken); err != nil {
		t.Fatalf("fresh token refused: %v", err)
	}
	return u, resp.AccessToken
}

func TestTokenEpoch_TokenCarriesTheUsersEpoch(t *testing.T) {
	store := &mockStore{}
	tm := newTestTokenManager(store)
	u := testUser()
	u.TokenEpoch = 3
	store.users = append(store.users, *u)

	tokenStr, err := tm.SignJWT(u)
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}
	claims, err := tm.ValidateAccessToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if claims.TokenEpoch == nil || *claims.TokenEpoch != 3 {
		t.Fatalf("claims.TokenEpoch = %v, want 3", claims.TokenEpoch)
	}
}

// legacyClaims are the access token claims before KI-143 (no epoch).
type legacyClaims struct {
	JTI      string    `json:"jti"`
	UserID   string    `json:"sub"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Role     user.Role `json:"role"`
	TenantID string    `json:"tid"`
	Audience string    `json:"aud"`
	Issuer   string    `json:"iss"`
	IssuedAt int64     `json:"iat"`
	Expiry   int64     `json:"exp"`
}

func TestTokenEpoch_TokenWithoutTheClaimIsRefused(t *testing.T) {
	store := &mockStore{}
	tm := newTestTokenManager(store)
	u := testUser()
	store.users = append(store.users, *u)

	now := time.Now()
	payload, err := json.Marshal(legacyClaims{
		JTI: "jti-old", UserID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role,
		TenantID: u.TenantID, Audience: "codeforge", Issuer: "codeforge-core",
		IssuedAt: now.Unix(), Expiry: now.Add(10 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	signingInput := jwtHeader + "." + base64URLEncode(payload)
	mac := hmac.New(sha256.New, tm.secret)
	mac.Write([]byte(signingInput))
	legacy := signingInput + "." + base64URLEncode(mac.Sum(nil))

	if _, err := tm.ValidateAccessToken(legacy); err == nil {
		t.Fatal("a token issued before the token epoch must be refused")
	}

	current, err := tm.SignJWT(u) // epoch 0, like every user after migration 126
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tm.ValidateAccessToken(current); err != nil {
		t.Fatalf("a token with epoch 0 must pass: %v", err)
	}
}

func TestTokenEpoch_TriggersRefuseEarlierTokens(t *testing.T) {
	tests := []struct {
		name   string
		act    func(ctx context.Context, svc *AuthService, store *mockStore, id string) error
		refuse bool
	}{
		{"role change", func(ctx context.Context, svc *AuthService, _ *mockStore, id string) error {
			_, err := svc.UpdateUser(ctx, id, user.UpdateRequest{Role: user.RoleViewer})
			return err
		}, true},
		{"disable", func(ctx context.Context, svc *AuthService, _ *mockStore, id string) error {
			_, err := svc.UpdateUser(ctx, id, user.UpdateRequest{Enabled: ptrTo(false)})
			return err
		}, true},
		{"delete", func(ctx context.Context, svc *AuthService, _ *mockStore, id string) error {
			return svc.DeleteUser(ctx, id)
		}, true},
		{"erase", func(ctx context.Context, svc *AuthService, store *mockStore, id string) error {
			gdpr := NewGDPRService(store)
			gdpr.SetTokenInvalidator(svc.Tokens())
			return gdpr.DeleteUserData(ctx, id)
		}, true},
		{"rename only", func(ctx context.Context, svc *AuthService, _ *mockStore, id string) error {
			_, err := svc.UpdateUser(ctx, id, user.UpdateRequest{Name: "Renamed"})
			return err
		}, false},
		{"same role", func(ctx context.Context, svc *AuthService, _ *mockStore, id string) error {
			_, err := svc.UpdateUser(ctx, id, user.UpdateRequest{Role: user.RoleEditor})
			return err
		}, false},
		{"enable an enabled user", func(ctx context.Context, svc *AuthService, _ *mockStore, id string) error {
			_, err := svc.UpdateUser(ctx, id, user.UpdateRequest{Enabled: ptrTo(true)})
			return err
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockStore{}
			svc := newTestAuthService(store)
			u, token := loginToken(t, svc, "epoch@test.com")

			if err := tt.act(context.Background(), svc, store, u.ID); err != nil {
				t.Fatalf("act: %v", err)
			}
			_, err := svc.ValidateAccessToken(token)
			if tt.refuse && err == nil {
				t.Fatal("the earlier token must be refused at once on this replica")
			}
			if !tt.refuse && err != nil {
				t.Fatalf("the token must stay valid: %v", err)
			}
		})
	}
}

func TestTokenEpoch_ReenablingDoesNotRestoreOldTokens(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	u, token := loginToken(t, svc, "reenable@test.com")
	ctx := context.Background()

	if _, err := svc.UpdateUser(ctx, u.ID, user.UpdateRequest{Enabled: ptrTo(false)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateUser(ctx, u.ID, user.UpdateRequest{Enabled: ptrTo(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateAccessToken(token); err == nil {
		t.Fatal("a token from before the disable must stay refused")
	}
	resp, _, err := svc.Login(ctx, user.LoginRequest{Email: "reenable@test.com", Password: "Password123"}, testTenantID)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := svc.ValidateAccessToken(resp.AccessToken); err != nil {
		t.Fatalf("a new token must pass: %v", err)
	}
}

// The role change and the epoch raise are saved together: a failed save
// changes nothing, and a retry raises the epoch (S9-A review).
func TestTokenEpoch_FailedUpdateSavesNothingAndRetryRaises(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	u, token := loginToken(t, svc, "raisefail@test.com")
	ctx := context.Background()
	store.invalidateTokensErr = errors.New("connection refused")

	if _, err := svc.UpdateUser(ctx, u.ID, user.UpdateRequest{Role: user.RoleAdmin}); err == nil {
		t.Fatal("the failed save must be reported")
	}
	if got := store.users[0]; got.Role != user.RoleEditor || got.TokenEpoch != 0 {
		t.Fatalf("a failed save must change nothing, user = role %s epoch %d", got.Role, got.TokenEpoch)
	}

	store.invalidateTokensErr = nil
	updated, err := svc.UpdateUser(ctx, u.ID, user.UpdateRequest{Role: user.RoleAdmin})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := store.users[0]; got.Role != user.RoleAdmin || got.TokenEpoch != 1 || updated.TokenEpoch != 1 {
		t.Fatalf("retry: stored role %s epoch %d, returned epoch %d", got.Role, got.TokenEpoch, updated.TokenEpoch)
	}
	if _, err := svc.ValidateAccessToken(token); err == nil {
		t.Fatal("the earlier token must be refused after the retry")
	}
}

func TestTokenEpoch_CachedBriefly(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	now := time.Now()
	svc.Tokens().epochs.now = func() time.Time { return now }
	u, token := loginToken(t, svc, "cache@test.com") // one lookup

	for range 5 {
		if _, err := svc.ValidateAccessToken(token); err != nil {
			t.Fatalf("validate: %v", err)
		}
	}
	if store.tokenEpochLookups != 1 {
		t.Fatalf("lookups = %d, want 1 within the cache TTL", store.tokenEpochLookups)
	}

	// Another replica raises the epoch: this one refuses the token once the
	// cached value has expired.
	for i := range store.users {
		if store.users[i].ID == u.ID {
			store.users[i].TokenEpoch++
		}
	}
	if _, err := svc.ValidateAccessToken(token); err != nil {
		t.Fatalf("still cached, the token passes: %v", err)
	}
	now = now.Add(tokenEpochCacheTTL + time.Millisecond)
	if _, err := svc.ValidateAccessToken(token); err == nil {
		t.Fatal("after the TTL the raised epoch must refuse the token")
	}
	if store.tokenEpochLookups != 2 {
		t.Fatalf("lookups = %d, want 2", store.tokenEpochLookups)
	}
}

func TestTokenEpoch_StoreErrorFailsClosed(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	_, resp, _ := registerAndLogin(t, svc, "dberr@test.com", "Password123")
	store.tokenEpochErr = errors.New("connection refused")

	if _, err := svc.ValidateAccessToken(resp.AccessToken); err == nil {
		t.Fatal("a failed epoch lookup must refuse the token")
	}
	store.tokenEpochErr = nil
	if _, err := svc.ValidateAccessToken(resp.AccessToken); err != nil {
		t.Fatalf("the failure must not be cached: %v", err)
	}
}

func TestTokenEpochCache_Bounded(t *testing.T) {
	tests := []struct {
		name  string
		max   int
		users int
	}{
		{"one", 1, 2},
		{"at the bound", 8, 8},
		{"over the bound", 8, 9},
		{"far over", 8, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTokenEpochCache(time.Minute, tt.max)
			for i := range tt.users {
				c.putIfCurrent(c.generation(), fmt.Sprintf("user-%d", i), "tenant", int64(i), false)
			}
			if n := c.len(); n > tt.max {
				t.Fatalf("cache holds %d entries, bound %d", n, tt.max)
			}
			last := fmt.Sprintf("user-%d", tt.users-1)
			if e, ok := c.get(last, "tenant"); !ok || e.epoch != int64(tt.users-1) {
				t.Fatalf("the newest entry must be cached, got %+v %v", e, ok)
			}
		})
	}
}

func TestTokenEpochCache_ExpiredEntriesGoFirst(t *testing.T) {
	now := time.Now()
	c := newTokenEpochCache(time.Second, 2)
	c.now = func() time.Time { return now }
	c.putIfCurrent(c.generation(), "old", "tenant", 1, false)
	now = now.Add(2 * time.Second)
	c.putIfCurrent(c.generation(), "a", "tenant", 2, false)
	c.putIfCurrent(c.generation(), "b", "tenant", 3, false)
	if _, ok := c.get("a", "tenant"); !ok {
		t.Fatal("a live entry was evicted while an expired one was there")
	}
	if _, ok := c.get("b", "tenant"); !ok {
		t.Fatal("the newest entry must be cached")
	}
}

func TestTokenEpochCache_TenantMustMatch(t *testing.T) {
	c := newTokenEpochCache(time.Minute, 4)
	c.putIfCurrent(c.generation(), "u1", "tenant-a", 1, false)
	if _, ok := c.get("u1", "tenant-b"); ok {
		t.Fatal("an entry cached for another tenant must not be used")
	}
	c.forget("u1")
	if _, ok := c.get("u1", "tenant-a"); ok {
		t.Fatal("forget must drop the entry")
	}
}

// The password paths end the user's sessions: earlier access tokens are
// refused, refresh tokens deleted, and the raise is saved in the user's own
// tenant whatever tenant the request named (the reset endpoints are public;
// S9-A review).
func TestTokenEpoch_PasswordPathsEndSessions(t *testing.T) {
	const tenant = "11111111-1111-1111-1111-111111111111"
	const oldPassword, newPassword = "Password123", "NewPassword456"
	tests := []struct {
		name string
		act  func(ctx context.Context, svc *AuthService, u *user.User) error
	}{
		{"admin reset", func(ctx context.Context, svc *AuthService, u *user.User) error {
			return svc.AdminResetPassword(ctx, u.Email, u.TenantID, newPassword)
		}},
		{"reset link", func(ctx context.Context, svc *AuthService, u *user.User) error {
			raw, err := svc.RequestPasswordReset(ctx, u.Email, u.TenantID)
			if err != nil || raw == "" {
				return fmt.Errorf("request reset: %q %w", raw, err)
			}
			return svc.ConfirmPasswordReset(ctx, raw, newPassword)
		}},
		{"change password", func(ctx context.Context, svc *AuthService, u *user.User) error {
			return svc.ChangePassword(ctx, u.ID, user.ChangePasswordRequest{OldPassword: oldPassword, NewPassword: newPassword})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockStore{}
			svc := newTestAuthService(store)
			ctx := context.Background() // the default tenant, not the user's
			u, err := svc.Register(ctx, &user.CreateRequest{
				Email: "pw@test.com", Name: "PW", Password: oldPassword, Role: user.RoleEditor, TenantID: tenant,
			})
			if err != nil {
				t.Fatal(err)
			}
			resp, _, err := svc.Login(ctx, user.LoginRequest{Email: u.Email, Password: oldPassword}, tenant)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.ValidateAccessToken(resp.AccessToken); err != nil {
				t.Fatalf("fresh token: %v", err)
			}

			if err := tt.act(ctx, svc, u); err != nil {
				t.Fatalf("act: %v", err)
			}
			if _, err := svc.ValidateAccessToken(resp.AccessToken); err == nil {
				t.Fatal("the earlier access token must be refused")
			}
			if len(store.refreshTokens) != 0 {
				t.Fatalf("refresh tokens left: %d", len(store.refreshTokens))
			}
			if len(store.invalidatedTenants) != 1 || store.invalidatedTenants[0] != tenant {
				t.Fatalf("epoch raised in tenants %v, want [%s]", store.invalidatedTenants, tenant)
			}
			// The UI signs in again with the new password.
			fresh, _, err := svc.Login(ctx, user.LoginRequest{Email: u.Email, Password: newPassword}, tenant)
			if err != nil {
				t.Fatalf("login with the new password: %v", err)
			}
			if _, err := svc.ValidateAccessToken(fresh.AccessToken); err != nil {
				t.Fatalf("a new token must pass: %v", err)
			}
		})
	}
}

// recordingDropper records whose WebSocket connections were closed.
type recordingDropper struct{ users []string }

func (d *recordingDropper) DropUser(userID string) int {
	d.users = append(d.users, userID)
	return 1
}

// Whatever ends a user's tokens also closes the user's WebSocket connections
// on this replica (S9-A review).
func TestTokenEpoch_EndingSessionsClosesConnections(t *testing.T) {
	tests := []struct {
		name string
		act  func(ctx context.Context, svc *AuthService, store *mockStore, u *user.User) error
		drop bool
	}{
		{"role change", func(ctx context.Context, svc *AuthService, _ *mockStore, u *user.User) error {
			_, err := svc.UpdateUser(ctx, u.ID, user.UpdateRequest{Role: user.RoleViewer})
			return err
		}, true},
		{"disable", func(ctx context.Context, svc *AuthService, _ *mockStore, u *user.User) error {
			_, err := svc.UpdateUser(ctx, u.ID, user.UpdateRequest{Enabled: ptrTo(false)})
			return err
		}, true},
		{"delete", func(ctx context.Context, svc *AuthService, _ *mockStore, u *user.User) error {
			return svc.DeleteUser(ctx, u.ID)
		}, true},
		{"erase", func(ctx context.Context, svc *AuthService, store *mockStore, u *user.User) error {
			gdpr := NewGDPRService(store)
			gdpr.SetTokenInvalidator(svc.Tokens())
			return gdpr.DeleteUserData(ctx, u.ID)
		}, true},
		{"admin reset", func(ctx context.Context, svc *AuthService, _ *mockStore, u *user.User) error {
			return svc.AdminResetPassword(ctx, u.Email, u.TenantID, "NewPassword456")
		}, true},
		{"change password", func(ctx context.Context, svc *AuthService, _ *mockStore, u *user.User) error {
			return svc.ChangePassword(ctx, u.ID, user.ChangePasswordRequest{OldPassword: "Password123", NewPassword: "NewPassword456"})
		}, true},
		{"rename only", func(ctx context.Context, svc *AuthService, _ *mockStore, u *user.User) error {
			_, err := svc.UpdateUser(ctx, u.ID, user.UpdateRequest{Name: "Renamed"})
			return err
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockStore{}
			svc := newTestAuthService(store)
			dropper := &recordingDropper{}
			svc.SetConnectionDropper(dropper)
			u, _ := loginToken(t, svc, "ws@test.com")

			if err := tt.act(context.Background(), svc, store, u); err != nil {
				t.Fatalf("act: %v", err)
			}
			want := 0
			if tt.drop {
				want = 1
			}
			if len(dropper.users) != want || (want == 1 && dropper.users[0] != u.ID) {
				t.Fatalf("connections dropped for %v, want %d drop(s) of %s", dropper.users, want, u.ID)
			}
		})
	}
}

// A raise that lands while a lookup is under way: the lookup's (old) epoch is
// not cached after the raise dropped the entry, so the next check reads the
// new one (S9-A review).
func TestTokenEpoch_LookupRacingARaiseIsNotCached(t *testing.T) {
	store := &mockStore{}
	svc := newTestAuthService(store)
	u, token := loginToken(t, svc, "race@test.com")
	svc.Tokens().EndUserSessions(u.ID) // empty cache: the next check reads the store

	store.afterTokenEpochRead = func() {
		store.afterTokenEpochRead = nil
		store.users[0].TokenEpoch++        // another request raises the epoch ...
		svc.Tokens().EndUserSessions(u.ID) // ... and drops the cached value
	}
	// This check read the epoch before the raise and may pass.
	_, _ = svc.ValidateAccessToken(token)

	if _, err := svc.ValidateAccessToken(token); err == nil {
		t.Fatal("the epoch read before the raise was cached: the stale token passes")
	}
}

func TestTokenEpochCache_PutAfterForgetIsSkipped(t *testing.T) {
	c := newTokenEpochCache(time.Minute, 4)
	gen := c.generation()
	c.forget("other-user")
	c.putIfCurrent(gen, "u1", "tenant", 1, false)
	if _, ok := c.get("u1", "tenant"); ok {
		t.Fatal("a value read before a forget must not be cached")
	}
	c.putIfCurrent(c.generation(), "u1", "tenant", 2, false)
	if e, ok := c.get("u1", "tenant"); !ok || e.epoch != 2 {
		t.Fatalf("a value read after the forget must be cached, got %+v %v", e, ok)
	}
}
