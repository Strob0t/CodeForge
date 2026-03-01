package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/settings"
)

// ListSettings returns all settings for the current tenant.
func (s *Store) ListSettings(ctx context.Context) ([]settings.Setting, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT key, value, updated_at FROM settings WHERE tenant_id = $1 ORDER BY key`,
		tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}
	defer rows.Close()

	var result []settings.Setting
	for rows.Next() {
		var st settings.Setting
		if err := rows.Scan(&st.Key, &st.Value, &st.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan setting: %w", err)
		}
		result = append(result, st)
	}
	return result, rows.Err()
}

// GetSetting returns a single setting by key for the current tenant.
func (s *Store) GetSetting(ctx context.Context, key string) (*settings.Setting, error) {
	var st settings.Setting
	err := s.pool.QueryRow(ctx,
		`SELECT key, value, updated_at FROM settings WHERE key = $1 AND tenant_id = $2`,
		key, tenantFromCtx(ctx)).Scan(&st.Key, &st.Value, &st.UpdatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get setting %s", key)
	}
	return &st, nil
}

// UpsertSetting inserts or updates a single setting for the current tenant.
func (s *Store) UpsertSetting(ctx context.Context, key string, value json.RawMessage) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO settings (tenant_id, key, value, updated_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT (key, tenant_id) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		tenantFromCtx(ctx), key, value)
	if err != nil {
		return fmt.Errorf("upsert setting %s: %w", key, err)
	}
	return nil
}
