package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/goal"
)

// CreateProjectGoal inserts a new project goal.
func (s *Store) CreateProjectGoal(ctx context.Context, g *goal.ProjectGoal) error {
	const q = `
		INSERT INTO project_goals (tenant_id, project_id, kind, title, content, source, source_path, priority, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`

	return s.pool.QueryRow(ctx, q,
		tenantFromCtx(ctx), g.ProjectID, string(g.Kind), g.Title, g.Content,
		g.Source, g.SourcePath, g.Priority, g.Enabled,
	).Scan(&g.ID, &g.CreatedAt, &g.UpdatedAt)
}

// GetProjectGoal retrieves a project goal by ID.
func (s *Store) GetProjectGoal(ctx context.Context, id string) (*goal.ProjectGoal, error) {
	const q = `
		SELECT id, tenant_id, project_id, kind, title, content, source, source_path, priority, enabled, created_at, updated_at
		FROM project_goals
		WHERE id = $1 AND tenant_id = $2`

	var g goal.ProjectGoal
	err := s.pool.QueryRow(ctx, q, id, tenantFromCtx(ctx)).Scan(
		&g.ID, &g.TenantID, &g.ProjectID, &g.Kind, &g.Title, &g.Content,
		&g.Source, &g.SourcePath, &g.Priority, &g.Enabled,
		&g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return nil, notFoundWrap(err, "get project goal %s", id)
	}
	return &g, nil
}

// ListProjectGoals returns all goals for a project.
func (s *Store) ListProjectGoals(ctx context.Context, projectID string) ([]goal.ProjectGoal, error) {
	const q = `
		SELECT id, tenant_id, project_id, kind, title, content, source, source_path, priority, enabled, created_at, updated_at
		FROM project_goals
		WHERE project_id = $1 AND tenant_id = $2
		ORDER BY priority DESC, created_at ASC`

	return s.scanGoals(ctx, q, projectID, tenantFromCtx(ctx))
}

// ListEnabledGoals returns enabled goals for a project, ordered by priority descending.
func (s *Store) ListEnabledGoals(ctx context.Context, projectID string) ([]goal.ProjectGoal, error) {
	const q = `
		SELECT id, tenant_id, project_id, kind, title, content, source, source_path, priority, enabled, created_at, updated_at
		FROM project_goals
		WHERE project_id = $1 AND tenant_id = $2 AND enabled = TRUE
		ORDER BY priority DESC, created_at ASC`

	return s.scanGoals(ctx, q, projectID, tenantFromCtx(ctx))
}

// UpdateProjectGoal updates a project goal.
func (s *Store) UpdateProjectGoal(ctx context.Context, g *goal.ProjectGoal) error {
	const q = `
		UPDATE project_goals
		SET kind = $2, title = $3, content = $4, source = $5, source_path = $6,
		    priority = $7, enabled = $8, updated_at = now()
		WHERE id = $1 AND tenant_id = $9`

	tag, err := s.pool.Exec(ctx, q,
		g.ID, string(g.Kind), g.Title, g.Content, g.Source, g.SourcePath,
		g.Priority, g.Enabled, tenantFromCtx(ctx),
	)
	return execExpectOne(tag, err, "update project goal %s", g.ID)
}

// DeleteProjectGoal removes a project goal by ID.
func (s *Store) DeleteProjectGoal(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM project_goals WHERE id = $1 AND tenant_id = $2`,
		id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete project goal %s", id)
}

// DeleteProjectGoalsBySource removes all goals for a project with a given source.
// Used for idempotent re-import.
func (s *Store) DeleteProjectGoalsBySource(ctx context.Context, projectID, source string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM project_goals WHERE project_id = $1 AND tenant_id = $2 AND source = $3`,
		projectID, tenantFromCtx(ctx), source)
	if err != nil {
		return fmt.Errorf("delete project goals by source: %w", err)
	}
	return nil
}

// scanGoals is a helper that scans goal rows from a query.
func (s *Store) scanGoals(ctx context.Context, query string, args ...any) ([]goal.ProjectGoal, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list project goals: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (goal.ProjectGoal, error) {
		var g goal.ProjectGoal
		err := r.Scan(
			&g.ID, &g.TenantID, &g.ProjectID, &g.Kind, &g.Title, &g.Content,
			&g.Source, &g.SourcePath, &g.Priority, &g.Enabled,
			&g.CreatedAt, &g.UpdatedAt,
		)
		return g, err
	})
}
