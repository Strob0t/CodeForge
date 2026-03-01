package postgres

import (
	"context"
	"fmt"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
)

// --- Retrieval Scope CRUD ---

func (s *Store) CreateScope(ctx context.Context, req cfcontext.CreateScopeRequest) (*cfcontext.RetrievalScope, error) {
	tid := tenantFromCtx(ctx)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var sc cfcontext.RetrievalScope
	err = tx.QueryRow(ctx,
		`INSERT INTO retrieval_scopes (tenant_id, name, type, description)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, name, type, description, created_at, updated_at`,
		tid, req.Name, string(req.Type), req.Description,
	).Scan(&sc.ID, &sc.Name, &sc.Type, &sc.Description, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert scope: %w", err)
	}

	for _, pid := range req.ProjectIDs {
		_, err = tx.Exec(ctx,
			`INSERT INTO retrieval_scope_projects (scope_id, project_id) VALUES ($1, $2)`,
			sc.ID, pid,
		)
		if err != nil {
			return nil, fmt.Errorf("insert scope project %s: %w", pid, err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	sc.ProjectIDs = req.ProjectIDs
	return &sc, nil
}

func (s *Store) GetScope(ctx context.Context, id string) (*cfcontext.RetrievalScope, error) {
	tid := tenantFromCtx(ctx)

	var sc cfcontext.RetrievalScope
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, type, description, created_at, updated_at
		 FROM retrieval_scopes WHERE id = $1 AND tenant_id = $2`, id, tid,
	).Scan(&sc.ID, &sc.Name, &sc.Type, &sc.Description, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get scope %s", id)
	}

	pids, err := s.scopeProjectIDs(ctx, sc.ID)
	if err != nil {
		return nil, err
	}
	sc.ProjectIDs = pids
	return &sc, nil
}

func (s *Store) ListScopes(ctx context.Context) ([]cfcontext.RetrievalScope, error) {
	tid := tenantFromCtx(ctx)

	rows, err := s.pool.Query(ctx,
		`SELECT id, name, type, description, created_at, updated_at
		 FROM retrieval_scopes WHERE tenant_id = $1 ORDER BY created_at ASC`, tid)
	if err != nil {
		return nil, fmt.Errorf("list scopes: %w", err)
	}
	defer rows.Close()

	var scopes []cfcontext.RetrievalScope
	for rows.Next() {
		var sc cfcontext.RetrievalScope
		if err := rows.Scan(&sc.ID, &sc.Name, &sc.Type, &sc.Description, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan scope: %w", err)
		}
		pids, err := s.scopeProjectIDs(ctx, sc.ID)
		if err != nil {
			return nil, err
		}
		sc.ProjectIDs = pids
		scopes = append(scopes, sc)
	}
	return scopes, rows.Err()
}

func (s *Store) UpdateScope(ctx context.Context, id string, req cfcontext.UpdateScopeRequest) (*cfcontext.RetrievalScope, error) {
	tid := tenantFromCtx(ctx)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Apply partial updates.
	if req.Name != nil {
		tag, err := tx.Exec(ctx,
			`UPDATE retrieval_scopes SET name = $1, updated_at = now() WHERE id = $2 AND tenant_id = $3`,
			*req.Name, id, tid)
		if err := execExpectOne(tag, err, "update scope %s", id); err != nil {
			return nil, err
		}
	}
	if req.Description != nil {
		_, err := tx.Exec(ctx,
			`UPDATE retrieval_scopes SET description = $1, updated_at = now() WHERE id = $2 AND tenant_id = $3`,
			*req.Description, id, tid)
		if err != nil {
			return nil, fmt.Errorf("update scope description: %w", err)
		}
	}

	// Replace project IDs if provided.
	if req.ProjectIDs != nil {
		_, err := tx.Exec(ctx, `DELETE FROM retrieval_scope_projects WHERE scope_id = $1`, id)
		if err != nil {
			return nil, fmt.Errorf("clear scope projects: %w", err)
		}
		for _, pid := range req.ProjectIDs {
			_, err = tx.Exec(ctx,
				`INSERT INTO retrieval_scope_projects (scope_id, project_id) VALUES ($1, $2)`,
				id, pid)
			if err != nil {
				return nil, fmt.Errorf("re-insert scope project %s: %w", pid, err)
			}
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return s.GetScope(ctx, id)
}

func (s *Store) DeleteScope(ctx context.Context, id string) error {
	tid := tenantFromCtx(ctx)
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM retrieval_scopes WHERE id = $1 AND tenant_id = $2`, id, tid)
	return execExpectOne(tag, err, "delete scope %s", id)
}

func (s *Store) ListScopesByProject(ctx context.Context, projectID string) ([]cfcontext.RetrievalScope, error) {
	tid := tenantFromCtx(ctx)

	rows, err := s.pool.Query(ctx,
		`SELECT rs.id, rs.name, rs.type, rs.description, rs.created_at, rs.updated_at
		 FROM retrieval_scopes rs
		 JOIN retrieval_scope_projects rsp ON rs.id = rsp.scope_id
		 WHERE rsp.project_id = $1 AND rs.tenant_id = $2
		 ORDER BY rs.created_at ASC`, projectID, tid)
	if err != nil {
		return nil, fmt.Errorf("list scopes by project: %w", err)
	}
	defer rows.Close()

	var scopes []cfcontext.RetrievalScope
	for rows.Next() {
		var sc cfcontext.RetrievalScope
		if err := rows.Scan(&sc.ID, &sc.Name, &sc.Type, &sc.Description, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan scope: %w", err)
		}
		pids, err := s.scopeProjectIDs(ctx, sc.ID)
		if err != nil {
			return nil, err
		}
		sc.ProjectIDs = pids
		scopes = append(scopes, sc)
	}
	return scopes, rows.Err()
}

func (s *Store) AddProjectToScope(ctx context.Context, scopeID, projectID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO retrieval_scope_projects (scope_id, project_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`,
		scopeID, projectID)
	if err != nil {
		return fmt.Errorf("add project to scope: %w", err)
	}
	return nil
}

func (s *Store) RemoveProjectFromScope(ctx context.Context, scopeID, projectID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM retrieval_scope_projects WHERE scope_id = $1 AND project_id = $2`,
		scopeID, projectID)
	return execExpectOne(tag, err, "project %s not in scope %s", projectID, scopeID)
}

// scopeProjectIDs returns the project IDs belonging to a scope.
func (s *Store) scopeProjectIDs(ctx context.Context, scopeID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT project_id FROM retrieval_scope_projects WHERE scope_id = $1 ORDER BY added_at ASC`,
		scopeID)
	if err != nil {
		return nil, fmt.Errorf("scope project ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan project id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
