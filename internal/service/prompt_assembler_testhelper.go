package service

import (
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
)

// NewTestPromptAssemblerWithFingerprint creates a PromptAssembler for testing
// that returns a known fingerprint for specific mode IDs. It builds a minimal
// in-memory prompt library with one entry per mode. Empty-condition entries
// match all contexts, so fingerprints are deterministic per library content.
func NewTestPromptAssemblerWithFingerprint(modeFingerprints map[string]string) *PromptAssembler {
	// Build one entry per mode with unique content (drives the fingerprint).
	var entries []prompt.PromptEntry
	for modeID, fp := range modeFingerprints {
		entries = append(entries, prompt.PromptEntry{
			ID:       "test-" + modeID,
			Name:     "test entry for " + modeID,
			Content:  fp, // content drives the fingerprint hash
			Category: prompt.CategoryIdentity,
			Priority: 50,
			Conditions: prompt.Conditions{
				Modes: []string{modeID},
			},
		})
	}

	lib := &PromptLibraryService{entries: entries}
	return NewPromptAssembler(lib, 0)
}
