package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/user"
)

func (s *Store) CreateAPIKey(ctx context.Context, key *user.APIKey) error {
	key.CreatedAt = time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO api_keys (id, user_id, name, prefix, key_hash, scopes, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		key.ID, key.UserID, key.Name, key.Prefix, key.KeyHash, key.Scopes, nullTime(key.ExpiresAt), key.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	return nil
}

func (s *Store) GetAPIKeyByHash(ctx context.Context, keyHash string) (*user.APIKey, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, prefix, key_hash, scopes, expires_at, created_at
		FROM api_keys WHERE key_hash = $1`, keyHash)

	var key user.APIKey
	var expiresAt sql.NullTime
	err := row.Scan(&key.ID, &key.UserID, &key.Name, &key.Prefix, &key.KeyHash, &key.Scopes, &expiresAt, &key.CreatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get api key")
	}
	if expiresAt.Valid {
		key.ExpiresAt = expiresAt.Time
	}
	return &key, nil
}

func (s *Store) ListAPIKeysByUser(ctx context.Context, userID string) ([]user.APIKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, prefix, key_hash, scopes, expires_at, created_at
		FROM api_keys WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []user.APIKey
	for rows.Next() {
		var key user.APIKey
		var expiresAt sql.NullTime
		if err := rows.Scan(&key.ID, &key.UserID, &key.Name, &key.Prefix, &key.KeyHash, &key.Scopes, &expiresAt, &key.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		if expiresAt.Valid {
			key.ExpiresAt = expiresAt.Time
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Store) DeleteAPIKey(ctx context.Context, id, userID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM api_keys WHERE id = $1 AND user_id = $2`, id, userID)
	return execExpectOne(tag, err, "delete api key %s", id)
}
