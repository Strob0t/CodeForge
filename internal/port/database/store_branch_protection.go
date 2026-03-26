package database

import (
	"context"

	bp "github.com/Strob0t/CodeForge/internal/domain/branchprotection"
)

// BranchProtectionStore defines database operations for branch protection rules.
type BranchProtectionStore interface {
	CreateBranchProtectionRule(ctx context.Context, req bp.CreateRuleRequest) (*bp.ProtectionRule, error)
	GetBranchProtectionRule(ctx context.Context, id string) (*bp.ProtectionRule, error)
	ListBranchProtectionRules(ctx context.Context, projectID string) ([]bp.ProtectionRule, error)
	UpdateBranchProtectionRule(ctx context.Context, rule *bp.ProtectionRule) error
	DeleteBranchProtectionRule(ctx context.Context, id string) error
}
