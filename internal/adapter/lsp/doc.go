// Package lsp implements the Language Server Protocol client adapter.
//
// This adapter is fully implemented and wired into the application lifecycle
// (cmd/codeforge/main.go). It is conditionally enabled via config.LSP.Enabled.
//
// Supported language servers (internal/domain/lsp/language.go):
//   - Go: gopls serve
//   - Python: pyright-langserver --stdio
//   - TypeScript/JavaScript: typescript-language-server --stdio
//
// See also: internal/service/lsp.go (service layer), internal/port/lsp/ (port interfaces)
package lsp
