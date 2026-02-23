package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// AuthService handles authentication, JWT tokens, and API keys.
type AuthService struct {
	store  database.Store
	cfg    *config.Auth
	secret []byte
}

// NewAuthService creates a new authentication service.
func NewAuthService(store database.Store, cfg *config.Auth) *AuthService {
	return &AuthService{
		store:  store,
		cfg:    cfg,
		secret: []byte(cfg.JWTSecret),
	}
}

// Register creates a new user with a bcrypt-hashed password.
func (s *AuthService) Register(ctx context.Context, req *user.CreateRequest) (*user.User, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.cfg.BcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u := &user.User{
		ID:           generateID(),
		Email:        req.Email,
		Name:         req.Name,
		PasswordHash: string(hash),
		Role:         req.Role,
		TenantID:     req.TenantID,
		Enabled:      true,
	}

	if err := s.store.CreateUser(ctx, u); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

// Login authenticates a user and returns an access token + refresh token hash.
func (s *AuthService) Login(ctx context.Context, req user.LoginRequest, tenantID string) (*user.LoginResponse, string, error) {
	if err := req.Validate(); err != nil {
		return nil, "", fmt.Errorf("validate: %w", err)
	}

	u, err := s.store.GetUserByEmail(ctx, req.Email, tenantID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, "", errors.New("invalid credentials")
		}
		return nil, "", fmt.Errorf("get user: %w", err)
	}

	if !u.Enabled {
		return nil, "", errors.New("account is disabled")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		return nil, "", errors.New("invalid credentials")
	}

	accessToken, err := s.signJWT(u)
	if err != nil {
		return nil, "", fmt.Errorf("sign jwt: %w", err)
	}

	// Create refresh token
	rawToken, err := generateRandomToken(32)
	if err != nil {
		return nil, "", fmt.Errorf("generate refresh token: %w", err)
	}

	tokenHash := hashSHA256(rawToken)
	rt := &user.RefreshToken{
		ID:        generateID(),
		UserID:    u.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(s.cfg.RefreshTokenExpiry),
	}

	if err := s.store.CreateRefreshToken(ctx, rt); err != nil {
		return nil, "", fmt.Errorf("store refresh token: %w", err)
	}

	resp := &user.LoginResponse{
		AccessToken: accessToken,
		ExpiresIn:   int(s.cfg.AccessTokenExpiry.Seconds()),
		User:        *u,
	}
	return resp, rawToken, nil
}

// RefreshTokens validates a refresh token, atomically rotates it, and issues a new access token.
func (s *AuthService) RefreshTokens(ctx context.Context, rawToken string) (*user.LoginResponse, string, error) {
	tokenHash := hashSHA256(rawToken)

	rt, err := s.store.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, "", errors.New("invalid refresh token")
	}

	if time.Now().After(rt.ExpiresAt) {
		_ = s.store.DeleteRefreshToken(ctx, rt.ID)
		return nil, "", errors.New("refresh token expired")
	}

	u, err := s.store.GetUser(ctx, rt.UserID)
	if err != nil {
		return nil, "", fmt.Errorf("get user: %w", err)
	}

	if !u.Enabled {
		return nil, "", errors.New("account is disabled")
	}

	accessToken, err := s.signJWT(u)
	if err != nil {
		return nil, "", fmt.Errorf("sign jwt: %w", err)
	}

	// Issue new refresh token via atomic rotation (P2-3)
	newRawToken, err := generateRandomToken(32)
	if err != nil {
		return nil, "", fmt.Errorf("generate refresh token: %w", err)
	}

	newRT := &user.RefreshToken{
		ID:        generateID(),
		UserID:    u.ID,
		TokenHash: hashSHA256(newRawToken),
		ExpiresAt: time.Now().Add(s.cfg.RefreshTokenExpiry),
	}

	if err := s.store.RotateRefreshToken(ctx, rt.ID, newRT); err != nil {
		return nil, "", fmt.Errorf("rotate refresh token: %w", err)
	}

	resp := &user.LoginResponse{
		AccessToken: accessToken,
		ExpiresIn:   int(s.cfg.AccessTokenExpiry.Seconds()),
		User:        *u,
	}
	return resp, newRawToken, nil
}

