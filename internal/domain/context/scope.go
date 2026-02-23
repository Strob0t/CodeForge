package context

import (
	"errors"
	"time"
)

// ScopeType classifies a retrieval scope.
type ScopeType string

const (
	// ScopeShared is a named scope grouping multiple projects for cross-project search.
	ScopeShared ScopeType = "shared"
	// ScopeGlobal is a shared scope available to all projects.
	ScopeGlobal ScopeType = "global"
)

// ValidScopeType reports whether t is a known scope type.
func ValidScopeType(t ScopeType) bool {
	switch t {
	case ScopeShared, ScopeGlobal:
		return true
	}
	return false
}

// RetrievalScope defines a boundary for cross-project retrieval and graph search.
// Project scopes are implicit (every project has one); only shared and global scopes
// are stored in the database.
type RetrievalScope struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        ScopeType `json:"type"`
	Description string    `json:"description"`
	ProjectIDs  []string  `json:"project_ids"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Validate checks that a RetrievalScope is well-formed.
func (s *RetrievalScope) Validate() error {
	if s.Name == "" {
		return errors.New("name is required")
	}
	if !ValidScopeType(s.Type) {
		return errors.New("invalid scope type: " + string(s.Type))
	}
	if s.Type == ScopeShared && len(s.ProjectIDs) == 0 {
		return errors.New("shared scope requires at least one project")
	}
	return nil
}

// CreateScopeRequest holds the input for creating a scope.
type CreateScopeRequest struct {
	Name        string    `json:"name"`
	Type        ScopeType `json:"type"`
	ProjectIDs  []string  `json:"project_ids"`
	Description string    `json:"description"`
}

// Validate checks that a CreateScopeRequest is well-formed.
func (r *CreateScopeRequest) Validate() error {
	if r.Name == "" {
		return errors.New("name is required")
	}
	if !ValidScopeType(r.Type) {
		return errors.New("invalid scope type: " + string(r.Type))
	}
	if r.Type == ScopeShared && len(r.ProjectIDs) == 0 {
		return errors.New("shared scope requires at least one project")
	}
	return nil
}

// UpdateScopeRequest holds the input for updating a scope.
type UpdateScopeRequest struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	ProjectIDs  []string `json:"project_ids,omitempty"` // full replace when non-nil
}
