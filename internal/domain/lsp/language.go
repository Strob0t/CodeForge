package lsp

// LanguageServerConfig defines how to launch a language server for a given language.
type LanguageServerConfig struct {
	Command  []string       // e.g. ["gopls", "serve"]
	InitOpts map[string]any // LSP initializationOptions (optional)
}

// DefaultServers maps detected language names (matching project/scan.go's manifestMap)
// to their default server configurations. All servers communicate via stdio.
var DefaultServers = map[string]LanguageServerConfig{
	"go":         {Command: []string{"gopls", "serve"}},
	"python":     {Command: []string{"pyright-langserver", "--stdio"}},
	"typescript": {Command: []string{"typescript-language-server", "--stdio"}},
	"javascript": {Command: []string{"typescript-language-server", "--stdio"}},
}
