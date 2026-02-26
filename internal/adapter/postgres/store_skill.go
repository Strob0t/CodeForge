package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/skill"
)

// CreateSkill inserts a new skill.
func (s *Store) CreateSkill(ctx context.Context, sk *skill.Skill) error {
	const q = `
		INSERT INTO skills (tenant_id, project_id, name, description, language, code, tags, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`

	tags := pgTextArray(sk.Tags)
	return s.pool.QueryRow(ctx, q,
		tenantFromCtx(ctx), sk.ProjectID, sk.Name, sk.Description,
		sk.Language, sk.Code, tags, sk.Enabled,
	).Scan(&sk.ID, &sk.CreatedAt)
}

// GetSkill retrieves a skill by ID.
func (s *Store) GetSkill(ctx context.Context, id string) (*skill.Skill, error) {
	const q = `
		SELECT id, tenant_id, project_id, name, description, language, code, tags, enabled, created_at
		FROM skills
		WHERE id = $1 AND tenant_id = $2`

	var sk skill.Skill
	err := s.pool.QueryRow(ctx, q, id, tenantFromCtx(ctx)).Scan(
		&sk.ID, &sk.TenantID, &sk.ProjectID, &sk.Name, &sk.Description,
		&sk.Language, &sk.Code, &sk.Tags, &sk.Enabled, &sk.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get skill %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get skill %s: %w", id, err)
	}
	return &sk, nil
}

// ListSkills returns all skills for a project (empty projectID = global).
func (s *Store) ListSkills(ctx context.Context, projectID string) ([]skill.Skill, error) {
	const q = `
		SELECT id, tenant_id, project_id, name, description, language, code, tags, enabled, created_at
		FROM skills
		WHERE (project_id = $1 OR project_id = '') AND tenant_id = $2
		ORDER BY created_at ASC`

	rows, err := s.pool.Query(ctx, q, projectID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()

	var result []skill.Skill
	for rows.Next() {
		var sk skill.Skill
		if err := rows.Scan(
			&sk.ID, &sk.TenantID, &sk.ProjectID, &sk.Name, &sk.Description,
			&sk.Language, &sk.Code, &sk.Tags, &sk.Enabled, &sk.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		result = append(result, sk)
	}
	return result, rows.Err()
}

// UpdateSkill updates a skill in the database.
func (s *Store) UpdateSkill(ctx context.Context, sk *skill.Skill) error {
	const q = `
		UPDATE skills
		SET name = $2, description = $3, language = $4, code = $5, tags = $6, enabled = $7
		WHERE id = $1 AND tenant_id = $8`

	tags := pgTextArray(sk.Tags)
	tag, err := s.pool.Exec(ctx, q,
		sk.ID, sk.Name, sk.Description, sk.Language, sk.Code, tags, sk.Enabled,
		tenantFromCtx(ctx),
	)
	if err != nil {
		return fmt.Errorf("update skill %s: %w", sk.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update skill %s: %w", sk.ID, domain.ErrNotFound)
	}
	return nil
}

// DeleteSkill removes a skill by ID.
func (s *Store) DeleteSkill(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM skills WHERE id = $1 AND tenant_id = $2`,
		id, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("delete skill %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete skill %s: %w", id, domain.ErrNotFound)
	}
	return nil
}
