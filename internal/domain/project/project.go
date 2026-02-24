// Package project defines the Project domain entity.
package project

import "time"

// Project represents a code repository managed by CodeForge.
type Project struct {
	ID            string            `json:"id"`
	TenantID      string            `json:"tenant_id,omitempty"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	RepoURL       string            `json:"repo_url"`
	Provider      string            `json:"provider"`
	WorkspacePath string            `json:"workspace_path,omitempty"`
	Config        map[string]string `json:"config"`
	PolicyProfile string            `json:"policy_profile,omitempty"`
	Version       int               `json:"version"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// CreateRequest holds the fields needed to create a new project.
type CreateRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	RepoURL     string            `json:"repo_url"`
	Provider    string            `json:"provider"`
	Branch      string            `json:"branch,omitempty"`
	Config      map[string]string `json:"config"`
}

// UpdateRequest holds the fields for a partial project update.
// Nil pointer fields are left unchanged.
type UpdateRequest struct {
	Name        *string           `json:"name"`
	Description *string           `json:"description"`
	RepoURL     *string           `json:"repo_url"`
	Provider    *string           `json:"provider"`
	Config      map[string]string `json:"config"`
}

// AdoptRequest holds the fields for adopting an existing directory as a workspace.
type AdoptRequest struct {
	Path string `json:"path"`
}

// SetupResult holds the outcome of the automated project setup chain.
type SetupResult struct {
	Cloned        bool                  `json:"cloned"`
	StackDetected bool                  `json:"stack_detected"`
	Stack         *StackDetectionResult `json:"stack,omitempty"`
	SpecsDetected bool                  `json:"specs_detected"`
	Steps         []SetupStep           `json:"steps"`
}

// SetupStep records the outcome of a single step in the setup chain.
type SetupStep struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "completed", "skipped", "failed"
	Error  string `json:"error,omitempty"`
}

// WorkspaceInfo holds health and status information about a project's workspace.
type WorkspaceInfo struct {
	Exists         bool      `json:"exists"`
	Path           string    `json:"path"`
	DiskUsageBytes int64     `json:"disk_usage_bytes"`
	GitRepo        bool      `json:"git_repo"`
	LastModified   time.Time `json:"last_modified"`
}