// Logout deletes all refresh tokens for a user and optionally revokes the
// current access token by JTI. Pass empty jti to skip revocation.
func (s *AuthService) Logout(ctx context.Context, userID, jti string, tokenExpiry time.Time) error {
	if jti != "" {
		if err := s.store.RevokeToken(ctx, jti, tokenExpiry); err != nil {
			slog.Warn("failed to revoke access token on logout", "jti", jti, "error", err)
		}
	}
	return s.store.DeleteRefreshTokensByUser(ctx, userID)
}

// RevokeAccessToken adds a token JTI to the revocation blacklist.
func (s *AuthService) RevokeAccessToken(ctx context.Context, jti string, expiresAt time.Time) error {
	return s.store.RevokeToken(ctx, jti, expiresAt)
}

// ValidateAccessToken verifies a JWT and returns the claims.
// It checks token revocation when a JTI is present (fail-closed on DB error).
func (s *AuthService) ValidateAccessToken(tokenStr string) (*user.TokenClaims, error) {
	claims, err := s.verifyJWT(tokenStr)
	if err != nil {
		return nil, err
	}

	// Check revocation for tokens with JTI (backward compat: old tokens without jti skip this)
	if claims.JTI != "" {
		revoked, dbErr := s.store.IsTokenRevoked(context.Background(), claims.JTI)
		if dbErr != nil {
			// Fail-closed: deny access when revocation check is unavailable.
			slog.Error("token revocation check failed, denying token", "jti", claims.JTI, "error", dbErr)
			return nil, errors.New("unable to verify token status")
		}
		if revoked {
			return nil, errors.New("token has been revoked")
		}
	}

	return claims, nil
}

// ValidateAPIKey looks up an API key by its SHA-256 hash.
// Returns the user and the API key (for scope checking).
func (s *AuthService) ValidateAPIKey(ctx context.Context, rawKey string) (*user.User, *user.APIKey, error) {
	keyHash := hashSHA256(rawKey)
	apiKey, err := s.store.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		return nil, nil, errors.New("invalid api key")
	}

	if !apiKey.ExpiresAt.IsZero() && time.Now().After(apiKey.ExpiresAt) {
		return nil, nil, errors.New("api key expired")
	}

	u, err := s.store.GetUser(ctx, apiKey.UserID)
	if err != nil {
		return nil, nil, fmt.Errorf("get user: %w", err)
	}
	return u, apiKey, nil
}

// CreateAPIKey generates a new API key for a user.
func (s *AuthService) CreateAPIKey(ctx context.Context, userID string, req user.CreateAPIKeyRequest) (*user.CreateAPIKeyResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	rawKey, err := generateRandomToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	plainKey := user.APIKeyPrefix + rawKey

	var expiresAt time.Time
	if req.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(req.ExpiresIn) * time.Second)
	}

	key := &user.APIKey{
		ID:        generateID(),
		UserID:    userID,
		Name:      req.Name,
		Prefix:    plainKey[:12], // "cfk_" + 8 chars
		KeyHash:   hashSHA256(plainKey),
		ExpiresAt: expiresAt,
		Scopes:    req.Scopes,
	}

	if err := s.store.CreateAPIKey(ctx, key); err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}

	return &user.CreateAPIKeyResponse{
		APIKey:   *key,
		PlainKey: plainKey,
	}, nil
}

// ListAPIKeys returns all API keys for a user.
func (s *AuthService) ListAPIKeys(ctx context.Context, userID string) ([]user.APIKey, error) {
	return s.store.ListAPIKeysByUser(ctx, userID)
}

// DeleteAPIKey removes an API key.
func (s *AuthService) DeleteAPIKey(ctx context.Context, id string) error {
	return s.store.DeleteAPIKey(ctx, id)
}

// ListUsers returns all users for a tenant.
func (s *AuthService) ListUsers(ctx context.Context, tenantID string) ([]user.User, error) {
	return s.store.ListUsers(ctx, tenantID)
}

// GetUser returns a user by ID.
func (s *AuthService) GetUser(ctx context.Context, id string) (*user.User, error) {
	return s.store.GetUser(ctx, id)
}

