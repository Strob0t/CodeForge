package context

import (
	"errors"
	"time"
)

// RepoMap represents a repository structure map used for providing
// codebase context to AI coding agents.
type RepoMap struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	MapText     string    `json:"map_text"`
	TokenCount  int       `json:"token_count"`
	FileCount   int       `json:"file_count"`
	SymbolCount int       `json:"symbol_count"`
	Languages   []string  `json:"languages"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Validate checks that a RepoMap is well-formed.
func (m *RepoMap) Validate() error {
	if m.ProjectID == "" {
		return errors.New("project_id is required")
	}
	if m.MapText == "" {
		return errors.New("map_text is required")
	}
	return nil
}
