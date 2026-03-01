package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/user"
)

func (s *Store) CreateRefreshToken(ctx context.Context, rt *user.RefreshToken) error {
	rt.CreatedAt = time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		rt.ID, rt.UserID, rt.TokenHash, rt.ExpiresAt, rt.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

func (s *Store) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*user.RefreshToken, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, token_hash, expires_at, created_at
		FROM refresh_tokens WHERE token_hash = $1`, tokenHash)

	var rt user.RefreshToken
	err := row.Scan(&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.CreatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get refresh token")
	}
	return &rt, nil
}

// getRefreshTokenByHashForUpdate retrieves a refresh token with a row-level lock
// to prevent concurrent rotation of the same token.
func (s *Store) getRefreshTokenByHashForUpdate(ctx context.Context, tx pgx.Tx, tokenHash string) (*user.RefreshToken, error) {
	row := tx.QueryRow(ctx, `
		SELECT id, user_id, token_hash, expires_at, created_at
		FROM refresh_tokens WHERE token_hash = $1 FOR UPDATE`, tokenHash)

	var rt user.RefreshToken
	err := row.Scan(&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.CreatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get refresh token")
	}
	return &rt, nil
}

func (s *Store) DeleteRefreshToken(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}
	return nil
}

func (s *Store) DeleteRefreshTokensByUser(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete refresh tokens by user: %w", err)
	}
	return nil
}

// RotateRefreshToken atomically locks the old token by hash, deletes it, and creates
// a new one in a single transaction. The SELECT ... FOR UPDATE prevents concurrent
// rotation of the same token (refresh token replay protection).
func (s *Store) RotateRefreshToken(ctx context.Context, oldTokenHash string, newRT *user.RefreshToken) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the old token row to prevent concurrent rotation
	oldRT, err := s.getRefreshTokenByHashForUpdate(ctx, tx, oldTokenHash)
	if err != nil {
		return fmt.Errorf("lock old token: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, oldRT.ID); err != nil {
		return fmt.Errorf("delete old refresh token: %w", err)
	}

	newRT.CreatedAt = time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		newRT.ID, newRT.UserID, newRT.TokenHash, newRT.ExpiresAt, newRT.CreatedAt,
	); err != nil {
		return fmt.Errorf("create new refresh token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rotate: %w", err)
	}
	return nil
}
