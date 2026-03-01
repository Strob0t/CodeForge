package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/service"
)

// --- File Browser Handler Tests ---

func TestHandleListFiles_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files.
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}

	store := &mockStore{}
	store.projects = append(store.projects, project.Project{
		ID:            "proj-1",
		Name:          "test",
		WorkspacePath: tmpDir,
	})
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/projects/proj-1/files?path=.", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var entries []service.FileEntry
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}

	// Verify we got the file and directory.
	var foundFile, foundDir bool
	for _, e := range entries {
		if e.Name == "main.go" && !e.IsDir {
			foundFile = true
		}
		if e.Name == "pkg" && e.IsDir {
			foundDir = true
		}
	}
	if !foundFile {
		t.Fatal("expected main.go in listing")
	}
	if !foundDir {
		t.Fatal("expected pkg/ directory in listing")
	}
}

func TestHandleListFiles_DefaultPath(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := &mockStore{}
	store.projects = append(store.projects, project.Project{
		ID:            "proj-1",
		Name:          "test",
		WorkspacePath: tmpDir,
	})
	r := newTestRouterWithStore(store)

	// No path query param — should default to "."
	req := httptest.NewRequest("GET", "/api/v1/projects/proj-1/files", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListFiles_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	store := &mockStore{}
	store.projects = append(store.projects, project.Project{
		ID:            "proj-1",
		Name:          "test",
		WorkspacePath: tmpDir,
	})
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/projects/proj-1/files?path=../../etc", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Path traversal must be blocked — not 200.
	if w.Code == http.StatusOK {
		t.Fatalf("expected path traversal to be blocked, but got 200")
	}
}

func TestHandleReadFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	content := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	store := &mockStore{}
	store.projects = append(store.projects, project.Project{
		ID:            "proj-1",
		Name:          "test",
		WorkspacePath: tmpDir,
	})
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/projects/proj-1/files/content?path=main.go", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var fc service.FileContent
	if err := json.NewDecoder(w.Body).Decode(&fc); err != nil {
		t.Fatal(err)
	}
	if fc.Content != content {
		t.Fatalf("expected content %q, got %q", content, fc.Content)
	}
	if fc.Language != "go" {
		t.Fatalf("expected language 'go', got %q", fc.Language)
	}
}

func TestHandleReadFile_MissingPath(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/projects/proj-1/files/content", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWriteFile_Success(t *testing.T) {
	tmpDir := t.TempDir()

	store := &mockStore{}
	store.projects = append(store.projects, project.Project{
		ID:            "proj-1",
		Name:          "test",
		WorkspacePath: tmpDir,
	})
	r := newTestRouterWithStore(store)

	body, _ := json.Marshal(map[string]string{
		"path":    "newfile.txt",
		"content": "hello world",
	})
	req := httptest.NewRequest("PUT", "/api/v1/projects/proj-1/files/content", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the file was actually written.
	data, err := os.ReadFile(filepath.Join(tmpDir, "newfile.txt")) //nolint:gosec // test-only: path from t.TempDir()
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", string(data))
	}
}

func TestHandleWriteFile_MissingPath(t *testing.T) {
	store := &mockStore{}
	store.projects = append(store.projects, project.Project{
		ID:            "proj-1",
		Name:          "test",
		WorkspacePath: t.TempDir(),
	})
	r := newTestRouterWithStore(store)

	body, _ := json.Marshal(map[string]string{
		"content": "hello",
	})
	req := httptest.NewRequest("PUT", "/api/v1/projects/proj-1/files/content", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
