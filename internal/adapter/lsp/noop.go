package lsp

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/port/codeintel"
)

// NoOpProvider satisfies codeintel.Provider when LSP is unavailable.
type NoOpProvider struct{}

func (NoOpProvider) Initialize(context.Context, string, string) error { return nil }
func (NoOpProvider) Shutdown(context.Context, string) error           { return nil }
func (NoOpProvider) Diagnostics(context.Context, string, string) ([]codeintel.Diagnostic, error) {
	return nil, nil
}
