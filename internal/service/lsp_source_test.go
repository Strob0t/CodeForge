package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	lspDomain "github.com/Strob0t/CodeForge/internal/domain/lsp"
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

// newTestLSPService creates an LSPService with a noop broadcaster and no real clients.
// Uses noopBroadcaster defined in autoagent_test.go.
func newTestLSPService() *LSPService {
	cfg := &config.LSP{
		Enabled:         true,
		StartTimeout:    5 * time.Second,
		ShutdownTimeout: 5 * time.Second,
		DiagnosticDelay: 100 * time.Millisecond,
	}
	return NewLSPService(cfg, &noopBroadcaster{}, &mockStore{}, nil)
}

// NOTE(FIX-032): TestStartServers_MultipleLanguages is skipped because it
// requires launching real language server processes (gopls, pyright, etc.)
// which are not available in the test environment.

func TestStopServers_CleansUpClients(t *testing.T) {
	svc := newTestLSPService()

	// Stopping servers for a project that has no clients should not error.
	err := svc.StopServers(context.Background(), "nonexistent-project")
	if err != nil {
		t.Fatalf("expected no error stopping servers for unknown project, got: %v", err)
	}
}

func TestDefinition_ClientNotFound(t *testing.T) {
	svc := newTestLSPService()

	// No servers running — should return error.
	_, err := svc.Definition(context.Background(), "p1", "file:///main.go", lspDomain.Position{Line: 0, Character: 0})
	if err == nil {
		t.Fatal("expected error when no LSP servers are running")
	}
	if !strings.Contains(err.Error(), "no LSP servers running") {
		t.Errorf("expected 'no LSP servers running' error, got: %v", err)
	}
}

// NOTE(FIX-032): TestDiagnostics_Debouncing is skipped because it requires
// real LSP clients that produce diagnostics, plus time-based assertions on
// the debounce timer. This is an integration test.

func TestDiagnosticsAsContextEntries_Format(t *testing.T) {
	svc := newTestLSPService()

	// No clients registered — should return nil.
	entries := svc.DiagnosticsAsContextEntries("p1")
	if entries != nil {
		t.Errorf("expected nil entries for project with no clients, got %d", len(entries))
	}
}

func TestStatus_NoClients(t *testing.T) {
	svc := newTestLSPService()

	// No clients registered — should return nil.
	infos := svc.Status("p1")
	if infos != nil {
		t.Errorf("expected nil status for project with no clients, got %d", len(infos))
	}
}

func TestDiagnostics_NoClients(t *testing.T) {
	svc := newTestLSPService()

	// No clients registered — should return nil.
	diags := svc.Diagnostics("p1", "")
	if diags != nil {
		t.Errorf("expected nil diagnostics for project with no clients, got %d", len(diags))
	}
}

func TestStartServers_EmptyWorkspacePath(t *testing.T) {
	svc := newTestLSPService()

	// Empty workspace path should return an error.
	err := svc.StartServers(context.Background(), "p1", "", nil)
	if err == nil {
		t.Fatal("expected error for empty workspace path")
	}
	if !strings.Contains(err.Error(), "workspace path is empty") {
		t.Errorf("expected 'workspace path is empty' error, got: %v", err)
	}
}
