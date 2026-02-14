// Package lsp provides a stub for Language Server Protocol client integration.
// This will be implemented in Phase 2 to provide code intelligence to AI agents.
package lsp

import "log/slog"

// Client is a placeholder for an LSP client that connects to language servers
// (gopls, pyright, typescript-language-server, etc.) for diagnostics and completions.
type Client struct {
	language string
}

// NewClient creates a new LSP client stub for the given language.
func NewClient(language string) *Client {
	return &Client{language: language}
}

// Start is a no-op stub. In Phase 2, this will start the LSP server process
// and establish JSON-RPC communication.
func (c *Client) Start() error {
	slog.Info("lsp client stub: start called", "language", c.language)
	return nil
}

// Stop is a no-op stub.
func (c *Client) Stop() error {
	slog.Info("lsp client stub: stop called", "language", c.language)
	return nil
}
