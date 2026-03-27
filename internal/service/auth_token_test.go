package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/crypto"
	"github.com/Strob0t/CodeForge/internal/domain/user"
)

// newTestTokenManager creates a TokenManager with sensible test defaults
// and the given mock store.
func newTestTokenManager(store *mockStore, overrides ...func(*config.Auth)) *TokenManager {
	cfg := config.Auth{
		Enabled:            true,
		JWTSecret:          "test-secret-key-must-be-long-enough-for-hmac",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         4,
	}
	for _, fn := range overrides {
		fn(&cfg)
	}
	return NewTokenManager(store, &cfg)
}

// testUser returns a valid user suitable for JWT signing tests.
func testUser() *user.User {
	return &user.User{
		ID:       "user-001",
		Email:    "token@test.com",
		Name:     "Token Tester",
		Role:     user.RoleEditor,
		TenantID: "tenant-001",
		Enabled:  true,
	}
}

// --- Test: GenerateAccessToken_Valid ---

func TestGenerateAccessToken_Valid(t *testing.T) {
	store := &mockStore{}
	tm := newTestTokenManager(store)
	u := testUser()

	tokenStr, err := tm.SignJWT(u)
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("expected non-empty token")
	}

	// Parse it back without revocation check (direct verifyJWT).
	claims, err := tm.ValidateAccessToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}

	// Assert all claims match the input user.
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"UserID", claims.UserID, u.ID},
		{"Email", claims.Email, u.Email},
		{"Name", claims.Name, u.Name},
		{"Role", string(claims.Role), string(u.Role)},
		{"TenantID", claims.TenantID, u.TenantID},
		{"Audience", claims.Audience, "codeforge"},
		{"Issuer", claims.Issuer, "codeforge-core"},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	if claims.JTI == "" {
		t.Error("JTI should be non-empty")
	}
	if claims.IssuedAt == 0 {
		t.Error("IssuedAt should be non-zero")
	}
	if claims.Expiry == 0 {
		t.Error("Expiry should be non-zero")
	}
	if claims.Expiry <= claims.IssuedAt {
		t.Errorf("Expiry (%d) should be after IssuedAt (%d)", claims.Expiry, claims.IssuedAt)
	}
}

// --- Test: GenerateAccessToken_Expiry ---

func TestGenerateAccessToken_Expiry(t *testing.T) {
	store := &mockStore{}
	// Use a very short expiry so the token is already expired when we validate.
	tm := newTestTokenManager(store, func(cfg *config.Auth) {
		cfg.AccessTokenExpiry = -1 * time.Second // already in the past
	})
	u := testUser()

	tokenStr, err := tm.SignJWT(u)
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}

	_, err = tm.ValidateAccessToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error = %q, want to contain 'expired'", err.Error())
	}
}

// --- Test: RefreshToken_Valid ---

func TestRefreshToken_Valid(t *testing.T) {
	store := &mockStore{}
	tm := newTestTokenManager(store)
	u := testUser()

	// Seed the user into the store.
	store.users = append(store.users, *u)

	// Create an initial refresh token.
	rawToken, err := crypto.GenerateRandomToken()
	if err != nil {
		t.Fatalf("GenerateRandomToken: %v", err)
	}
	rt := &user.RefreshToken{
		ID:        crypto.GenerateUUIDv4(),
		UserID:    u.ID,
		TokenHash: crypto.HashSHA256(rawToken),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := store.CreateRefreshToken(context.Background(), rt); err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}

	// Refresh and get a new pair.
	resp, newRawToken, err := tm.RefreshTokens(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("RefreshTokens: %v", err)
	}

	if resp.AccessToken == "" {
		t.Error("new access token is empty")
	}
	if newRawToken == "" {
		t.Error("new raw refresh token is empty")
	}
	if newRawToken == rawToken {
		t.Error("new refresh token must differ from old (rotation)")
	}

	// Validate the new access token.
	claims, err := tm.ValidateAccessToken(resp.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken on refreshed token: %v", err)
	}
	if claims.UserID != u.ID {
		t.Errorf("UserID = %q, want %q", claims.UserID, u.ID)
	}

	// Old refresh token should be invalidated (rotated away).
	_, _, err = tm.RefreshTokens(context.Background(), rawToken)
	if err == nil {
		t.Fatal("expected error when reusing old refresh token after rotation")
	}
}

