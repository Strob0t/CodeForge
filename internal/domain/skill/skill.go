// Package skill provides the domain model for reusable workflows or code patterns
// that are automatically injected into agent prompts via BM25 recommendation.
package skill

import (
	"errors"
	"time"
)

// Status constants for skill lifecycle.
const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// Source constants for skill origin.
const (
	SourceBuiltin = "builtin"
	SourceImport  = "import"
	SourceUser    = "user"
	SourceAgent   = "agent"
)

// Type constants for skill semantics.
const (
	TypeWorkflow = "workflow"
	TypePattern  = "pattern"
)

var validTypes = map[string]bool{TypeWorkflow: true, TypePattern: true}

// Skill represents a reusable workflow or code pattern with metadata for BM25 matching.
type Skill struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	ProjectID    string    `json:"project_id,omitempty"` // empty = global
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	Description  string    `json:"description"`
	Language     string    `json:"language"`
	Content      string    `json:"content"`
	Tags         []string  `json:"tags"`
	Source       string    `json:"source"`
	SourceURL    string    `json:"source_url,omitempty"`
	FormatOrigin string    `json:"format_origin"`
	Status       string    `json:"status"`
	UsageCount   int       `json:"usage_count"`
	CreatedAt    time.Time `json:"created_at"`

	// Deprecated: use Content instead. Kept for backwards compat with existing DB rows.
	Code string `json:"code,omitempty"`
	// Deprecated: use Status instead.
	Enabled bool `json:"enabled"`
}

// CreateRequest is the input for creating a new skill.
type CreateRequest struct {
	ProjectID    string   `json:"project_id,omitempty"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Description  string   `json:"description"`
	Language     string   `json:"language"`
	Content      string   `json:"content"`
	Tags         []string `json:"tags"`
	Source       string   `json:"source,omitempty"`
	SourceURL    string   `json:"source_url,omitempty"`
	FormatOrigin string   `json:"format_origin,omitempty"`
}

// UpdateRequest is the input for updating a skill.
type UpdateRequest struct {
	Name        string   `json:"name,omitempty"`
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description,omitempty"`
	Language    string   `json:"language,omitempty"`
	Content     string   `json:"content,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Status      *string  `json:"status,omitempty"`
}

// Validate checks that a CreateRequest has all required fields.
func (r *CreateRequest) Validate() error {
	if r.Name == "" {
		return errors.New("name is required")
	}
	if r.Content == "" {
		return errors.New("content is required")
	}
	if r.Description == "" {
		return errors.New("description is required")
	}
	if r.Type != "" && !validTypes[r.Type] {
		return errors.New("type must be 'workflow' or 'pattern'")
	}
	return nil
}
