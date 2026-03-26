package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/crypto"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// AuthService handles authentication, JWT tokens, and API keys.
// It composes TokenManager and APIKeyManager sub-services.
type AuthService struct {
	store   database.Store
	cfg     *config.Auth
	secret  []byte
	tokens  *TokenManager
	apiKeys *APIKeyManager
}

// NewAuthService creates a new authentication service with sub-services.
func NewAuthService(store database.Store, cfg *config.Auth) *AuthService {
	return &AuthService{
		store:   store,
		cfg:     cfg,
		secret:  []byte(cfg.JWTSecret),
		tokens:  NewTokenManager(store, cfg),
		apiKeys: NewAPIKeyManager(store),
	}
}

// Tokens returns the token manager sub-service.
func (s *AuthService) Tokens() *TokenManager { return s.tokens }

// APIKeys returns the API key manager sub-service.
func (s *AuthService) APIKeys() *APIKeyManager { return s.apiKeys }

// Register creates a new user with a bcrypt-hashed password.
// If the requested role is empty or not in the valid set, it defaults to viewer.
func (s *AuthService) Register(ctx context.Context, req *user.CreateRequest) (*user.User, error) {
	return s.register(ctx, req, false)
}

// RegisterFirstUser atomically creates a user only if no users exist for the tenant.
// Returns domain.ErrConflict if any user already exists (setup already done).
func (s *AuthService) RegisterFirstUser(ctx context.Context, req *user.CreateRequest) (*user.User, error) {
	return s.register(ctx, req, true)
}

// register is the shared implementation for Register and RegisterFirstUser.
func (s *AuthService) register(ctx context.Context, req *user.CreateRequest, firstOnly bool) (*user.User, error) {
	if req.Role == "" {
		req.Role = user.RoleViewer
	}

	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.cfg.BcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u := &user.User{
		ID:           crypto.GenerateUUIDv4(),
		Email:        req.Email,
		Name:         req.Name,
		PasswordHash: string(hash),
		Role:         req.Role,
		TenantID:     req.TenantID,
		Enabled:      true,
	}

	if firstOnly {
		if err := s.store.CreateFirstUser(ctx, u); err != nil {
			return nil, fmt.Errorf("create first user: %w", err)
		}
	} else {
		if err := s.store.CreateUser(ctx, u); err != nil {
			return nil, fmt.Errorf("create user: %w", err)
		}
	}
	return u, nil
}

