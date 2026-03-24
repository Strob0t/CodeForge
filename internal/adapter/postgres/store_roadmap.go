package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
)

// --- Roadmaps ---

func (s *Store) CreateRoadmap(ctx context.Context, req roadmap.CreateRoadmapRequest) (*roadmap.Roadmap, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO roadmaps (tenant_id, project_id, title, description)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, project_id, tenant_id, title, description, status, version, created_at, updated_at`,
		tenantFromCtx(ctx), req.ProjectID, req.Title, req.Description)

	r, err := scanRoadmap(row)
	if err != nil {
		return nil, fmt.Errorf("create roadmap: %w", err)
	}
	return &r, nil
}

func (s *Store) GetRoadmap(ctx context.Context, id string) (*roadmap.Roadmap, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, tenant_id, title, description, status, version, created_at, updated_at
		 FROM roadmaps WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	r, err := scanRoadmap(row)
	if err != nil {
		return nil, notFoundWrap(err, "get roadmap %s", id)
	}
	return &r, nil
}

func (s *Store) GetRoadmapByProject(ctx context.Context, projectID string) (*roadmap.Roadmap, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, tenant_id, title, description, status, version, created_at, updated_at
		 FROM roadmaps WHERE project_id = $1 AND tenant_id = $2`, projectID, tenantFromCtx(ctx))

	r, err := scanRoadmap(row)
	if err != nil {
		return nil, notFoundWrap(err, "get roadmap for project %s", projectID)
	}
	return &r, nil
}

func (s *Store) UpdateRoadmap(ctx context.Context, r *roadmap.Roadmap) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE roadmaps SET title = $2, description = $3, status = $4
		 WHERE id = $1 AND version = $5 AND tenant_id = $6`,
		r.ID, r.Title, r.Description, string(r.Status), r.Version, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("update roadmap %s: %w", r.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update roadmap %s: %w", r.ID, domain.ErrConflict)
	}
	r.Version++
	return nil
}

func (s *Store) DeleteRoadmap(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM roadmaps WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete roadmap %s", id)
}

// --- Milestones ---

func (s *Store) CreateMilestone(ctx context.Context, req roadmap.CreateMilestoneRequest) (*roadmap.Milestone, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO milestones (tenant_id, roadmap_id, title, description, due_date)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, roadmap_id, title, description, status, sort_order, due_date, version, created_at, updated_at`,
		tenantFromCtx(ctx), req.RoadmapID, req.Title, req.Description, req.DueDate)

	m, err := scanMilestone(row)
	if err != nil {
		return nil, fmt.Errorf("create milestone: %w", err)
	}
	return &m, nil
}

func (s *Store) GetMilestone(ctx context.Context, id string) (*roadmap.Milestone, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, roadmap_id, title, description, status, sort_order, due_date, version, created_at, updated_at
		 FROM milestones WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	m, err := scanMilestone(row)
	if err != nil {
		return nil, notFoundWrap(err, "get milestone %s", id)
	}
	return &m, nil
}

func (s *Store) ListMilestones(ctx context.Context, roadmapID string) ([]roadmap.Milestone, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, roadmap_id, title, description, status, sort_order, due_date, version, created_at, updated_at
		 FROM milestones WHERE roadmap_id = $1 AND tenant_id = $2 ORDER BY sort_order ASC, created_at ASC`, roadmapID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list milestones: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (roadmap.Milestone, error) {
		return scanMilestone(r)
	})
}

func (s *Store) UpdateMilestone(ctx context.Context, m *roadmap.Milestone) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE milestones SET title = $2, description = $3, status = $4, sort_order = $5, due_date = $6
		 WHERE id = $1 AND version = $7 AND tenant_id = $8`,
		m.ID, m.Title, m.Description, string(m.Status), m.SortOrder, m.DueDate, m.Version, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("update milestone %s: %w", m.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update milestone %s: %w", m.ID, domain.ErrConflict)
	}
	m.Version++
	return nil
}

func (s *Store) DeleteMilestone(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM milestones WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete milestone %s", id)
}

