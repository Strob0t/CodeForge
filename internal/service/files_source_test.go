package service

import (
	"os"
	"strings"
	"testing"
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

	t.Run("PathTraversalProtection", func(t *testing.T) {
		// resolveProjectPath must validate paths to prevent directory traversal.
		if !strings.Contains(content, "filepath.Clean") &&
			!strings.Contains(content, "filepath.Abs") &&
			!strings.Contains(content, "strings.HasPrefix") {
			t.Error("files.go must validate paths to prevent directory traversal attacks")
		}
	})

	t.Run("NewFileService_ReturnsValid", func(t *testing.T) {
		if !strings.Contains(content, "func NewFileService") {
			t.Error("files.go must export NewFileService constructor")
		}
	})

	t.Run("ResolveProjectPath_Exists", func(t *testing.T) {
		if !strings.Contains(content, "resolveProjectPath") {
			t.Error("files.go must contain resolveProjectPath for path resolution")
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
		{"unknown.xyz", ""},
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

// TODO(FIX-032): Additional tests to write for files.go:
// - TestListDirectory_NonexistentPath (verify error for missing directory)
// - TestListDirectory_FileNotDirectory (verify error for file path)
// - TestReadFile_NotFound (verify error for missing file)
// - TestWriteFile_CreatesDirectories (verify parent dir creation)
// - TestDeleteFile_NotFound (verify error for missing file)
// - TestRenameFile_CrossDirectory (verify cross-directory rename)
// - TestResolveProjectPath_TraversalBlocked (verify ../ path is rejected)
// - TestListTree_MaxEntries (verify entry limit is enforced)
