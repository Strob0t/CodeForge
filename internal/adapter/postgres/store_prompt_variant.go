package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
)

// InsertVariant inserts a new prompt variant into the prompt_sections table
// with evolution tracking columns populated.
func (s *Store) InsertVariant(ctx context.Context, v *prompt.PromptVariant) error {
	tid := tenantFromCtx(ctx)

	_, err := s.pool.Exec(ctx,
		`INSERT INTO prompt_sections
		   (tenant_id, name, scope, content, priority, enabled, version,
		    parent_id, mutation_source, promotion_status, trial_count, avg_score,
		    mode_id, model_family)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		tid,
		v.Name,
		v.Scope,
		v.Content,
		v.Priority,
		v.Enabled,
		v.Version,
		nullIfEmpty(v.ParentID),
		nullIfEmpty(v.MutationSource),
		string(v.PromotionStatus),
		v.TrialCount,
		v.AvgScore,
		v.ModeID,
		v.ModelFamily,
	)
	if err != nil {
		return fmt.Errorf("insert prompt variant: %w", err)
	}
	return nil
}

// GetVariantByID returns a single prompt variant by ID.
func (s *Store) GetVariantByID(ctx context.Context, id string) (prompt.PromptVariant, error) {
	tid := tenantFromCtx(ctx)

	row := s.pool.QueryRow(ctx,
		`SELECT id, name, scope, content, priority, enabled, version,
		        COALESCE(parent_id, ''), COALESCE(mutation_source, ''),
		        promotion_status, trial_count, avg_score,
		        mode_id, model_family, created_at, updated_at
		 FROM prompt_sections
		 WHERE id = $1 AND tenant_id = $2`, id, tid)

	var v prompt.PromptVariant
	v.TenantID = tid
	err := row.Scan(
		&v.ID, &v.Name, &v.Scope, &v.Content, &v.Priority, &v.Enabled, &v.Version,
		&v.ParentID, &v.MutationSource,
		&v.PromotionStatus, &v.TrialCount, &v.AvgScore,
		&v.ModeID, &v.ModelFamily, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		return prompt.PromptVariant{}, notFoundWrap(err, "get prompt variant %s", id)
	}
	return v, nil
}

// GetVariantsByModeAndModel returns all prompt variants for a given mode and model family.
func (s *Store) GetVariantsByModeAndModel(ctx context.Context, modeID, modelFamily string) ([]prompt.PromptVariant, error) {
	tid := tenantFromCtx(ctx)

	rows, err := s.pool.Query(ctx,
		`SELECT id, name, scope, content, priority, enabled, version,
		        COALESCE(parent_id, ''), COALESCE(mutation_source, ''),
		        promotion_status, trial_count, avg_score,
		        mode_id, model_family, created_at, updated_at
		 FROM prompt_sections
		 WHERE tenant_id = $1 AND mode_id = $2 AND model_family = $3
		 ORDER BY version DESC`, tid, modeID, modelFamily)
	if err != nil {
		return nil, fmt.Errorf("get variants by mode and model: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (prompt.PromptVariant, error) {
		var v prompt.PromptVariant
		v.TenantID = tid
		err := r.Scan(
			&v.ID, &v.Name, &v.Scope, &v.Content, &v.Priority, &v.Enabled, &v.Version,
			&v.ParentID, &v.MutationSource,
			&v.PromotionStatus, &v.TrialCount, &v.AvgScore,
			&v.ModeID, &v.ModelFamily, &v.CreatedAt, &v.UpdatedAt,
		)
		return v, err
	})
}

// ListVariants returns all prompt variants for a tenant, optionally filtered
// by mode_id and promotion status.
func (s *Store) ListVariants(ctx context.Context, modeID, status string) ([]prompt.PromptVariant, error) {
	tid := tenantFromCtx(ctx)

	query := `SELECT id, name, scope, content, priority, enabled, version,
	                COALESCE(parent_id, ''), COALESCE(mutation_source, ''),
	                promotion_status, trial_count, avg_score,
	                mode_id, model_family, created_at, updated_at
	         FROM prompt_sections
	         WHERE tenant_id = $1 AND mode_id != ''`
	args := []any{tid}
	argIdx := 2

	if modeID != "" {
		query += fmt.Sprintf(" AND mode_id = $%d", argIdx)
		args = append(args, modeID)
		argIdx++
	}
	if status != "" {
		query += fmt.Sprintf(" AND promotion_status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	query += " ORDER BY mode_id, version DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list variants: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (prompt.PromptVariant, error) {
		var v prompt.PromptVariant
		v.TenantID = tid
		err := r.Scan(
			&v.ID, &v.Name, &v.Scope, &v.Content, &v.Priority, &v.Enabled, &v.Version,
			&v.ParentID, &v.MutationSource,
			&v.PromotionStatus, &v.TrialCount, &v.AvgScore,
			&v.ModeID, &v.ModelFamily, &v.CreatedAt, &v.UpdatedAt,
		)
		return v, err
	})
}

// UpdatePromotionStatus updates the promotion status of a prompt variant.
func (s *Store) UpdatePromotionStatus(ctx context.Context, id string, status prompt.PromotionStatus) error {
	tid := tenantFromCtx(ctx)

	tag, err := s.pool.Exec(ctx,
		`UPDATE prompt_sections
		 SET promotion_status = $1, updated_at = now()
		 WHERE id = $2 AND tenant_id = $3`,
		string(status), id, tid)
	return execExpectOne(tag, err, "update promotion status %s", id)
}

// UpdateVariantStats updates the trial count and average score for a prompt variant.
func (s *Store) UpdateVariantStats(ctx context.Context, id string, trialCount int, avgScore float64) error {
	tid := tenantFromCtx(ctx)

	tag, err := s.pool.Exec(ctx,
		`UPDATE prompt_sections
		 SET trial_count = $1, avg_score = $2, updated_at = now()
		 WHERE id = $3 AND tenant_id = $4`,
		trialCount, avgScore, id, tid)
	return execExpectOne(tag, err, "update variant stats %s", id)
}
