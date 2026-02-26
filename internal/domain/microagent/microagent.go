// Package microagent provides the domain model for microagents —
// small YAML+Markdown-defined agents activated by trigger patterns.
package microagent

import (
	"errors"
	"slices"
	"time"
)

// Type categorizes the microagent's purpose.
type Type string

const (
	TypeKnowledge Type = "knowledge"
	TypeRepo      Type = "repo"
	TypeTask      Type = "task"
)

// ValidTypes lists all valid microagent types.
var ValidTypes = []Type{TypeKnowledge, TypeRepo, TypeTask}

// Microagent represents a lightweight, trigger-based agent.
type Microagent struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	ProjectID      string    `json:"project_id,omitempty"` // empty = global
	Name           string    `json:"name"`
	Type           Type      `json:"type"`
	TriggerPattern string    `json:"trigger_pattern"`
	Description    string    `json:"description"`
	Prompt         string    `json:"prompt"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CreateRequest is the input for creating a new microagent.
type CreateRequest struct {
	ProjectID      string `json:"project_id,omitempty"`
	Name           string `json:"name"`
	Type           Type   `json:"type"`
	TriggerPattern string `json:"trigger_pattern"`
	Description    string `json:"description"`
	Prompt         string `json:"prompt"`
}

// UpdateRequest is the input for updating a microagent.
type UpdateRequest struct {
	Name           string `json:"name,omitempty"`
	TriggerPattern string `json:"trigger_pattern,omitempty"`
	Description    string `json:"description,omitempty"`
	Prompt         string `json:"prompt,omitempty"`
	Enabled        *bool  `json:"enabled,omitempty"`
}

// Validate checks that a CreateRequest has all required fields.
func (r *CreateRequest) Validate() error {
	if r.Name == "" {
		return errors.New("name is required")
	}
	if !slices.Contains(ValidTypes, r.Type) {
		return errors.New("invalid type: must be knowledge, repo, or task")
	}
	if r.TriggerPattern == "" {
		return errors.New("trigger_pattern is required")
	}
	if r.Prompt == "" {
		return errors.New("prompt is required")
	}
	return nil
}

// Validate checks domain invariants.
func (m *Microagent) Validate() error {
	if m.Name == "" {
		return errors.New("name is required")
	}
	if !slices.Contains(ValidTypes, m.Type) {
		return errors.New("invalid type")
	}
	return nil
}
