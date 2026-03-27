package service

import (
	"context"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
)

// ContextProvider fetches context entries from a single source.
// Each implementation encapsulates one retrieval strategy (workspace scan,
// retrieval search, GraphRAG, LSP diagnostics, goals, knowledge bases, etc.).
//
// TODO(ARCH-015): Refactor ContextOptimizerService.assembleAndPack to use a
// []ContextProvider registry instead of inline calls. This interface is the
// first step; the full provider-registry migration is a separate effort.
type ContextProvider interface {
	// Name returns a human-readable identifier for logging and debugging.
	Name() string

	// FetchEntries retrieves context entries relevant to the given prompt,
	// constrained by the token budget. Implementations should return entries
	// sorted by priority (descending) when possible.
	FetchEntries(ctx context.Context, projectID, prompt string, budget int) ([]cfcontext.ContextEntry, error)
}
