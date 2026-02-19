package service

import (
	"context"
	"fmt"

	bp "github.com/Strob0t/CodeForge/internal/domain/branchprotection"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// BranchProtectionService manages branch protection rules for projects.
type BranchProtectionService struct {
	store database.Store
}

// NewBranchProtectionService creates a new service.
func NewBranchProtectionService(store database.Store) *BranchProtectionService {
	return &BranchProtectionService{store: store}
}

// CreateRule creates a new branch protection rule.
func (s *BranchProtectionService) CreateRule(ctx context.Context, req bp.CreateRuleRequest) (*bp.ProtectionRule, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}
	return s.store.CreateBranchProtectionRule(ctx, req)
}

// GetRule returns a branch protection rule by ID.
func (s *BranchProtectionService) GetRule(ctx context.Context, id string) (*bp.ProtectionRule, error) {
	return s.store.GetBranchProtectionRule(ctx, id)
}

// ListRules returns all branch protection rules for a project.
func (s *BranchProtectionService) ListRules(ctx context.Context, projectID string) ([]bp.ProtectionRule, error) {
	return s.store.ListBranchProtectionRules(ctx, projectID)
}

// UpdateRule applies an update to an existing rule.
func (s *BranchProtectionService) UpdateRule(ctx context.Context, id string, req bp.UpdateRuleRequest) (*bp.ProtectionRule, error) {
	rule, err := s.store.GetBranchProtectionRule(ctx, id)
	if err != nil {
		return nil, err
	}
	rule.Apply(req)
	if err := s.store.UpdateBranchProtectionRule(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

// DeleteRule removes a branch protection rule.
func (s *BranchProtectionService) DeleteRule(ctx context.Context, id string) error {
	return s.store.DeleteBranchProtectionRule(ctx, id)
}

// CheckBranch evaluates all enabled rules for a project against a push action.
func (s *BranchProtectionService) CheckBranch(ctx context.Context, projectID string, action bp.PushAction) (*bp.EvalResult, error) {
	rules, err := s.store.ListBranchProtectionRules(ctx, projectID)
	if err != nil {
		return nil, err
	}
	result := bp.EvaluatePush(rules, action)
	return &result, nil
}

// CheckMerge evaluates all enabled rules for a project against a merge action.
func (s *BranchProtectionService) CheckMerge(ctx context.Context, projectID string, action bp.MergeAction) (*bp.EvalResult, error) {
	rules, err := s.store.ListBranchProtectionRules(ctx, projectID)
	if err != nil {
		return nil, err
	}
	result := bp.EvaluateMerge(rules, action)
	return &result, nil
}
