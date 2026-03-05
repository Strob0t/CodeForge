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

// DefaultTenantID is used when no tenant context is available (single-tenant mode).
const DefaultTenantID = "00000000-0000-0000-0000-000000000000"

// CreateRequest is the input for storing a new memory.
type CreateRequest struct {
	TenantID   string            `json:"tenant_id,omitempty"`
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
	RequestID string `json:"request_id,omitempty"`
	TenantID  string `json:"tenant_id,omitempty"`
	ProjectID string `json:"project_id"`
	Query     string `json:"query"`
	TopK      int    `json:"top_k"`
	Kind      Kind   `json:"kind,omitempty"` // empty = all kinds
}

// RecallResult is returned by the Python worker after scoring memories.
type RecallResult struct {
	RequestID string         `json:"request_id"`
	ProjectID string         `json:"project_id"`
	Query     string         `json:"query"`
	Results   []ScoredResult `json:"results"`
	Error     string         `json:"error,omitempty"`
}

// ScoredResult is a single scored memory in a recall result.
type ScoredResult struct {
	ID      string  `json:"id"`
	Content string  `json:"content"`
	Kind    string  `json:"kind"`
	Score   float64 `json:"score"`
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
