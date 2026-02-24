// Package lsp defines domain types for Language Server Protocol integration.
// These types represent LSP concepts (diagnostics, locations, symbols) in a
// transport-independent way for use across the service, adapter, and handler layers.
package lsp

// Position in a text document (0-based line and character).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range in a text document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location links a URI to a range.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// DiagnosticSeverity mirrors LSP DiagnosticSeverity.
const (
	SeverityError   = 1
	SeverityWarning = 2
	SeverityInfo    = 3
	SeverityHint    = 4
)

// Diagnostic represents a compiler/linter diagnostic.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"` // 1=Error, 2=Warning, 3=Info, 4=Hint
	Source   string `json:"source"`
	Message  string `json:"message"`
	Code     string `json:"code,omitempty"`
}

// DocumentSymbol represents a symbol in a document (function, class, etc.).
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Kind           int              `json:"kind"` // LSP SymbolKind enum
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// HoverResult contains hover information for a position.
type HoverResult struct {
	Contents string `json:"contents"` // Markdown
	Range    *Range `json:"range,omitempty"`
}

// ServerStatus represents the lifecycle state of a language server.
type ServerStatus string

const (
	ServerStatusStopped  ServerStatus = "stopped"
	ServerStatusStarting ServerStatus = "starting"
	ServerStatusReady    ServerStatus = "ready"
	ServerStatusFailed   ServerStatus = "failed"
)

// ServerInfo describes a running language server instance.
type ServerInfo struct {
	Language    string       `json:"language"`
	Status      ServerStatus `json:"status"`
	Command     string       `json:"command"`
	PID         int          `json:"pid,omitempty"`
	Error       string       `json:"error,omitempty"`
	Diagnostics int          `json:"diagnostics"` // Count of cached diagnostics
}
