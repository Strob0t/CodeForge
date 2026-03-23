package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/crypto"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// TokenManager handles JWT signing/verification, refresh token rotation,
// token revocation, and background cleanup of expired tokens.
type TokenManager struct {
	store  database.Store
	secret []byte
	cfg    *config.Auth
}

// NewTokenManager creates a token manager.
func NewTokenManager(store database.Store, cfg *config.Auth) *TokenManager {
	return &TokenManager{
		store:  store,
		secret: []byte(cfg.JWTSecret),
		cfg:    cfg,
	}
}

// RefreshTokens validates a refresh token, atomically rotates it, and issues a new access token.
func (t *TokenManager) RefreshTokens(ctx context.Context, rawToken string) (*user.LoginResponse, string, error) {
	tokenHash := crypto.HashSHA256(rawToken)

	rt, err := t.store.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, "", errors.New("invalid refresh token")
	}

	if time.Now().After(rt.ExpiresAt) {
		logBestEffort(ctx, t.store.DeleteRefreshToken(ctx, rt.ID), "DeleteRefreshToken")
		return nil, "", errors.New("refresh token expired")
	}

	u, err := t.store.GetUser(ctx, rt.UserID)
	if err != nil {
		return nil, "", fmt.Errorf("get user: %w", err)
	}

	if !u.Enabled {
		return nil, "", errors.New("account is disabled")
	}

	accessToken, err := t.signJWT(u)
	if err != nil {
		return nil, "", fmt.Errorf("sign jwt: %w", err)
	}

	// Issue new refresh token via atomic rotation.
	newRawToken, err := crypto.GenerateRandomToken()
	if err != nil {
		return nil, "", fmt.Errorf("generate refresh token: %w", err)
	}

	newRT := &user.RefreshToken{
		ID:        crypto.GenerateUUIDv4(),
		UserID:    u.ID,
		TokenHash: crypto.HashSHA256(newRawToken),
		ExpiresAt: time.Now().Add(t.cfg.RefreshTokenExpiry),
	}

	if err := t.store.RotateRefreshToken(ctx, tokenHash, newRT); err != nil {
		return nil, "", fmt.Errorf("rotate refresh token: %w", err)
	}

	resp := &user.LoginResponse{
		AccessToken: accessToken,
		ExpiresIn:   int(t.cfg.AccessTokenExpiry.Seconds()),
		User:        *u,
	}
	return resp, newRawToken, nil
}

// Logout deletes all refresh tokens for a user and optionally revokes the
// current access token by JTI.
func (t *TokenManager) Logout(ctx context.Context, userID, jti string, tokenExpiry time.Time) error {
	if jti != "" {
		if err := t.store.RevokeToken(ctx, jti, tokenExpiry); err != nil {
			slog.Warn("failed to revoke access token on logout", "jti", jti, "error", err)
		}
	}
	return t.store.DeleteRefreshTokensByUser(ctx, userID)
}

// RevokeAccessToken adds a token JTI to the revocation blacklist.
func (t *TokenManager) RevokeAccessToken(ctx context.Context, jti string, expiresAt time.Time) error {
	return t.store.RevokeToken(ctx, jti, expiresAt)
}

// ValidateAccessToken verifies a JWT and returns the claims.
// It checks token revocation when a JTI is present (fail-closed on DB error).
func (t *TokenManager) ValidateAccessToken(tokenStr string) (*user.TokenClaims, error) {
	claims, err := t.verifyJWT(tokenStr)
	if err != nil {
		return nil, err
	}

	if claims.JTI != "" {
		revoked, dbErr := t.store.IsTokenRevoked(context.Background(), claims.JTI)
		if dbErr != nil {
			slog.Error("token revocation check failed, denying token", "jti", claims.JTI, "error", dbErr)
			return nil, errors.New("unable to verify token status")
		}
		if revoked {
			return nil, errors.New("token has been revoked")
		}
	}

	return claims, nil
}

// StartTokenCleanup starts a background goroutine that periodically purges
// expired revoked tokens and expired password reset tokens.
func (t *TokenManager) StartTokenCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := t.store.PurgeExpiredTokens(ctx)
				if err != nil {
					slog.Warn("failed to purge expired tokens", "error", err)
				} else if n > 0 {
					slog.Info("purged expired revoked tokens", "count", n)
				}

				rn, err := t.store.DeleteExpiredPasswordResetTokens(ctx)
				if err != nil {
					slog.Warn("failed to purge expired password reset tokens", "error", err)
				} else if rn > 0 {
					slog.Info("purged expired password reset tokens", "count", rn)
				}
			}
		}
	}()
}

// SignJWT creates a signed JWT for the given user.
func (t *TokenManager) SignJWT(u *user.User) (string, error) {
	return t.signJWT(u)
}

// --- JWT implementation (HS256 with stdlib) ---

// jwtHeader is the fixed base64url-encoded header for HS256.
var jwtHeader = base64URLEncode([]byte(`{"alg":"HS256","typ":"JWT"}`))

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

func (t *TokenManager) signJWT(u *user.User) (string, error) {
	now := time.Now()
	claims := user.TokenClaims{
		UserID:             u.ID,
		Email:              u.Email,
		Name:               u.Name,
		Role:               u.Role,
		TenantID:           u.TenantID,
		IssuedAt:           now.Unix(),
		Expiry:             now.Add(t.cfg.AccessTokenExpiry).Unix(),
		JTI:                crypto.GenerateUUIDv4(),
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

	mac := hmac.New(sha256.New, t.secret)
	mac.Write([]byte(signingInput))
	sig := base64URLEncode(mac.Sum(nil))

	return signingInput + "." + sig, nil
}

func (t *TokenManager) verifyJWT(tokenStr string) (*user.TokenClaims, error) {
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}

	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, t.secret)
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