// FindMilestoneByTitle returns the first milestone matching the given title within a roadmap.
// Returns domain.ErrNotFound if no match exists.
func (s *Store) FindMilestoneByTitle(ctx context.Context, roadmapID, title string) (*roadmap.Milestone, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, roadmap_id, title, description, status, sort_order, due_date, version, created_at, updated_at
		 FROM milestones WHERE roadmap_id = $1 AND title = $2 AND tenant_id = $3
		 LIMIT 1`, roadmapID, title, tenantFromCtx(ctx))

	m, err := scanMilestone(row)
	if err != nil {
		return nil, notFoundWrap(err, "find milestone by title %q in roadmap %s", title, roadmapID)
	}
	return &m, nil
}

// --- Features ---

func (s *Store) CreateFeature(ctx context.Context, req *roadmap.CreateFeatureRequest) (*roadmap.Feature, error) {
	externalIDsJSON, err := marshalJSON(req.ExternalIDs, "external_ids")
	if err != nil {
		return nil, err
	}

	tid := tenantFromCtx(ctx)

	// Resolve roadmap_id from milestone.
	var roadmapID string
	if err := s.pool.QueryRow(ctx,
		`SELECT roadmap_id FROM milestones WHERE id = $1 AND tenant_id = $2`, req.MilestoneID, tid,
	).Scan(&roadmapID); err != nil {
		return nil, notFoundWrap(err, "milestone %s", req.MilestoneID)
	}

	labels := orEmpty(req.Labels)

	row := s.pool.QueryRow(ctx,
		`INSERT INTO features (tenant_id, milestone_id, roadmap_id, title, description, labels, spec_ref, external_ids)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, milestone_id, roadmap_id, title, description, status, labels, spec_ref, external_ids, sort_order, version, created_at, updated_at`,
		tid, req.MilestoneID, roadmapID, req.Title, req.Description, labels, req.SpecRef, externalIDsJSON)

	f, err := scanFeature(row)
	if err != nil {
		return nil, fmt.Errorf("create feature: %w", err)
	}
	return &f, nil
}

func (s *Store) GetFeature(ctx context.Context, id string) (*roadmap.Feature, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, milestone_id, roadmap_id, title, description, status, labels, spec_ref, external_ids, sort_order, version, created_at, updated_at
		 FROM features WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	f, err := scanFeature(row)
	if err != nil {
		return nil, notFoundWrap(err, "get feature %s", id)
	}
	return &f, nil
}

// FindFeatureBySpecRef returns the first feature matching the given spec_ref within a milestone.
// Returns domain.ErrNotFound if no match exists.
func (s *Store) FindFeatureBySpecRef(ctx context.Context, milestoneID, specRef string) (*roadmap.Feature, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, milestone_id, roadmap_id, title, description, status, labels, spec_ref, external_ids, sort_order, version, created_at, updated_at
		 FROM features WHERE milestone_id = $1 AND spec_ref = $2 AND tenant_id = $3
		 LIMIT 1`, milestoneID, specRef, tenantFromCtx(ctx))

	f, err := scanFeature(row)
	if err != nil {
		return nil, notFoundWrap(err, "find feature by spec_ref %q in milestone %s", specRef, milestoneID)
	}
	return &f, nil
}

func (s *Store) ListFeatures(ctx context.Context, milestoneID string) ([]roadmap.Feature, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, milestone_id, roadmap_id, title, description, status, labels, spec_ref, external_ids, sort_order, version, created_at, updated_at
		 FROM features WHERE milestone_id = $1 AND tenant_id = $2 ORDER BY sort_order ASC, created_at ASC`, milestoneID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list features: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (roadmap.Feature, error) {
		return scanFeature(r)
	})
}

func (s *Store) ListFeaturesByRoadmap(ctx context.Context, roadmapID string) ([]roadmap.Feature, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, milestone_id, roadmap_id, title, description, status, labels, spec_ref, external_ids, sort_order, version, created_at, updated_at
		 FROM features WHERE roadmap_id = $1 AND tenant_id = $2 ORDER BY sort_order ASC, created_at ASC`, roadmapID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list features by roadmap: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (roadmap.Feature, error) {
		return scanFeature(r)
	})
}

func (s *Store) UpdateFeature(ctx context.Context, f *roadmap.Feature) error {
	externalIDsJSON, err := marshalJSON(f.ExternalIDs, "external_ids")
	if err != nil {
		return err
	}

	labels := orEmpty(f.Labels)

	tag, err := s.pool.Exec(ctx,
		`UPDATE features SET title = $2, description = $3, status = $4, labels = $5, spec_ref = $6, external_ids = $7, sort_order = $8, milestone_id = $9
		 WHERE id = $1 AND version = $10 AND tenant_id = $11`,
		f.ID, f.Title, f.Description, string(f.Status), labels, f.SpecRef, externalIDsJSON, f.SortOrder, f.MilestoneID, f.Version, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("update feature %s: %w", f.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update feature %s: %w", f.ID, domain.ErrConflict)
	}
	f.Version++
	return nil
}

func (s *Store) DeleteFeature(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM features WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete feature %s", id)
}

func scanRoadmap(row scannable) (roadmap.Roadmap, error) {
	var r roadmap.Roadmap
	err := row.Scan(&r.ID, &r.ProjectID, &r.TenantID, &r.Title, &r.Description, &r.Status, &r.Version, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func scanMilestone(row scannable) (roadmap.Milestone, error) {
	var m roadmap.Milestone
	err := row.Scan(&m.ID, &m.RoadmapID, &m.Title, &m.Description, &m.Status, &m.SortOrder, &m.DueDate, &m.Version, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

func scanFeature(row scannable) (roadmap.Feature, error) {
	var f roadmap.Feature
	var externalIDsJSON []byte
	err := row.Scan(&f.ID, &f.MilestoneID, &f.RoadmapID, &f.Title, &f.Description, &f.Status, &f.Labels, &f.SpecRef, &externalIDsJSON, &f.SortOrder, &f.Version, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return f, err
	}
	if err := unmarshalJSONField(externalIDsJSON, &f.ExternalIDs, "external_ids"); err != nil {
		return f, err
	}
	return f, nil
}
