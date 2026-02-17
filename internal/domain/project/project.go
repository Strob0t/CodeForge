// Package project defines the Project domain entity.
package project

import "time"

// Project represents a code repository managed by CodeForge.
type Project struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	RepoURL       string            `json:"repo_url"`
	Provider      string            `json:"provider"`
	WorkspacePath string            `json:"workspace_path,omitempty"`
	Config        map[string]string `json:"config"`
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
	Config      map[string]string `json:"config"`
}
