package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/experience"
)

// CreateExperienceEntry inserts a new experience entry.
func (s *Store) CreateExperienceEntry(ctx context.Context, e *experience.Entry) error {
	const q = `
		INSERT INTO experience_entries (tenant_id, project_id, task_description, task_embedding, result_output, result_cost, result_status, run_id, confidence)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, hit_count, created_at, last_used_at`

	return s.pool.QueryRow(ctx, q,
		tenantFromCtx(ctx), e.ProjectID, e.TaskDescription, e.TaskEmbedding,
		e.ResultOutput, e.ResultCost, e.ResultStatus, e.RunID, e.Confidence,
	).Scan(&e.ID, &e.HitCount, &e.CreatedAt, &e.LastUsedAt)
}

// GetExperienceEntry retrieves an experience entry by ID.
func (s *Store) GetExperienceEntry(ctx context.Context, id string) (*experience.Entry, error) {
	const q = `
		SELECT id, tenant_id, project_id, task_description, result_output, result_cost,
		       result_status, run_id, confidence, hit_count, created_at, last_used_at
		FROM experience_entries
		WHERE id = $1 AND tenant_id = $2`

	var e experience.Entry
	err := s.pool.QueryRow(ctx, q, id, tenantFromCtx(ctx)).Scan(
		&e.ID, &e.TenantID, &e.ProjectID, &e.TaskDescription,
		&e.ResultOutput, &e.ResultCost, &e.ResultStatus, &e.RunID,
		&e.Confidence, &e.HitCount, &e.CreatedAt, &e.LastUsedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get experience %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get experience %s: %w", id, err)
	}
	return &e, nil
}

// ListExperienceEntries returns all experience entries for a project.
func (s *Store) ListExperienceEntries(ctx context.Context, projectID string) ([]experience.Entry, error) {
	const q = `
		SELECT id, tenant_id, project_id, task_description, result_output, result_cost,
		       result_status, run_id, confidence, hit_count, created_at, last_used_at
		FROM experience_entries
		WHERE project_id = $1 AND tenant_id = $2
		ORDER BY last_used_at DESC`

	rows, err := s.pool.Query(ctx, q, projectID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list experience entries: %w", err)
	}
	defer rows.Close()

	var result []experience.Entry
	for rows.Next() {
		var e experience.Entry
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.ProjectID, &e.TaskDescription,
			&e.ResultOutput, &e.ResultCost, &e.ResultStatus, &e.RunID,
			&e.Confidence, &e.HitCount, &e.CreatedAt, &e.LastUsedAt,
		); err != nil {
			return nil, fmt.Errorf("scan experience entry: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// DeleteExperienceEntry removes an experience entry by ID.
func (s *Store) DeleteExperienceEntry(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM experience_entries WHERE id = $1 AND tenant_id = $2`,
		id, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("delete experience %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete experience %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

// UpdateExperienceHit increments the hit count and updates last_used_at.
func (s *Store) UpdateExperienceHit(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE experience_entries SET hit_count = hit_count + 1, last_used_at = now()
		 WHERE id = $1 AND tenant_id = $2`,
		id, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("update experience hit %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update experience hit %s: %w", id, domain.ErrNotFound)
	}
	return nil
}
