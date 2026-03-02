// Package goal provides the domain model for project goals —
// structured representations of a project's vision, requirements,
// constraints, state, and context discovered from repository files.
package goal

import (
	"errors"
	"slices"
	"time"
)

// GoalKind categorizes a project goal.
type GoalKind string

const (
	KindVision      GoalKind = "vision"      // Why this project exists
	KindRequirement GoalKind = "requirement" // What's in scope
	KindConstraint  GoalKind = "constraint"  // Decisions, rules, architecture
	KindState       GoalKind = "state"       // Current position, blockers
	KindContext     GoalKind = "context"     // Phase/implementation context
)

// ValidKinds lists all valid goal kinds.
var ValidKinds = []GoalKind{KindVision, KindRequirement, KindConstraint, KindState, KindContext}

// IsValid reports whether k is a known GoalKind.
func (k GoalKind) IsValid() bool {
	return slices.Contains(ValidKinds, k)
}

// ProjectGoal represents a single goal discovered from or manually added to a project.
type ProjectGoal struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	ProjectID  string    `json:"project_id"`
	Kind       GoalKind  `json:"kind"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`     // markdown
	Source     string    `json:"source"`      // "gsd", "readme", "claude_md", "manual", etc.
	SourcePath string    `json:"source_path"` // relative file path (empty for manual)
	Priority   int       `json:"priority"`    // 0-100
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Validate checks domain invariants for a ProjectGoal.
func (g *ProjectGoal) Validate() error {
	if g.ProjectID == "" {
		return errors.New("project_id is required")
	}
	if !g.Kind.IsValid() {
		return errors.New("invalid kind: " + string(g.Kind))
	}
	if g.Title == "" {
		return errors.New("title is required")
	}
	if g.Content == "" {
		return errors.New("content is required")
	}
	if g.Priority < 0 || g.Priority > 100 {
		return errors.New("priority must be 0-100")
	}
	return nil
}

// CreateRequest is the input for creating a new project goal.
type CreateRequest struct {
	Kind     GoalKind `json:"kind"`
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	Source   string   `json:"source,omitempty"`
	Priority int      `json:"priority,omitempty"`
}

// Validate checks that a CreateRequest has all required fields.
func (r *CreateRequest) Validate() error {
	if !r.Kind.IsValid() {
		return errors.New("invalid kind: " + string(r.Kind))
	}
	if r.Title == "" {
		return errors.New("title is required")
	}
	if r.Content == "" {
		return errors.New("content is required")
	}
	if r.Priority < 0 || r.Priority > 100 {
		return errors.New("priority must be 0-100")
	}
	return nil
}

// UpdateRequest is the input for updating a project goal (partial update).
type UpdateRequest struct {
	Kind     *GoalKind `json:"kind,omitempty"`
	Title    *string   `json:"title,omitempty"`
	Content  *string   `json:"content,omitempty"`
	Priority *int      `json:"priority,omitempty"`
	Enabled  *bool     `json:"enabled,omitempty"`
}

// Validate checks that any provided fields are valid.
func (r *UpdateRequest) Validate() error {
	if r.Kind != nil && !r.Kind.IsValid() {
		return errors.New("invalid kind: " + string(*r.Kind))
	}
	if r.Priority != nil && (*r.Priority < 0 || *r.Priority > 100) {
		return errors.New("priority must be 0-100")
	}
	return nil
}