// Login authenticates a user and returns an access token + refresh token hash.
// Accounts are temporarily locked after 5 consecutive failed attempts (15 min lockout).
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

	// Check account lockout.
	if u.IsLocked() {
		return nil, "", errors.New("account is temporarily locked, try again later")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		// Increment failed attempts and potentially lock the account.
		u.FailedAttempts++
		if u.FailedAttempts >= user.MaxFailedAttempts {
			u.LockedUntil = time.Now().Add(user.LockoutDuration)
			slog.Warn("account locked due to failed login attempts",
				"email", u.Email, "attempts", u.FailedAttempts)
		}
		if updateErr := s.store.UpdateUser(ctx, u); updateErr != nil {
			slog.Error("failed to update user lockout state", "error", updateErr)
		}
		return nil, "", errors.New("invalid credentials")
	}

	// Successful login: reset failed attempts and lockout.
	if u.FailedAttempts > 0 || !u.LockedUntil.IsZero() {
		u.FailedAttempts = 0
		u.LockedUntil = time.Time{}
		if updateErr := s.store.UpdateUser(ctx, u); updateErr != nil {
			slog.Error("failed to reset user lockout state", "error", updateErr)
		}
	}

	accessToken, err := s.tokens.SignJWT(u)
	if err != nil {
		return nil, "", fmt.Errorf("sign jwt: %w", err)
	}

	// Create refresh token
	rawToken, err := crypto.GenerateRandomToken()
	if err != nil {
		return nil, "", fmt.Errorf("generate refresh token: %w", err)
	}

	tokenHash := crypto.HashSHA256(rawToken)
	rt := &user.RefreshToken{
		ID:        crypto.GenerateUUIDv4(),
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

// RefreshTokens delegates to TokenManager.
func (s *AuthService) RefreshTokens(ctx context.Context, rawToken string) (*user.LoginResponse, string, error) {
	return s.tokens.RefreshTokens(ctx, rawToken)
}

// Logout delegates to TokenManager.
func (s *AuthService) Logout(ctx context.Context, userID, jti string, tokenExpiry time.Time) error {
	return s.tokens.Logout(ctx, userID, jti, tokenExpiry)
}

// RevokeAccessToken delegates to TokenManager.
func (s *AuthService) RevokeAccessToken(ctx context.Context, jti string, expiresAt time.Time) error {
	return s.tokens.RevokeAccessToken(ctx, jti, expiresAt)
}

// ValidateAccessToken delegates to TokenManager.
func (s *AuthService) ValidateAccessToken(tokenStr string) (*user.TokenClaims, error) {
	return s.tokens.ValidateAccessToken(tokenStr)
}

// ValidateAPIKey delegates to APIKeyManager.
func (s *AuthService) ValidateAPIKey(ctx context.Context, rawKey string) (*user.User, *user.APIKey, error) {
	return s.apiKeys.ValidateAPIKey(ctx, rawKey)
}

// CreateAPIKey delegates to APIKeyManager.
func (s *AuthService) CreateAPIKey(ctx context.Context, userID string, req user.CreateAPIKeyRequest) (*user.CreateAPIKeyResponse, error) {
	return s.apiKeys.CreateAPIKey(ctx, userID, req)
}

// ListAPIKeys delegates to APIKeyManager.
func (s *AuthService) ListAPIKeys(ctx context.Context, userID string) ([]user.APIKey, error) {
	return s.apiKeys.ListAPIKeys(ctx, userID)
}

// DeleteAPIKey delegates to APIKeyManager.
func (s *AuthService) DeleteAPIKey(ctx context.Context, id, userID string) error {
	return s.apiKeys.DeleteAPIKey(ctx, id, userID)
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

// SetupStatus represents the initial setup state of the system.
type SetupStatus struct {
	NeedsSetup          bool `json:"needs_setup"`
	SetupTimeoutMinutes int  `json:"setup_timeout_minutes"`
}

// GetSetupStatus checks if the system needs initial setup (no users exist).
func (s *AuthService) GetSetupStatus(ctx context.Context, tenantID string) (*SetupStatus, error) {
	users, err := s.store.ListUsers(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return &SetupStatus{
		NeedsSetup:          len(users) == 0,
		SetupTimeoutMinutes: s.cfg.SetupTimeoutMinutes,
	}, nil
}

// SeedDefaultAdmin is an alias for BootstrapAdmin for backward compatibility.
func (s *AuthService) SeedDefaultAdmin(ctx context.Context, tenantID string) error {
	return s.BootstrapAdmin(ctx, tenantID)
}

// BootstrapAdmin creates the initial admin user using one of three paths:
// 1. If DefaultAdminPass is set, create admin with that password.
// 2. If AutoGenerateInitialPassword is true, generate a random password and write to file.
// 3. Otherwise, log "waiting for setup wizard" and return (no user created).
func (s *AuthService) BootstrapAdmin(ctx context.Context, tenantID string) error {
	users, err := s.store.ListUsers(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	if len(users) > 0 {
		// If an explicit password is configured via env var, sync it to the admin user
		// so F5-debugging always works with the configured credentials.
		if s.cfg.DefaultAdminPass != "" {
			return s.syncAdminPassword(ctx, tenantID)
		}
		return nil // already bootstrapped
	}

	// Path 1: explicit password from config/env
	if s.cfg.DefaultAdminPass != "" {
		return s.createAdminWithPassword(ctx, tenantID, s.cfg.DefaultAdminPass)
	}

	// Path 2: auto-generate password to file
	if s.cfg.AutoGenerateInitialPassword {
		password, err := crypto.GenerateRandomPassword(24)
		if err != nil {
			return fmt.Errorf("generate initial password: %w", err)
		}

		if err := writePasswordFile(s.cfg.InitialPasswordFile, password); err != nil {
			return fmt.Errorf("write initial password file: %w", err)
		}

		if err := s.createAdminWithPassword(ctx, tenantID, password); err != nil {
			return err
		}

		slog.Warn("initial admin password written to file — change it on first login",
			"file", s.cfg.InitialPasswordFile,
			"email", s.cfg.DefaultAdminEmail)
		return nil
	}

	// Path 3: no password configured — wait for setup wizard
	slog.Info("no admin password configured, waiting for setup wizard",
		"email", s.cfg.DefaultAdminEmail)
	return nil
}

// createAdminWithPassword creates the admin user with the given password and sets must_change_password.
func (s *AuthService) createAdminWithPassword(ctx context.Context, tenantID, password string) error {
	u, err := s.Register(ctx, &user.CreateRequest{
		Email:    s.cfg.DefaultAdminEmail,
		Name:     "Admin",
		Password: password,
		Role:     user.RoleAdmin,
		TenantID: tenantID,
	})
	if err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}

	u.MustChangePassword = true
	if err := s.store.UpdateUser(ctx, u); err != nil {
		return fmt.Errorf("set must_change_password: %w", err)
	}

	slog.Info("bootstrapped admin user", "email", s.cfg.DefaultAdminEmail)
	return nil
}

// syncAdminPassword ensures the default admin user's password matches the configured env var.
func (s *AuthService) syncAdminPassword(ctx context.Context, tenantID string) error {
	u, err := s.store.GetUserByEmail(ctx, s.cfg.DefaultAdminEmail, tenantID)
	if err != nil {
		return nil // admin user doesn't exist (different email), skip
	}

	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(s.cfg.DefaultAdminPass)) == nil {
		return nil // password already matches
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(s.cfg.DefaultAdminPass), s.cfg.BcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	u.PasswordHash = string(hash)
	u.FailedAttempts = 0
	u.LockedUntil = time.Time{}
	if err := s.store.UpdateUser(ctx, u); err != nil {
		return fmt.Errorf("sync admin password: %w", err)
	}

	slog.Info("admin password synced from env var", "email", s.cfg.DefaultAdminEmail)
	return nil
}

// AdminResetPassword resets a user's password by email without requiring the old password.
// It clears must_change_password, failed_attempts, locked_until, and invalidates all sessions.
func (s *AuthService) AdminResetPassword(ctx context.Context, email, tenantID, newPassword string) error {
	if err := user.ValidatePasswordComplexity(newPassword); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	u, err := s.store.GetUserByEmail(ctx, email, tenantID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), s.cfg.BcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	u.PasswordHash = string(hash)
	u.MustChangePassword = false
	u.FailedAttempts = 0
	u.LockedUntil = time.Time{}

	if err := s.store.UpdateUser(ctx, u); err != nil {
		return fmt.Errorf("update user: %w", err)
	}

	// Invalidate all sessions for this user
	if err := s.store.DeleteRefreshTokensByUser(ctx, u.ID); err != nil {
		slog.Warn("failed to invalidate sessions after admin password reset", "user_id", u.ID, "error", err)
	}

	slog.Info("admin password reset completed", "email", email)
	return nil
}

// RequestPasswordReset generates a password reset token for the given email.
// Returns empty string and nil error for unknown emails to prevent user enumeration.
func (s *AuthService) RequestPasswordReset(ctx context.Context, email, tenantID string) (string, error) {
	u, err := s.store.GetUserByEmail(ctx, email, tenantID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", nil // prevent enumeration
		}
		return "", fmt.Errorf("get user: %w", err)
	}

	rawToken, err := crypto.GenerateRandomToken()
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}

	tokenHash := crypto.HashSHA256(rawToken)
	prt := &user.PasswordResetToken{
		ID:        crypto.GenerateUUIDv4(),
		UserID:    u.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		CreatedAt: time.Now().UTC(),
	}

	if err := s.store.CreatePasswordResetToken(ctx, prt); err != nil {
		return "", fmt.Errorf("store reset token: %w", err)
	}

	return rawToken, nil
}

// ConfirmPasswordReset validates a password reset token and sets a new password.
// Marks the token as used and invalidates all sessions.
func (s *AuthService) ConfirmPasswordReset(ctx context.Context, rawToken, newPassword string) error {
	if err := user.ValidatePasswordComplexity(newPassword); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	tokenHash := crypto.HashSHA256(rawToken)
	prt, err := s.store.GetPasswordResetTokenByHash(ctx, tokenHash)
	if err != nil {
		return fmt.Errorf("%w: invalid or expired reset token", domain.ErrValidation)
	}

	if prt.Used {
		return fmt.Errorf("%w: reset token has already been used", domain.ErrValidation)
	}
	if time.Now().After(prt.ExpiresAt) {
		return fmt.Errorf("%w: reset token has expired", domain.ErrValidation)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), s.cfg.BcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	u, err := s.store.GetUser(ctx, prt.UserID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	u.PasswordHash = string(hash)
	u.MustChangePassword = false
	u.FailedAttempts = 0
	u.LockedUntil = time.Time{}

	if err := s.store.UpdateUser(ctx, u); err != nil {
		return fmt.Errorf("update user: %w", err)
	}

	if err := s.store.MarkPasswordResetTokenUsed(ctx, prt.ID); err != nil {
		slog.Warn("failed to mark reset token as used", "token_id", prt.ID, "error", err)
	}

	// Invalidate all sessions
	if err := s.store.DeleteRefreshTokensByUser(ctx, u.ID); err != nil {
		slog.Warn("failed to invalidate sessions after password reset", "user_id", u.ID, "error", err)
	}

	slog.Info("password reset completed via token", "user_id", u.ID)
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

	// Clean up initial password file if it exists
	if s.cfg.InitialPasswordFile != "" {
		if err := os.Remove(s.cfg.InitialPasswordFile); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove initial password file", "path", s.cfg.InitialPasswordFile, "error", err)
		}
	}

	return nil
}

// StartTokenCleanup delegates to TokenManager.
func (s *AuthService) StartTokenCleanup(ctx context.Context, interval time.Duration) {
	s.tokens.StartTokenCleanup(ctx, interval)
}

// Note: JWT signing/verification, base64URLEncode/Decode, and jwtHeader
// are now in auth_token.go (TokenManager). The AuthService delegates via
// s.tokens.SignJWT() and s.tokens.ValidateAccessToken().

// writePasswordFile writes the password to a file, creating parent directories as needed.
func writePasswordFile(path, password string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}
	return os.WriteFile(path, []byte(password+"\n"), 0o600)
}
