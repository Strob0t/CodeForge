package database

import (
	"context"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/user"
)

// AuthTokenStore defines database operations for refresh tokens, password reset
// tokens, API keys, and token revocation.
type AuthTokenStore interface {
	// Refresh Tokens
	CreateRefreshToken(ctx context.Context, rt *user.RefreshToken) error
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*user.RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, id string) error
	DeleteRefreshTokensByUser(ctx context.Context, userID string) error

	// Password Reset Tokens
	CreatePasswordResetToken(ctx context.Context, token *user.PasswordResetToken) error
	GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (*user.PasswordResetToken, error)
	MarkPasswordResetTokenUsed(ctx context.Context, id string) error
	DeleteExpiredPasswordResetTokens(ctx context.Context) (int64, error)

	// API Keys
	CreateAPIKey(ctx context.Context, key *user.APIKey) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*user.APIKey, error)
	ListAPIKeysByUser(ctx context.Context, userID string) ([]user.APIKey, error)
	DeleteAPIKey(ctx context.Context, id, userID string) error

	// Token Revocation
	RevokeToken(ctx context.Context, jti string, expiresAt time.Time) error
	IsTokenRevoked(ctx context.Context, jti string) (bool, error)
	PurgeExpiredTokens(ctx context.Context) (int64, error)

	// Atomic Refresh Token Rotation
	RotateRefreshToken(ctx context.Context, oldTokenHash string, newRT *user.RefreshToken) error
}
