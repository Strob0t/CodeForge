package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
)

// PromptStore defines database operations for prompt section management.
type PromptStore interface {
	ListPromptSections(ctx context.Context, scope string) ([]prompt.SectionRow, error)
	UpsertPromptSection(ctx context.Context, row *prompt.SectionRow) error
	DeletePromptSection(ctx context.Context, id string) error
}