// --- Test: RefreshToken_Expired ---

func TestRefreshToken_Expired(t *testing.T) {
	store := &mockStore{}
	tm := newTestTokenManager(store)
	u := testUser()
	store.users = append(store.users, *u)

	rawToken, err := crypto.GenerateRandomToken()
	if err != nil {
		t.Fatalf("GenerateRandomToken: %v", err)
	}
	rt := &user.RefreshToken{
		ID:        crypto.GenerateUUIDv4(),
		UserID:    u.ID,
		TokenHash: crypto.HashSHA256(rawToken),
		ExpiresAt: time.Now().Add(-1 * time.Hour), // already expired
	}
	if err := store.CreateRefreshToken(context.Background(), rt); err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}

	_, _, err = tm.RefreshTokens(context.Background(), rawToken)
	if err == nil {
		t.Fatal("expected error for expired refresh token")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error = %q, want to contain 'expired'", err.Error())
	}
}

// --- Test: RefreshToken_Revoked ---

func TestRefreshToken_Revoked(t *testing.T) {
	store := &mockStore{}
	tm := newTestTokenManager(store)
	u := testUser()
	store.users = append(store.users, *u)

	// Create a valid refresh token.
	rawToken, err := crypto.GenerateRandomToken()
	if err != nil {
		t.Fatalf("GenerateRandomToken: %v", err)
	}
	rt := &user.RefreshToken{
		ID:        crypto.GenerateUUIDv4(),
		UserID:    u.ID,
		TokenHash: crypto.HashSHA256(rawToken),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := store.CreateRefreshToken(context.Background(), rt); err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}

	// Delete (revoke) the refresh token from the store directly.
	if err := store.DeleteRefreshToken(context.Background(), rt.ID); err != nil {
		t.Fatalf("DeleteRefreshToken: %v", err)
	}

	// Attempting refresh should fail because the token hash is no longer in the store.
	_, _, err = tm.RefreshTokens(context.Background(), rawToken)
	if err == nil {
		t.Fatal("expected error for revoked (deleted) refresh token")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error = %q, want to contain 'invalid'", err.Error())
	}
}

// --- Test: RevokeToken_Idempotent ---

func TestRevokeToken_Idempotent(t *testing.T) {
	store := &mockStore{}
	tm := newTestTokenManager(store)
	ctx := context.Background()

	jti := "jti-idempotent-001"
	expiresAt := time.Now().Add(15 * time.Minute)

	// Revoke once.
	if err := tm.RevokeAccessToken(ctx, jti, expiresAt); err != nil {
		t.Fatalf("first revoke: %v", err)
	}

	// Revoke the same JTI again -- should not error.
	if err := tm.RevokeAccessToken(ctx, jti, expiresAt); err != nil {
		t.Fatalf("second revoke (idempotent): %v", err)
	}

	// Verify it is actually revoked.
	u := testUser()
	tokenStr, err := tm.SignJWT(u)
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}

	// Parse to get the real JTI of this token, then revoke it and verify.
	claims, err := tm.ValidateAccessToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}

	if err := tm.RevokeAccessToken(ctx, claims.JTI, time.Unix(claims.Expiry, 0)); err != nil {
		t.Fatalf("revoke real token: %v", err)
	}

	_, err = tm.ValidateAccessToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for revoked access token")
	}
	if !strings.Contains(err.Error(), "revoked") {
		t.Errorf("error = %q, want to contain 'revoked'", err.Error())
	}
}

// --- Test: RevokeAllUserTokens ---

