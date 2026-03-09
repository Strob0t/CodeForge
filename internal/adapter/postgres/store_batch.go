package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// BatchDeleteProjects deletes multiple projects by ID in a single query.
// Returns the list of IDs that were actually deleted.
func (s *Store) BatchDeleteProjects(ctx context.Context, ids []string) ([]string, error) {
	tid := tenantFromCtx(ctx)
	rows, err := s.pool.Query(ctx,
		`DELETE FROM projects WHERE id = ANY($1) AND tenant_id = $2 RETURNING id`,
		ids, tid)
	if err != nil {
		return nil, fmt.Errorf("batch delete projects: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (string, error) {
		var id string
		err := r.Scan(&id)
		return id, err
	})
}

// BatchGetProjects retrieves multiple projects by ID in a single query.
func (s *Store) BatchGetProjects(ctx context.Context, ids []string) ([]project.Project, error) {
	tid := tenantFromCtx(ctx)
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, description, repo_url, provider, workspace_path, config, policy_profile, version, created_at, updated_at
		 FROM projects WHERE id = ANY($1) AND tenant_id = $2`, ids, tid)
	if err != nil {
		return nil, fmt.Errorf("batch get projects: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (project.Project, error) {
		return scanProject(r)
	})
}
