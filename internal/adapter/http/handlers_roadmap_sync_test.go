package http_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
)

// KI-203: a refused "Sync to file" answers 409 and says why (here: the
// roadmap is never rendered over an existing spec file), so the user knows
// to import the specs first; the file is not touched.
func TestHandleSyncToSpecFile_RefusalSaysWhy(t *testing.T) {
	ws := t.TempDir()
	todo := filepath.Join(ws, "TODO.md")
	if err := os.WriteFile(todo, []byte("# Mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &mockStore{}
	store.projects = append(store.projects, project.Project{ID: "proj-1", Name: "test", WorkspacePath: ws})
	store.roadmaps = append(store.roadmaps, roadmap.Roadmap{ID: "rm-1", ProjectID: "proj-1", Title: "Plan"})
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/roadmap/sync-to-file", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "TODO.md exists") {
		t.Fatalf("sync-to-file = %d %s; want 409 naming TODO.md", w.Code, w.Body.String())
	}
	if got, _ := os.ReadFile(todo); string(got) != "# Mine\n" { //nolint:gosec // test file
		t.Fatalf("TODO.md was overwritten: %q", got)
	}
}
