package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

// --------------------------------------------------------------------------
// TestFiles_SourceQuality (FIX-032)
// --------------------------------------------------------------------------

func TestFiles_SourceQuality(t *testing.T) {
	src, err := os.ReadFile("files.go") //nolint:gosec // test reads known source
	if err != nil {
		t.Fatalf("failed to read files.go: %v", err)
	}
	content := string(src)

	t.Run("ProperErrorHandling", func(t *testing.T) {
		errChecks := strings.Count(content, "if err != nil")
		if errChecks < 5 {
			t.Errorf("expected at least 5 error checks, got %d", errChecks)
		}
	})

	t.Run("NoRawPanic", func(t *testing.T) {
		if strings.Contains(content, "panic(") {
			t.Error("files.go should not use panic()")
		}
	})

	// Every path is resolved inside the workspace by os.Root (KI-95); the
	// former check-then-open with EvalSymlinks could be raced by a symlink swap.
	t.Run("PathTraversalProtection", func(t *testing.T) {
		if !strings.Contains(content, "workspacefs.Open") {
			t.Error("files.go must open workspaces through workspacefs (os.Root)")
		}
		if strings.Contains(content, "EvalSymlinks") || strings.Contains(content, "os.ReadFile") ||
			strings.Contains(content, "os.WriteFile") {
			t.Error("files.go must not resolve or open workspace paths outside workspacefs")
		}
	})

	t.Run("NewFileService_ReturnsValid", func(t *testing.T) {
		if !strings.Contains(content, "func NewFileService") {
			t.Error("files.go must export NewFileService constructor")
		}
	})

	t.Run("OpenWorkspace_Exists", func(t *testing.T) {
		if !strings.Contains(content, "openWorkspace") {
			t.Error("files.go must contain openWorkspace for path resolution")
		}
	})
}

// TestDetectLanguage verifies the language detection helper.
func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"app.py", "python"},
		{"index.ts", "typescript"},
		{"index.tsx", "typescript"},
		{"style.css", "css"},
		{"data.json", "json"},
		{"config.yaml", "yaml"},
		{"unknown.xyz", "plaintext"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectLanguage(tt.path)
			if got != tt.want {
				t.Errorf("detectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// newTestFileService creates a FileService backed by a mockStore whose project
// workspace points at wsDir.
func newTestFileService(wsDir string) *FileService {
	store := &mockStore{
		projects: []project.Project{{
			ID:            "p1",
			Name:          "Test",
			WorkspacePath: wsDir,
		}},
	}
	return NewFileService(store)
}

func TestListDirectory_NonexistentPath(t *testing.T) {
	wsDir := t.TempDir()
	svc := newTestFileService(wsDir)

	_, err := svc.ListDirectory(context.Background(), "p1", "no-such-dir")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' in error, got: %v", err)
	}
}

func TestListDirectory_FileNotDirectory(t *testing.T) {
	wsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(wsDir, "file.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := newTestFileService(wsDir)

	_, err := svc.ListDirectory(context.Background(), "p1", "file.txt")
	if err == nil {
		t.Fatal("expected error when path is a file, not a directory")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected 'not a directory' in error, got: %v", err)
	}
}

func TestReadFile_NotFound(t *testing.T) {
	wsDir := t.TempDir()
	svc := newTestFileService(wsDir)

	_, err := svc.ReadFile(context.Background(), "p1", "nonexistent.go")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' in error, got: %v", err)
	}
}

func TestWriteFile_CreatesDirectories(t *testing.T) {
	wsDir := t.TempDir()
	svc := newTestFileService(wsDir)

	// WriteFile creates the missing parent directories inside the workspace.
	relPath := filepath.Join("sub", "deeper", "newfile.txt")
	err := svc.WriteFile(context.Background(), "p1", relPath, "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	absPath := filepath.Join(wsDir, "sub", "deeper", "newfile.txt")
	data, readErr := os.ReadFile(absPath) //nolint:gosec // test reads from t.TempDir()
	if readErr != nil {
		t.Fatalf("file not created: %v", readErr)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestDeleteFile_NotFound(t *testing.T) {
	wsDir := t.TempDir()
	svc := newTestFileService(wsDir)

	err := svc.DeleteFile(context.Background(), "p1", "nonexistent.go")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent file")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' in error, got: %v", err)
	}
}

func TestRenameFile_CrossDirectory(t *testing.T) {
	wsDir := t.TempDir()

	srcDir := filepath.Join(wsDir, "subdir-a")
	dstDir := filepath.Join(wsDir, "subdir-b")
	if err := os.MkdirAll(srcDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dstDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := newTestFileService(wsDir)

	err := svc.RenameFile(context.Background(), "p1", "subdir-a/file.txt", "subdir-b/file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(wsDir, "subdir-a", "file.txt")); !os.IsNotExist(statErr) {
		t.Error("expected old file to be removed after rename")
	}

	data, readErr := os.ReadFile(filepath.Join(wsDir, "subdir-b", "file.txt")) //nolint:gosec // test reads from t.TempDir()
	if readErr != nil {
		t.Fatalf("new file not found: %v", readErr)
	}
	if string(data) != "data" {
		t.Errorf("expected 'data', got %q", string(data))
	}
}

func TestResolveProjectPath_TraversalBlocked(t *testing.T) {
	wsDir := t.TempDir()
	svc := newTestFileService(wsDir)

	_, err := svc.ReadFile(context.Background(), "p1", "../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal attempt")
	}
	// A refused path is the client's error with a clear message (KI-95).
	if !errors.Is(err, workspacefs.ErrLeavesWorkspace) || !errors.Is(err, domain.ErrValidation) {
		t.Errorf("expected a validation error that the path leaves the workspace, got: %v", err)
	}
}

func TestListTree_MaxEntries(t *testing.T) {
	wsDir := t.TempDir()

	for i := range 10 {
		name := filepath.Join(wsDir, strings.Repeat("f", i+1)+".txt")
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	svc := newTestFileService(wsDir)

	entries, err := svc.ListTree(context.Background(), "p1", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) > 3 {
		t.Errorf("expected at most 3 entries, got %d", len(entries))
	}
}
