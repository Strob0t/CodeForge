package postgres

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/microagent"
)

// CreateMicroagent inserts a new microagent.
func (s *Store) CreateMicroagent(ctx context.Context, m *microagent.Microagent) error {
	const q = `
		INSERT INTO microagents (tenant_id, project_id, name, type, trigger_pattern, description, prompt, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`

	return s.pool.QueryRow(ctx, q,
		tenantFromCtx(ctx), m.ProjectID, m.Name, string(m.Type),
		m.TriggerPattern, m.Description, m.Prompt, m.Enabled,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
}

// GetMicroagent retrieves a microagent by ID.
func (s *Store) GetMicroagent(ctx context.Context, id string) (*microagent.Microagent, error) {
	const q = `
		SELECT id, tenant_id, project_id, name, type, trigger_pattern, description, prompt, enabled, created_at, updated_at
		FROM microagents
		WHERE id = $1 AND tenant_id = $2`

	var m microagent.Microagent
	err := s.pool.QueryRow(ctx, q, id, tenantFromCtx(ctx)).Scan(
		&m.ID, &m.TenantID, &m.ProjectID, &m.Name, &m.Type,
		&m.TriggerPattern, &m.Description, &m.Prompt, &m.Enabled,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, notFoundWrap(err, "get microagent %s", id)
	}
	return &m, nil
}

// ListMicroagents returns all microagents for a project (empty projectID = global).
func (s *Store) ListMicroagents(ctx context.Context, projectID string) ([]microagent.Microagent, error) {
	const q = `
		SELECT id, tenant_id, project_id, name, type, trigger_pattern, description, prompt, enabled, created_at, updated_at
		FROM microagents
		WHERE (project_id = $1 OR project_id = '') AND tenant_id = $2
		ORDER BY created_at ASC`

	rows, err := s.pool.Query(ctx, q, projectID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list microagents: %w", err)
	}
	defer rows.Close()

	var result []microagent.Microagent
	for rows.Next() {
		var m microagent.Microagent
		if err := rows.Scan(
			&m.ID, &m.TenantID, &m.ProjectID, &m.Name, &m.Type,
			&m.TriggerPattern, &m.Description, &m.Prompt, &m.Enabled,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan microagent: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// UpdateMicroagent updates a microagent in the database.
func (s *Store) UpdateMicroagent(ctx context.Context, m *microagent.Microagent) error {
	const q = `
		UPDATE microagents
		SET name = $2, trigger_pattern = $3, description = $4, prompt = $5, enabled = $6, updated_at = now()
		WHERE id = $1 AND tenant_id = $7`

	tag, err := s.pool.Exec(ctx, q,
		m.ID, m.Name, m.TriggerPattern, m.Description, m.Prompt, m.Enabled,
		tenantFromCtx(ctx),
	)
	return execExpectOne(tag, err, "update microagent %s", m.ID)
}

// DeleteMicroagent removes a microagent by ID.
func (s *Store) DeleteMicroagent(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM microagents WHERE id = $1 AND tenant_id = $2`,
		id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete microagent %s", id)
}
