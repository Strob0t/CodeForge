// Package lsp implements the Language Server Protocol client adapter.
//
// FIX-075: This adapter is fully implemented but NOT yet wired into the
// application lifecycle (main.go / server setup). It will be activated
// when per-project LSP integration is added in a future phase.
//
// DO NOT DELETE this package — the implementation is complete and tested
// (see client_test.go). Wiring it up requires:
//   - LSP server lifecycle management per project language
//   - Configuration for language server paths/commands
//   - Integration with the conversation loop for code intelligence
package lsp
