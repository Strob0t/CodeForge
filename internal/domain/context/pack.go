// Package context defines the ContextPack and SharedContext domain models
// for managing structured context delivery to AI coding agents.
package context

import (
	"errors"
	"time"
)

// EntryKind classifies a context entry.
type EntryKind string

const (
	EntryFile    EntryKind = "file"    // Full file content
	EntrySnippet EntryKind = "snippet" // Partial file / code excerpt
	EntrySummary EntryKind = "summary" // Text summary of a larger body
	EntryShared  EntryKind = "shared"  // Item from SharedContext
)

// ValidEntryKind reports whether k is a known entry kind.
func ValidEntryKind(k EntryKind) bool {
	switch k {
	case EntryFile, EntrySnippet, EntrySummary, EntryShared:
		return true
	}
	return false
}

// ContextPack bundles relevant context (files, snippets, shared state) for a task,
// constrained by a token budget.
type ContextPack struct {
	ID          string         `json:"id"`
	TaskID      string         `json:"task_id"`
	ProjectID   string         `json:"project_id"`
	TokenBudget int            `json:"token_budget"` // maximum tokens allowed
	TokensUsed  int            `json:"tokens_used"`  // estimated tokens consumed
	Entries     []ContextEntry `json:"entries"`
	CreatedAt   time.Time      `json:"created_at"`
}

// ContextEntry is a single piece of context within a pack.
type ContextEntry struct {
	ID       string    `json:"id"`
	PackID   string    `json:"pack_id"`
	Kind     EntryKind `json:"kind"`     // file, snippet, summary, shared
	Path     string    `json:"path"`     // file path relative to workspace (empty for shared/summary)
	Content  string    `json:"content"`  // actual content
	Tokens   int       `json:"tokens"`   // estimated token count for this entry
	Priority int       `json:"priority"` // 0-100, higher = more important
}

// Validate checks that a ContextPack is well-formed.
func (p *ContextPack) Validate() error {
	if p.TaskID == "" {
		return errors.New("task_id is required")
	}
	if p.ProjectID == "" {
		return errors.New("project_id is required")
	}
	if p.TokenBudget <= 0 {
		return errors.New("token_budget must be positive")
	}
	if len(p.Entries) == 0 {
		return errors.New("at least one entry is required")
	}
	for i, e := range p.Entries {
		if err := e.Validate(); err != nil {
			return errors.New("entry " + itoa(i) + ": " + err.Error())
		}
	}
	return nil
}

// Validate checks that a ContextEntry is well-formed.
func (e *ContextEntry) Validate() error {
	if e.Content == "" {
		return errors.New("content is required")
	}
	if !ValidEntryKind(e.Kind) {
		return errors.New("invalid entry kind: " + string(e.Kind))
	}
	return nil
}

// EstimateTokens returns an approximate token count for a string.
// Uses the heuristic 1 token ≈ 4 characters.
func EstimateTokens(s string) int {
	n := len(s) / 4
	if n == 0 && s != "" {
		return 1
	}
	return n
}

// itoa converts a small int to string without importing strconv.
func itoa(i int) string {
	if i < 0 {
		return "-" + itoa(-i)
	}
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}
