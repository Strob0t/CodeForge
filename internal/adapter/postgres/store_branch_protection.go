package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	bp "github.com/Strob0t/CodeForge/internal/domain/branchprotection"
)

// --- Branch Protection Rules ---

func (s *Store) CreateBranchProtectionRule(ctx context.Context, req bp.CreateRuleRequest) (*bp.ProtectionRule, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO branch_protection_rules
		 (tenant_id, project_id, branch_pattern, require_reviews, require_tests, require_lint, allow_force_push, allow_delete, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, project_id, tenant_id, branch_pattern, require_reviews, require_tests, require_lint,
		           allow_force_push, allow_delete, enabled, version, created_at, updated_at`,
		tenantFromCtx(ctx), req.ProjectID, req.BranchPattern, req.RequireReviews, req.RequireTests, req.RequireLint,
		req.AllowForcePush, req.AllowDelete, req.Enabled)

	r, err := scanBranchProtectionRule(row)
	if err != nil {
		return nil, fmt.Errorf("create branch protection rule: %w", err)
	}
	return &r, nil
}

func (s *Store) GetBranchProtectionRule(ctx context.Context, id string) (*bp.ProtectionRule, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, tenant_id, branch_pattern, require_reviews, require_tests, require_lint,
		        allow_force_push, allow_delete, enabled, version, created_at, updated_at
		 FROM branch_protection_rules WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	r, err := scanBranchProtectionRule(row)
	if err != nil {
		return nil, notFoundWrap(err, "get branch protection rule %s", id)
	}
	return &r, nil
}

func (s *Store) ListBranchProtectionRules(ctx context.Context, projectID string) ([]bp.ProtectionRule, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, tenant_id, branch_pattern, require_reviews, require_tests, require_lint,
		        allow_force_push, allow_delete, enabled, version, created_at, updated_at
		 FROM branch_protection_rules WHERE project_id = $1 AND tenant_id = $2
		 ORDER BY created_at`, projectID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list branch protection rules: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (bp.ProtectionRule, error) {
		return scanBranchProtectionRule(r)
	})
}

func (s *Store) UpdateBranchProtectionRule(ctx context.Context, rule *bp.ProtectionRule) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE branch_protection_rules
		 SET branch_pattern = $1, require_reviews = $2, require_tests = $3, require_lint = $4,
		     allow_force_push = $5, allow_delete = $6, enabled = $7
		 WHERE id = $8 AND version = $9 AND tenant_id = $10`,
		rule.BranchPattern, rule.RequireReviews, rule.RequireTests, rule.RequireLint,
		rule.AllowForcePush, rule.AllowDelete, rule.Enabled,
		rule.ID, rule.Version, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("update branch protection rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (s *Store) DeleteBranchProtectionRule(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM branch_protection_rules WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete branch protection rule %s", id)
}

// scanBranchProtectionRule scans a single row into a ProtectionRule.
func scanBranchProtectionRule(row pgx.Row) (bp.ProtectionRule, error) {
	var r bp.ProtectionRule
	err := row.Scan(
		&r.ID, &r.ProjectID, &r.TenantID, &r.BranchPattern,
		&r.RequireReviews, &r.RequireTests, &r.RequireLint,
		&r.AllowForcePush, &r.AllowDelete, &r.Enabled,
		&r.Version, &r.CreatedAt, &r.UpdatedAt,
	)
	return r, err
}
