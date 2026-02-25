package service

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// PromptSectionService manages prompt section CRUD operations.
type PromptSectionService struct {
	store database.Store
}

// NewPromptSectionService creates a new prompt section service.
func NewPromptSectionService(store database.Store) *PromptSectionService {
	return &PromptSectionService{store: store}
}

// List returns all prompt sections for a given scope.
func (s *PromptSectionService) List(ctx context.Context, scope string) ([]prompt.SectionRow, error) {
	return s.store.ListPromptSections(ctx, scope)
}

// Upsert creates or updates a prompt section.
func (s *PromptSectionService) Upsert(ctx context.Context, row *prompt.SectionRow) error {
	return s.store.UpsertPromptSection(ctx, row)
}

// Delete removes a prompt section by ID.
func (s *PromptSectionService) Delete(ctx context.Context, id string) error {
	return s.store.DeletePromptSection(ctx, id)
}

// Preview assembles a prompt from the given sections and returns the result with token counts.
func (s *PromptSectionService) Preview(sections []PromptSection, budget int) (string, []PromptSection) {
	if budget > 0 {
		sections = PruneToFitBudget(sections, budget)
	}
	text := AssembleSections(sections)
	return text, sections
}