func TestRevokeAllUserTokens(t *testing.T) {
	store := &mockStore{}
	tm := newTestTokenManager(store)
	ctx := context.Background()
	u := testUser()
	store.users = append(store.users, *u)

	// Create multiple refresh tokens for the user.
	var rawTokens []string
	for i := 0; i < 3; i++ {
		raw, err := crypto.GenerateRandomToken()
		if err != nil {
			t.Fatalf("GenerateRandomToken[%d]: %v", i, err)
		}
		rawTokens = append(rawTokens, raw)
		rt := &user.RefreshToken{
			ID:        crypto.GenerateUUIDv4(),
			UserID:    u.ID,
			TokenHash: crypto.HashSHA256(raw),
			ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		}
		if err := store.CreateRefreshToken(ctx, rt); err != nil {
			t.Fatalf("CreateRefreshToken[%d]: %v", i, err)
		}
	}

	// Also generate and revoke an access token via Logout.
	accessToken, err := tm.SignJWT(u)
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}
	claims, err := tm.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}

	// Logout revokes the access token and deletes all refresh tokens.
	if err := tm.Logout(ctx, u.ID, claims.JTI, time.Unix(claims.Expiry, 0)); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// All refresh tokens should be gone.
	for i, raw := range rawTokens {
		_, _, err := tm.RefreshTokens(ctx, raw)
		if err == nil {
			t.Errorf("refresh token[%d]: expected error after revoking all, got nil", i)
		}
	}

	// Access token should be revoked.
	_, err = tm.ValidateAccessToken(accessToken)
	if err == nil {
		t.Fatal("expected error for revoked access token after logout")
	}
}

// --- Test: CleanupExpiredTokens ---

func TestCleanupExpiredTokens(t *testing.T) {
	inner := &mockStore{}
	pStore := &purgeTrackingMockStore{mockStore: inner}

	cfg := config.Auth{
		Enabled:            true,
		JWTSecret:          "test-secret-key-must-be-long-enough-for-hmac",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}

	// Seed some expired revoked tokens.
	inner.revokedTokens = map[string]time.Time{
		"expired-jti-1": time.Now().Add(-1 * time.Hour),
		"expired-jti-2": time.Now().Add(-2 * time.Hour),
		"valid-jti-1":   time.Now().Add(1 * time.Hour),
	}

	// Override PurgeExpiredTokens to actually remove expired entries.
	pStore.purgeFunc = func(_ context.Context) (int64, error) {
		var count int64
		now := time.Now()
		for jti, exp := range inner.revokedTokens {
			if now.After(exp) {
				delete(inner.revokedTokens, jti)
				count++
			}
		}
		return count, nil
	}

	// Build TokenManager with the purge-tracking store.
	tm := &TokenManager{
		store:  pStore,
		secret: []byte(cfg.JWTSecret),
		cfg:    &cfg,
	}

	ctx, cancel := context.WithCancel(context.Background())
	tm.StartTokenCleanup(ctx, 50*time.Millisecond)

	// Wait for at least one tick.
	time.Sleep(200 * time.Millisecond)
	cancel()

	// Expired tokens should have been purged.
	if len(inner.revokedTokens) != 1 {
		t.Errorf("expected 1 remaining token, got %d", len(inner.revokedTokens))
	}
	if _, ok := inner.revokedTokens["valid-jti-1"]; !ok {
		t.Error("valid-jti-1 should remain after cleanup")
	}
}

// purgeTrackingMockStore wraps mockStore and lets us override PurgeExpiredTokens behavior.
type purgeTrackingMockStore struct {
	*mockStore
	purgeFunc func(ctx context.Context) (int64, error)
}

func (p *purgeTrackingMockStore) PurgeExpiredTokens(ctx context.Context) (int64, error) {
	if p.purgeFunc != nil {
		return p.purgeFunc(ctx)
	}
	return 0, nil
}

func (p *purgeTrackingMockStore) DeleteExpiredPasswordResetTokens(_ context.Context) (int64, error) {
	return 0, nil
}

// --- Test: HMACSignatureVerification ---

