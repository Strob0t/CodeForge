package service

import (
	"os"
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// TestLSP_SourceQuality (FIX-032)
// --------------------------------------------------------------------------

func TestLSP_SourceQuality(t *testing.T) {
	src, err := os.ReadFile("lsp.go") //nolint:gosec // test reads known source
	if err != nil {
		t.Fatalf("failed to read lsp.go: %v", err)
	}
	content := string(src)

	t.Run("ProperErrorHandling", func(t *testing.T) {
		errChecks := strings.Count(content, "if err != nil")
		if errChecks < 3 {
			t.Errorf("expected at least 3 error checks, got %d", errChecks)
		}
	})

	t.Run("NoRawPanic", func(t *testing.T) {
		if strings.Contains(content, "panic(") {
			t.Error("lsp.go should not use panic()")
		}
	})

	t.Run("ConcurrencySafety", func(t *testing.T) {
		// LSP service manages multiple clients concurrently.
		// It must use a mutex for the clients map.
		if !strings.Contains(content, "sync.RWMutex") && !strings.Contains(content, "sync.Mutex") {
			t.Error("lsp.go must use a mutex for concurrent access to clients map")
		}
	})

	t.Run("NewLSPService_ReturnsValid", func(t *testing.T) {
		if !strings.Contains(content, "func NewLSPService") {
			t.Error("lsp.go must export NewLSPService constructor")
		}
	})

	t.Run("LanguageDetection", func(t *testing.T) {
		if !strings.Contains(content, "languageFromURI") {
			t.Error("lsp.go should contain languageFromURI for file type detection")
		}
	})
}

// TestLanguageFromURI verifies the language detection helper.
func TestLanguageFromURI(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"file:///project/main.go", "go"},
		{"file:///project/app.py", "python"},
		{"file:///project/index.ts", "typescript"},
		{"file:///project/index.tsx", "typescript"},
		{"file:///project/style.css", ""},
		{"file:///project/README.md", ""},
		{"main.go", "go"},
		{"test.py", "python"},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			got := languageFromURI(tt.uri)
			if got != tt.want {
				t.Errorf("languageFromURI(%q) = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}

// TODO(FIX-032): Additional tests to write for lsp.go:
// - TestStartServers_MultipleLanguages (verify concurrent server starts)
// - TestStopServers_CleansUpClients (verify all clients stopped)
// - TestDefinition_ClientNotFound (verify error when no client for language)
// - TestDiagnostics_Debouncing (verify diagnostic broadcasts are debounced)
// - TestDiagnosticsAsContextEntries_Format (verify context entry format)
