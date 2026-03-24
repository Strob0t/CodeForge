// Package lsp defines the port interface for Language Server Protocol clients.
package lsp

import (
	"context"

	lspDomain "github.com/Strob0t/CodeForge/internal/domain/lsp"
)

// Client abstracts a single language server client for service layer decoupling.
// Each client manages one language server process for a specific workspace.
type Client interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	SetDiagnosticCallback(fn func(uri string, diags []lspDomain.Diagnostic))

	Status() lspDomain.ServerStatus
	Language() string
	PID() int
	DiagnosticCount() int

	Definition(ctx context.Context, uri string, pos lspDomain.Position) ([]lspDomain.Location, error)
	References(ctx context.Context, uri string, pos lspDomain.Position) ([]lspDomain.Location, error)
	DocumentSymbols(ctx context.Context, uri string) ([]lspDomain.DocumentSymbol, error)
	Hover(ctx context.Context, uri string, pos lspDomain.Position) (*lspDomain.HoverResult, error)
	Diagnostics(uri string) []lspDomain.Diagnostic
	AllDiagnostics() map[string][]lspDomain.Diagnostic
}

// ClientFactory creates LSP clients for a given language and workspace.
// The service layer uses this instead of directly importing the adapter constructor.
type ClientFactory func(language string, serverCfg lspDomain.LanguageServerConfig, workspacePath string) Client
