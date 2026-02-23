package knowledgebase

import (
	"errors"
	"time"
)

// Category classifies a knowledge base.
type Category string

const (
	CategoryFramework Category = "framework"
	CategoryParadigm  Category = "paradigm"
	CategoryLanguage  Category = "language"
	CategorySecurity  Category = "security"
	CategoryCustom    Category = "custom"
)

// ValidCategory reports whether c is a known category.
func ValidCategory(c Category) bool {
	switch c {
	case CategoryFramework, CategoryParadigm, CategoryLanguage, CategorySecurity, CategoryCustom:
		return true
	}
	return false
}

// Status represents the indexing status of a knowledge base.
type Status string

const (
	StatusPending Status = "pending"
	StatusIndexed Status = "indexed"
	StatusError   Status = "error"
)

// KnowledgeBase represents a curated knowledge module that can be attached to scopes.
type KnowledgeBase struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Category    Category  `json:"category"`
	Tags        []string  `json:"tags"`
	Builtin     bool      `json:"builtin"`
	ContentPath string    `json:"content_path"`
	Status      Status    `json:"status"`
	ChunkCount  int       `json:"chunk_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateRequest holds the input for creating a knowledge base.
type CreateRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    Category `json:"category"`
	Tags        []string `json:"tags"`
	ContentPath string   `json:"content_path"`
	Builtin     bool     `json:"-"` // set internally, not from API
}

// Validate checks that a CreateRequest is well-formed.
func (r *CreateRequest) Validate() error {
	if r.Name == "" {
		return errors.New("name is required")
	}
	if !ValidCategory(r.Category) {
		return errors.New("invalid category: " + string(r.Category))
	}
	return nil
}

// UpdateRequest holds the input for updating a knowledge base.
type UpdateRequest struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"` // full replace when non-nil
}
