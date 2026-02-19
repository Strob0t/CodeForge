// Package branchprotection defines branch protection rules for projects.
package branchprotection

import (
	"fmt"
	"time"
)

// ProtectionRule defines protection settings for branches matching a glob pattern.
type ProtectionRule struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	TenantID       string    `json:"tenant_id,omitempty"`
	BranchPattern  string    `json:"branch_pattern"`
	RequireReviews bool      `json:"require_reviews"`
	RequireTests   bool      `json:"require_tests"`
	RequireLint    bool      `json:"require_lint"`
	AllowForcePush bool      `json:"allow_force_push"`
	AllowDelete    bool      `json:"allow_delete"`
	Enabled        bool      `json:"enabled"`
	Version        int       `json:"version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CreateRuleRequest is the input for creating a new branch protection rule.
type CreateRuleRequest struct {
	ProjectID      string `json:"project_id"`
	BranchPattern  string `json:"branch_pattern"`
	RequireReviews bool   `json:"require_reviews"`
	RequireTests   bool   `json:"require_tests"`
	RequireLint    bool   `json:"require_lint"`
	AllowForcePush bool   `json:"allow_force_push"`
	AllowDelete    bool   `json:"allow_delete"`
	Enabled        bool   `json:"enabled"`
}

// UpdateRuleRequest is the input for updating a branch protection rule.
type UpdateRuleRequest struct {
	BranchPattern  *string `json:"branch_pattern,omitempty"`
	RequireReviews *bool   `json:"require_reviews,omitempty"`
	RequireTests   *bool   `json:"require_tests,omitempty"`
	RequireLint    *bool   `json:"require_lint,omitempty"`
	AllowForcePush *bool   `json:"allow_force_push,omitempty"`
	AllowDelete    *bool   `json:"allow_delete,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty"`
}

// Validate checks if the CreateRuleRequest is valid.
func (r *CreateRuleRequest) Validate() error {
	if r.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}
	if r.BranchPattern == "" {
		return fmt.Errorf("branch_pattern is required")
	}
	return nil
}

// Apply merges the non-nil fields from an UpdateRuleRequest into a ProtectionRule.
func (r *ProtectionRule) Apply(req UpdateRuleRequest) {
	if req.BranchPattern != nil {
		r.BranchPattern = *req.BranchPattern
	}
	if req.RequireReviews != nil {
		r.RequireReviews = *req.RequireReviews
	}
	if req.RequireTests != nil {
		r.RequireTests = *req.RequireTests
	}
	if req.RequireLint != nil {
		r.RequireLint = *req.RequireLint
	}
	if req.AllowForcePush != nil {
		r.AllowForcePush = *req.AllowForcePush
	}
	if req.AllowDelete != nil {
		r.AllowDelete = *req.AllowDelete
	}
	if req.Enabled != nil {
		r.Enabled = *req.Enabled
	}
}
