package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/boundary"
)

// GetProjectBoundaries retrieves the boundary configuration for a project.
func (s *Store) GetProjectBoundaries(ctx context.Context, projectID string) (*boundary.ProjectBoundaryConfig, error) {
	tid := tenantFromCtx(ctx)
	var cfg boundary.ProjectBoundaryConfig
	var boundariesJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT project_id, tenant_id, boundaries, last_analyzed, version
		 FROM project_boundaries
		 WHERE project_id = $1 AND tenant_id = $2`,
		projectID, tid,
	).Scan(&cfg.ProjectID, &cfg.TenantID, &boundariesJSON, &cfg.LastAnalyzed, &cfg.Version)
	if err != nil {
		return nil, notFoundWrap(err, "get project boundaries %s", projectID)
	}
	if err := json.Unmarshal(boundariesJSON, &cfg.Boundaries); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpsertProjectBoundaries inserts or updates the boundary configuration for a project.
func (s *Store) UpsertProjectBoundaries(ctx context.Context, cfg *boundary.ProjectBoundaryConfig) error {
	tid := tenantFromCtx(ctx)
	boundariesJSON, err := json.Marshal(cfg.Boundaries)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO project_boundaries (project_id, tenant_id, boundaries, last_analyzed, version)
		 VALUES ($1, $2, $3, $4, 1)
		 ON CONFLICT (project_id)
		 DO UPDATE SET boundaries = $3, last_analyzed = $4, version = project_boundaries.version + 1, updated_at = now()
		 WHERE project_boundaries.tenant_id = $2`,
		cfg.ProjectID, tid, boundariesJSON, time.Now(),
	)
	return err
}

// DeleteProjectBoundaries removes the boundary configuration for a project.
func (s *Store) DeleteProjectBoundaries(ctx context.Context, projectID string) error {
	tid := tenantFromCtx(ctx)
	_, err := s.pool.Exec(ctx,
		`DELETE FROM project_boundaries WHERE project_id = $1 AND tenant_id = $2`,
		projectID, tid,
	)
	return err
}
