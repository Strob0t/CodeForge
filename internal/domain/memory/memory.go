// Package memory provides the domain model for persistent agent memory
// with composite scoring (semantic + recency + importance).
package memory

import (
	"errors"
	"slices"
	"time"
)

// Kind categorizes the type of memory entry.
type Kind string

const (
	KindObservation Kind = "observation"
	KindDecision    Kind = "decision"
	KindError       Kind = "error"
	KindInsight     Kind = "insight"
)

// ValidKinds lists all valid memory kinds.
var ValidKinds = []Kind{KindObservation, KindDecision, KindError, KindInsight}

// Memory represents a single agent memory entry.
type Memory struct {
	ID         string            `json:"id"`
	TenantID   string            `json:"tenant_id"`
	ProjectID  string            `json:"project_id"`
	AgentID    string            `json:"agent_id,omitempty"`
	RunID      string            `json:"run_id,omitempty"`
	Content    string            `json:"content"`
	Kind       Kind              `json:"kind"`
	Importance float64           `json:"importance"`
	Embedding  []byte            `json:"-"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

// ScoredMemory wraps a Memory with its composite retrieval score.
type ScoredMemory struct {
	Memory
	Score float64 `json:"score"`
}

// CreateRequest is the input for storing a new memory.
type CreateRequest struct {
	ProjectID  string            `json:"project_id"`
	AgentID    string            `json:"agent_id,omitempty"`
	RunID      string            `json:"run_id,omitempty"`
	Content    string            `json:"content"`
	Kind       Kind              `json:"kind"`
	Importance float64           `json:"importance"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// RecallRequest is the input for querying memories.
type RecallRequest struct {
	ProjectID string `json:"project_id"`
	Query     string `json:"query"`
	TopK      int    `json:"top_k"`
	Kind      Kind   `json:"kind,omitempty"` // empty = all kinds
}

// Validate checks that a CreateRequest has all required fields.
func (r *CreateRequest) Validate() error {
	if r.ProjectID == "" {
		return errors.New("project_id is required")
	}
	if r.Content == "" {
		return errors.New("content is required")
	}
	if !slices.Contains(ValidKinds, r.Kind) {
		return errors.New("invalid kind: must be observation, decision, error, or insight")
	}
	if r.Importance < 0 || r.Importance > 1 {
		return errors.New("importance must be between 0 and 1")
	}
	return nil
}
