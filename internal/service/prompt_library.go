package service

import (
	"io/fs"
	"log/slog"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
)

// PromptLibraryService loads and indexes prompt entries from YAML files.
type PromptLibraryService struct {
	mu      sync.RWMutex
	entries []prompt.PromptEntry
}

// NewPromptLibraryService creates a new library from the given filesystem.
// It loads all YAML files from root within fsys.
func NewPromptLibraryService(fsys fs.FS, root string) (*PromptLibraryService, error) {
	entries, err := prompt.LoadFS(fsys, root)
	if err != nil {
		return nil, err
	}
	return &PromptLibraryService{entries: entries}, nil
}

// Query returns all entries matching the given assembly context.
func (s *PromptLibraryService) Query(ctx prompt.AssemblyContext) []prompt.PromptEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []prompt.PromptEntry
	for i := range s.entries {
		if s.entries[i].Matches(ctx) {
			result = append(result, s.entries[i])
		}
	}
	return result
}

// GetByCategory returns all entries with the given category.
func (s *PromptLibraryService) GetByCategory(cat prompt.Category) []prompt.PromptEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []prompt.PromptEntry
	for i := range s.entries {
		if s.entries[i].Category == cat {
			result = append(result, s.entries[i])
		}
	}
	return result
}

// Len returns the total number of loaded entries.
func (s *PromptLibraryService) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// GetEntry returns the entry with the given ID, or nil if not found.
func (s *PromptLibraryService) GetEntry(id string) *prompt.PromptEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.entries {
		if s.entries[i].ID == id {
			entry := s.entries[i]
			return &entry
		}
	}
	return nil
}

// LoadOverlay adds entries from an overlay filesystem, replacing any entries
// with matching IDs.
func (s *PromptLibraryService) LoadOverlay(fsys fs.FS, root string) error {
	overlay, err := prompt.LoadFS(fsys, root)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Build index of existing IDs for replacement.
	idIndex := make(map[string]int, len(s.entries))
	for i := range s.entries {
		idIndex[s.entries[i].ID] = i
	}
	for i := range overlay {
		if idx, ok := idIndex[overlay[i].ID]; ok {
			s.entries[idx] = overlay[i] // Replace existing.
			slog.Debug("prompt overlay replaced entry", "id", overlay[i].ID)
		} else {
			s.entries = append(s.entries, overlay[i])
			idIndex[overlay[i].ID] = len(s.entries) - 1
		}
	}
	return nil
}