// UpdateUser updates user fields (name, role, enabled).
func (s *AuthService) UpdateUser(ctx context.Context, id string, req user.UpdateRequest) (*user.User, error) {
	u, err := s.store.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		u.Name = req.Name
	}
	if req.Role != "" {
		if !user.ValidRoles[req.Role] {
			return nil, errors.New("invalid role")
		}
		u.Role = req.Role
	}
	if req.Enabled != nil {
		u.Enabled = *req.Enabled
	}

	if err := s.store.UpdateUser(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// DeleteUser removes a user and their refresh tokens.
func (s *AuthService) DeleteUser(ctx context.Context, id string) error {
	return s.store.DeleteUser(ctx, id)
}

// SeedDefaultAdmin creates the initial admin user if no users exist.
func (s *AuthService) SeedDefaultAdmin(ctx context.Context, tenantID string) error {
	users, err := s.store.ListUsers(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	if len(users) > 0 {
		return nil // already seeded
	}

	u, err := s.Register(ctx, &user.CreateRequest{
		Email:    s.cfg.DefaultAdminEmail,
		Name:     "Admin",
		Password: s.cfg.DefaultAdminPass,
		Role:     user.RoleAdmin,
		TenantID: tenantID,
	})
	if err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}

	// Force password change for seeded admin (P2-2)
	u.MustChangePassword = true
	if err := s.store.UpdateUser(ctx, u); err != nil {
		return fmt.Errorf("set must_change_password: %w", err)
	}

	slog.Info("seeded default admin user", "email", s.cfg.DefaultAdminEmail)
	return nil
}

// ChangePassword verifies the old password, validates complexity of the new one,
// hashes it, updates the user, and clears the MustChangePassword flag.
func (s *AuthService) ChangePassword(ctx context.Context, userID string, req user.ChangePasswordRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.OldPassword)); err != nil {
		return errors.New("current password is incorrect")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), s.cfg.BcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	u.PasswordHash = string(hash)
	u.MustChangePassword = false

	if err := s.store.UpdateUser(ctx, u); err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

// StartTokenCleanup starts a background goroutine that periodically purges
// expired revoked tokens. It stops when ctx is cancelled.
func (s *AuthService) StartTokenCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := s.store.PurgeExpiredTokens(ctx)
				if err != nil {
					slog.Warn("failed to purge expired tokens", "error", err)
				} else if n > 0 {
					slog.Info("purged expired revoked tokens", "count", n)
				}
			}
		}
	}()
}

// --- JWT implementation (HS256 with stdlib) ---

// jwtHeader is the fixed base64url-encoded header for HS256.
var jwtHeader = base64URLEncode([]byte(`{"alg":"HS256","typ":"JWT"}`))

func (s *AuthService) signJWT(u *user.User) (string, error) {
	now := time.Now()
	claims := user.TokenClaims{
		UserID:             u.ID,
		Email:              u.Email,
		Name:               u.Name,
		Role:               u.Role,
		TenantID:           u.TenantID,
		IssuedAt:           now.Unix(),
		Expiry:             now.Add(s.cfg.AccessTokenExpiry).Unix(),
		JTI:                generateID(),
		Audience:           "codeforge",
		Issuer:             "codeforge-core",
		MustChangePassword: u.MustChangePassword,
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}

	payloadB64 := base64URLEncode(payload)
	signingInput := jwtHeader + "." + payloadB64

	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(signingInput))
	sig := base64URLEncode(mac.Sum(nil))

	return signingInput + "." + sig, nil
}

func (s *AuthService) verifyJWT(tokenStr string) (*user.TokenClaims, error) {
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}

	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(signingInput))
	expectedSig := base64URLEncode(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, errors.New("invalid signature")
	}

	payload, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	var claims user.TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}

	if time.Now().Unix() > claims.Expiry {
		return nil, errors.New("token expired")
	}

	if claims.Audience != "codeforge" {
		return nil, errors.New("invalid token audience")
	}
	if claims.Issuer != "codeforge-core" {
		return nil, errors.New("invalid token issuer")
	}

	return &claims, nil
}

// --- Helpers ---

func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

func base64URLDecode(s string) ([]byte, error) {
	// Add padding back
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

func hashSHA256(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

func generateRandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// generateID produces a UUID v4 string using crypto/rand.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
