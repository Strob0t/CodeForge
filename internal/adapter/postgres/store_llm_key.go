package postgres

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/llmkey"
)

// --- User LLM Key CRUD ---

func (s *Store) CreateLLMKey(ctx context.Context, key *llmkey.LLMKey) error {
	tid := tenantFromCtx(ctx)
	return s.pool.QueryRow(ctx,
		`INSERT INTO user_llm_keys (user_id, tenant_id, provider, label, encrypted_key, key_prefix)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at, updated_at`,
		key.UserID, tid, key.Provider, key.Label, key.EncryptedKey, key.KeyPrefix,
	).Scan(&key.ID, &key.CreatedAt, &key.UpdatedAt)
}

func (s *Store) ListLLMKeysByUser(ctx context.Context, userID string) ([]llmkey.LLMKey, error) {
	tid := tenantFromCtx(ctx)
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, tenant_id, provider, label, encrypted_key, key_prefix, created_at, updated_at
		 FROM user_llm_keys WHERE user_id = $1 AND tenant_id = $2 ORDER BY created_at ASC`,
		userID, tid)
	if err != nil {
		return nil, fmt.Errorf("list llm keys: %w", err)
	}
	defer rows.Close()

	var keys []llmkey.LLMKey
	for rows.Next() {
		var k llmkey.LLMKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.TenantID, &k.Provider, &k.Label,
			&k.EncryptedKey, &k.KeyPrefix, &k.CreatedAt, &k.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan llm key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) GetLLMKeyByUserProvider(ctx context.Context, userID, provider string) (*llmkey.LLMKey, error) {
	tid := tenantFromCtx(ctx)
	var k llmkey.LLMKey
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, tenant_id, provider, label, encrypted_key, key_prefix, created_at, updated_at
		 FROM user_llm_keys WHERE user_id = $1 AND provider = $2 AND tenant_id = $3`,
		userID, provider, tid,
	).Scan(&k.ID, &k.UserID, &k.TenantID, &k.Provider, &k.Label,
		&k.EncryptedKey, &k.KeyPrefix, &k.CreatedAt, &k.UpdatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get llm key user=%s provider=%s", userID, provider)
	}
	return &k, nil
}

func (s *Store) DeleteLLMKey(ctx context.Context, id, userID string) error {
	tid := tenantFromCtx(ctx)
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM user_llm_keys WHERE id = $1 AND user_id = $2 AND tenant_id = $3`,
		id, userID, tid)
	return execExpectOne(tag, err, "delete llm key %s", id)
}
