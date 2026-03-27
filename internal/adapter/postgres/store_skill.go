package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/skill"
)

// CreateSkill inserts a new skill.
//
//nolint:staticcheck // Code and Enabled are deprecated but must be persisted for backward compat.
func (s *Store) CreateSkill(ctx context.Context, sk *skill.Skill) error {
	const q = `
		INSERT INTO skills (tenant_id, project_id, name, type, description, language, content, code, tags, source, source_url, format_origin, status, usage_count, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, created_at`

	tags := pgTextArray(sk.Tags)
	return s.pool.QueryRow(ctx, q,
		tenantFromCtx(ctx), sk.ProjectID, sk.Name, sk.Type, sk.Description,
		sk.Language, sk.Content, sk.Code, tags, sk.Source, sk.SourceURL,
		sk.FormatOrigin, sk.Status, sk.UsageCount, sk.Enabled,
	).Scan(&sk.ID, &sk.CreatedAt)
}

// GetSkill retrieves a skill by ID.
//
//nolint:staticcheck // Code and Enabled are deprecated but must be scanned for backward compat.
func (s *Store) GetSkill(ctx context.Context, id string) (*skill.Skill, error) {
	const q = `
		SELECT id, tenant_id, project_id, name, type, description, language,
		       content, code, tags, source, source_url, format_origin,
		       status, usage_count, enabled, created_at
		FROM skills
		WHERE id = $1 AND tenant_id = $2`

	var sk skill.Skill
	err := s.pool.QueryRow(ctx, q, id, tenantFromCtx(ctx)).Scan(
		&sk.ID, &sk.TenantID, &sk.ProjectID, &sk.Name, &sk.Type, &sk.Description,
		&sk.Language, &sk.Content, &sk.Code, &sk.Tags, &sk.Source, &sk.SourceURL,
		&sk.FormatOrigin, &sk.Status, &sk.UsageCount, &sk.Enabled, &sk.CreatedAt,
	)
	if err != nil {
		return nil, notFoundWrap(err, "get skill %s", id)
	}
	return &sk, nil
}

// ListSkills returns all skills for a project (empty projectID = global).
//
//nolint:staticcheck // Code and Enabled are deprecated but must be scanned for backward compat.
func (s *Store) ListSkills(ctx context.Context, projectID string) ([]skill.Skill, error) {
	const q = `
		SELECT id, tenant_id, project_id, name, type, description, language,
		       content, code, tags, source, source_url, format_origin,
		       status, usage_count, enabled, created_at
		FROM skills
		WHERE (project_id = $1 OR project_id = '') AND tenant_id = $2
		ORDER BY created_at ASC
		LIMIT $3`

	rows, err := s.pool.Query(ctx, q, projectID, tenantFromCtx(ctx), DefaultListLimit)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (skill.Skill, error) {
		var sk skill.Skill
		err := r.Scan(
			&sk.ID, &sk.TenantID, &sk.ProjectID, &sk.Name, &sk.Type, &sk.Description,
			&sk.Language, &sk.Content, &sk.Code, &sk.Tags, &sk.Source, &sk.SourceURL,
			&sk.FormatOrigin, &sk.Status, &sk.UsageCount, &sk.Enabled, &sk.CreatedAt,
		)
		return sk, err
	})
}

// UpdateSkill updates a skill in the database.
//
//nolint:staticcheck // Code and Enabled are deprecated but must be persisted for backward compat.
func (s *Store) UpdateSkill(ctx context.Context, sk *skill.Skill) error {
	const q = `
		UPDATE skills
		SET name = $2, type = $3, description = $4, language = $5,
		    content = $6, code = $7, tags = $8, source = $9, source_url = $10,
		    format_origin = $11, status = $12, usage_count = $13, enabled = $14
		WHERE id = $1 AND tenant_id = $15`

	tags := pgTextArray(sk.Tags)
	tag, err := s.pool.Exec(ctx, q,
		sk.ID, sk.Name, sk.Type, sk.Description, sk.Language,
		sk.Content, sk.Code, tags, sk.Source, sk.SourceURL,
		sk.FormatOrigin, sk.Status, sk.UsageCount, sk.Enabled,
		tenantFromCtx(ctx),
	)
	return execExpectOne(tag, err, "update skill %s", sk.ID)
}

// DeleteSkill removes a skill by ID.
func (s *Store) DeleteSkill(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM skills WHERE id = $1 AND tenant_id = $2`,
		id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete skill %s", id)
}

// IncrementSkillUsage atomically increments the usage_count for a skill.
func (s *Store) IncrementSkillUsage(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE skills SET usage_count = usage_count + 1 WHERE id = $1 AND tenant_id = $2`,
		id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "increment skill usage %s", id)
}

// ListActiveSkills returns all active skills for a project (including global).
//
//nolint:staticcheck // Code and Enabled are deprecated but must be scanned for backward compat.
func (s *Store) ListActiveSkills(ctx context.Context, projectID string) ([]skill.Skill, error) {
	const q = `
		SELECT id, tenant_id, project_id, name, type, description, language,
		       content, code, tags, source, source_url, format_origin,
		       status, usage_count, enabled, created_at
		FROM skills
		WHERE (project_id = $1 OR project_id = '') AND tenant_id = $2 AND status = 'active'
		ORDER BY created_at ASC
		LIMIT $3`

	rows, err := s.pool.Query(ctx, q, projectID, tenantFromCtx(ctx), DefaultListLimit)
	if err != nil {
		return nil, fmt.Errorf("list active skills: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (skill.Skill, error) {
		var sk skill.Skill
		err := r.Scan(
			&sk.ID, &sk.TenantID, &sk.ProjectID, &sk.Name, &sk.Type, &sk.Description,
			&sk.Language, &sk.Content, &sk.Code, &sk.Tags, &sk.Source, &sk.SourceURL,
			&sk.FormatOrigin, &sk.Status, &sk.UsageCount, &sk.Enabled, &sk.CreatedAt,
		)
		return sk, err
	})
}
