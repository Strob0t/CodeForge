// Package skill provides the domain model for reusable code snippets
// that are automatically injected into agent prompts via BM25 recommendation.
package skill

import (
	"errors"
	"time"
)

// Skill represents a reusable code snippet with metadata for BM25 matching.
type Skill struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	ProjectID   string    `json:"project_id,omitempty"` // empty = global
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Language    string    `json:"language"`
	Code        string    `json:"code"`
	Tags        []string  `json:"tags"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateRequest is the input for creating a new skill.
type CreateRequest struct {
	ProjectID   string   `json:"project_id,omitempty"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Language    string   `json:"language"`
	Code        string   `json:"code"`
	Tags        []string `json:"tags"`
}

// UpdateRequest is the input for updating a skill.
type UpdateRequest struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Language    string   `json:"language,omitempty"`
	Code        string   `json:"code,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
}

// Validate checks that a CreateRequest has all required fields.
func (r *CreateRequest) Validate() error {
	if r.Name == "" {
		return errors.New("name is required")
	}
	if r.Code == "" {
		return errors.New("code is required")
	}
	if r.Description == "" {
		return errors.New("description is required")
	}
	return nil
}