func TestHMACSignatureVerification(t *testing.T) {
	store := &mockStore{}
	tm := newTestTokenManager(store)
	u := testUser()

	tokenStr, err := tm.SignJWT(u)
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}

	t.Run("tampered_payload", func(t *testing.T) {
		parts := strings.SplitN(tokenStr, ".", 3)
		if len(parts) != 3 {
			t.Fatal("unexpected token structure")
		}

		// Decode payload, modify email, re-encode with WRONG signature.
		payloadBytes, err := base64URLDecodePadded(parts[1])
		if err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		var claims user.TokenClaims
		if err := json.Unmarshal(payloadBytes, &claims); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		claims.Email = "evil@attacker.com"
		modPayload, _ := json.Marshal(claims)
		modB64 := base64URLEncodePadded(modPayload)

		// Keep the original signature (which no longer matches).
		tampered := parts[0] + "." + modB64 + "." + parts[2]

		_, err = tm.ValidateAccessToken(tampered)
		if err == nil {
			t.Fatal("expected error for tampered payload")
		}
		if !strings.Contains(err.Error(), "invalid signature") {
			t.Errorf("error = %q, want to contain 'invalid signature'", err.Error())
		}
	})

	t.Run("tampered_signature", func(t *testing.T) {
		parts := strings.SplitN(tokenStr, ".", 3)
		if len(parts) != 3 {
			t.Fatal("unexpected token structure")
		}

		// Re-sign with a different secret.
		wrongSecret := []byte("wrong-secret-key-totally-different")
		signingInput := parts[0] + "." + parts[1]
		mac := hmac.New(sha256.New, wrongSecret)
		mac.Write([]byte(signingInput))
		wrongSig := base64URLEncodePadded(mac.Sum(nil))

		tampered := signingInput + "." + wrongSig

		_, err := tm.ValidateAccessToken(tampered)
		if err == nil {
			t.Fatal("expected error for wrong-secret signature")
		}
		if !strings.Contains(err.Error(), "invalid signature") {
			t.Errorf("error = %q, want to contain 'invalid signature'", err.Error())
		}
	})

	t.Run("truncated_signature", func(t *testing.T) {
		parts := strings.SplitN(tokenStr, ".", 3)
		if len(parts) != 3 {
			t.Fatal("unexpected token structure")
		}

		// Truncate signature to half length.
		truncSig := parts[2][:len(parts[2])/2]
		tampered := parts[0] + "." + parts[1] + "." + truncSig

		_, err := tm.ValidateAccessToken(tampered)
		if err == nil {
			t.Fatal("expected error for truncated signature")
		}
		if !strings.Contains(err.Error(), "invalid signature") {
			t.Errorf("error = %q, want to contain 'invalid signature'", err.Error())
		}
	})

	t.Run("different_secret_manager", func(t *testing.T) {
		// A token signed by one secret should not validate with another.
		tm2 := newTestTokenManager(store, func(cfg *config.Auth) {
			cfg.JWTSecret = "completely-different-secret-key-here"
		})

		_, err := tm2.ValidateAccessToken(tokenStr)
		if err == nil {
			t.Fatal("expected error when validating with different secret")
		}
		if !strings.Contains(err.Error(), "invalid signature") {
			t.Errorf("error = %q, want to contain 'invalid signature'", err.Error())
		}
	})

	t.Run("malformed_token", func(t *testing.T) {
		cases := []struct {
			name  string
			token string
		}{
			{"no_dots", "nodots"},
			{"one_dot", "one.dot"},
			{"empty_string", ""},
		}
		for _, tc := range cases {
			_, err := tm.ValidateAccessToken(tc.token)
			if err == nil {
				t.Errorf("%s: expected error for malformed token", tc.name)
			}
		}
	})

	t.Run("valid_token_succeeds", func(t *testing.T) {
		claims, err := tm.ValidateAccessToken(tokenStr)
		if err != nil {
			t.Fatalf("expected valid token to pass: %v", err)
		}
		if claims.Email != u.Email {
			t.Errorf("Email = %q, want %q", claims.Email, u.Email)
		}
	})
}

// --- Test: ValidateAccessToken with DB error fails closed ---

func TestValidateAccessToken_DBError_FailClosed(t *testing.T) {
	store := &mockStore{
		isTokenRevokedErr: errors.New("connection refused"),
	}
	tm := newTestTokenManager(store)
	u := testUser()

	tokenStr, err := tm.SignJWT(u)
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}

	_, err = tm.ValidateAccessToken(tokenStr)
	if err == nil {
		t.Fatal("expected fail-closed error when DB is unavailable")
	}
	if !strings.Contains(err.Error(), "unable to verify") {
		t.Errorf("error = %q, want to contain 'unable to verify'", err.Error())
	}
}

// --- Test: MustChangePassword claim propagation ---

func TestGenerateAccessToken_MustChangePassword(t *testing.T) {
	store := &mockStore{}
	tm := newTestTokenManager(store)
	u := testUser()
	u.MustChangePassword = true

	tokenStr, err := tm.SignJWT(u)
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}

	claims, err := tm.ValidateAccessToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if !claims.MustChangePassword {
		t.Error("MustChangePassword should be true in claims")
	}
}

// --- helpers ---

func base64URLDecodePadded(s string) ([]byte, error) {
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

func base64URLEncodePadded(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}
