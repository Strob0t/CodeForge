package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// --- Projects ---

func (s *Store) ListProjects(ctx context.Context) ([]project.Project, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, description, repo_url, provider, workspace_path, config, policy_profile, version, created_at, updated_at
		 FROM projects WHERE tenant_id = $1 ORDER BY created_at DESC
		 LIMIT $2`, tenantFromCtx(ctx), DefaultListLimit)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (project.Project, error) {
		return scanProject(r)
	})
}

func (s *Store) GetProject(ctx context.Context, id string) (*project.Project, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, description, repo_url, provider, workspace_path, config, policy_profile, version, created_at, updated_at
		 FROM projects WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	p, err := scanProject(row)
	if err != nil {
		return nil, notFoundWrap(err, "get project %s", id)
	}
	return &p, nil
}

func (s *Store) GetProjectByRepoName(ctx context.Context, repoName string) (*project.Project, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, description, repo_url, provider, workspace_path, config, policy_profile, version, created_at, updated_at
		 FROM projects WHERE repo_url LIKE '%' || $1 || '%' AND tenant_id = $2 LIMIT 1`, repoName, tenantFromCtx(ctx))

	p, err := scanProject(row)
	if err != nil {
		return nil, notFoundWrap(err, "get project by repo %s", repoName)
	}
	return &p, nil
}

func (s *Store) CreateProject(ctx context.Context, req *project.CreateRequest) (*project.Project, error) {
	configJSON, err := marshalJSON(req.Config, "config")
	if err != nil {
		return nil, err
	}

	row := s.pool.QueryRow(ctx,
		`INSERT INTO projects (tenant_id, name, description, repo_url, provider, config)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, name, description, repo_url, provider, workspace_path, config, policy_profile, version, created_at, updated_at`,
		tenantFromCtx(ctx), req.Name, req.Description, req.RepoURL, req.Provider, configJSON)

	p, err := scanProject(row)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	return &p, nil
}

func (s *Store) UpdateProject(ctx context.Context, p *project.Project) error {
	configJSON, err := marshalJSON(p.Config, "config")
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE projects SET name = $2, description = $3, repo_url = $4, provider = $5, workspace_path = $6, config = $7, policy_profile = $8
		 WHERE id = $1 AND version = $9 AND tenant_id = $10`,
		p.ID, p.Name, p.Description, p.RepoURL, p.Provider, p.WorkspacePath, configJSON, p.PolicyProfile, p.Version, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("update project %s: %w", p.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update project %s: %w", p.ID, domain.ErrConflict)
	}
	p.Version++
	return nil
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete project %s", id)
}

func scanProject(row scannable) (project.Project, error) {
	var p project.Project
	var configJSON []byte
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.RepoURL, &p.Provider, &p.WorkspacePath, &configJSON, &p.PolicyProfile, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	if err := unmarshalJSONField(configJSON, &p.Config, "config"); err != nil {
		return p, err
	}
	return p, nil
}
