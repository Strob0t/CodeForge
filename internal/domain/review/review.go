// Package review defines domain types for periodic review policies and reviews.
package review

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

// TriggerType identifies when a review should be automatically triggered.
type TriggerType string

const (
	TriggerCommitCount TriggerType = "commit_count"
	TriggerPreMerge    TriggerType = "pre_merge"
	TriggerCron        TriggerType = "cron"
)

// Status represents the lifecycle state of a review.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// ReviewPolicy defines an automated review trigger for a project.
type ReviewPolicy struct {
	ID              string      `json:"id"`
	ProjectID       string      `json:"project_id"`
	TenantID        string      `json:"tenant_id"`
	Name            string      `json:"name"`
	TriggerType     TriggerType `json:"trigger_type"`
	CommitThreshold int         `json:"commit_threshold,omitempty"`
	CronExpr        string      `json:"cron_expr,omitempty"`
	BranchPattern   string      `json:"branch_pattern,omitempty"`
	TemplateID      string      `json:"template_id"`
	Enabled         bool        `json:"enabled"`
	CommitCounter   int         `json:"commit_counter"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// Review represents a single triggered review instance linked to an execution plan.
type Review struct {
	ID          string     `json:"id"`
	PolicyID    string     `json:"policy_id"`
	ProjectID   string     `json:"project_id"`
	TenantID    string     `json:"tenant_id"`
	PlanID      string     `json:"plan_id"`
	Status      Status     `json:"status"`
	TriggerRef  string     `json:"trigger_ref"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// CreatePolicyRequest holds the fields for creating a new review policy.
type CreatePolicyRequest struct {
	Name            string      `json:"name"`
	TriggerType     TriggerType `json:"trigger_type"`
	CommitThreshold int         `json:"commit_threshold,omitempty"`
	CronExpr        string      `json:"cron_expr,omitempty"`
	BranchPattern   string      `json:"branch_pattern,omitempty"`
	TemplateID      string      `json:"template_id,omitempty"`
	Enabled         *bool       `json:"enabled,omitempty"`
}

// UpdatePolicyRequest holds the fields for updating an existing review policy.
type UpdatePolicyRequest struct {
	Name            *string      `json:"name,omitempty"`
	TriggerType     *TriggerType `json:"trigger_type,omitempty"`
	CommitThreshold *int         `json:"commit_threshold,omitempty"`
	CronExpr        *string      `json:"cron_expr,omitempty"`
	BranchPattern   *string      `json:"branch_pattern,omitempty"`
	TemplateID      *string      `json:"template_id,omitempty"`
	Enabled         *bool        `json:"enabled,omitempty"`
}

var (
	ErrNameRequired            = errors.New("policy name is required")
	ErrInvalidTriggerType      = errors.New("invalid trigger type")
	ErrCommitThresholdRequired = errors.New("commit_threshold must be >= 1 for commit_count trigger")
	ErrBranchPatternRequired   = errors.New("branch_pattern is required for pre_merge trigger")
	ErrCronExprRequired        = errors.New("cron_expr is required for cron trigger")
)

// Validate checks the create request for correctness.
func (r *CreatePolicyRequest) Validate() error {
	if r.Name == "" {
		return ErrNameRequired
	}
	switch r.TriggerType {
	case TriggerCommitCount:
		if r.CommitThreshold < 1 {
			return ErrCommitThresholdRequired
		}
	case TriggerPreMerge:
		if r.BranchPattern == "" {
			return ErrBranchPatternRequired
		}
	case TriggerCron:
		if r.CronExpr == "" {
			return ErrCronExprRequired
		}
		if err := ValidateCronExpr(r.CronExpr); err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
	default:
		return ErrInvalidTriggerType
	}
	return nil
}

// MatchesBranch returns true if the given branch name matches the policy's branch pattern.
func (p *ReviewPolicy) MatchesBranch(branch string) bool {
	if p.BranchPattern == "" {
		return false
	}
	matched, err := filepath.Match(p.BranchPattern, branch)
	if err != nil {
		return false
	}
	return matched
}
