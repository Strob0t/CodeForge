package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) RevokeToken(ctx context.Context, jti string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO revoked_tokens (jti, expires_at)
		VALUES ($1, $2)
		ON CONFLICT (jti) DO NOTHING`,
		jti, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	return nil
}

func (s *Store) IsTokenRevoked(ctx context.Context, jti string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM revoked_tokens WHERE jti = $1)`, jti).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("check token revocation: %w", err)
	}
	return exists, nil
}

func (s *Store) PurgeExpiredTokens(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM revoked_tokens WHERE expires_at < $1`, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("purge expired tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}
