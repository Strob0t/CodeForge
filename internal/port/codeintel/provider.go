// Package codeintel defines the port for language intelligence capabilities.
package codeintel

import "context"

// Provider abstracts language intelligence capabilities (go-to-definition,
// diagnostics, etc.). Implemented by the LSP adapter; a no-op fallback
// is used when LSP is not available.
type Provider interface {
	Initialize(ctx context.Context, projectID, workspacePath string) error
	Shutdown(ctx context.Context, projectID string) error
	Diagnostics(ctx context.Context, projectID, filePath string) ([]Diagnostic, error)
}

// Diagnostic represents a code issue reported by a language server.
type Diagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}
