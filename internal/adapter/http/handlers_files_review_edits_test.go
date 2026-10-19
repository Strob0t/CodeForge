package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-94: a change made through the file API is recorded for the user who
// made it, so the review pipeline's approval dialog can name them.

type handlerEdit struct {
	userID string
	op     review.UserEditOp
	paths  []string
}

type handlerEditRecorder struct {
	mu    sync.Mutex
	err   error
	edits []handlerEdit
}

func (r *handlerEditRecorder) RecordReviewUserEdits(_ context.Context, _, userID string, op review.UserEditOp, paths []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.edits = append(r.edits, handlerEdit{userID: userID, op: op, paths: slices.Clone(paths)})
	return r.err
}

func fileEditRouter(t *testing.T, rec *handlerEditRecorder) (router http.Handler, dir string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &mockStore{}
	store.projects = append(store.projects, project.Project{ID: "proj-1", Name: "test", WorkspacePath: dir})
	router = newTestRouterWithLLM(store, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000",
		func(h *cfhttp.Handlers) { h.Files.SetReviewEditRecorder(rec) })
	return router, dir
}

func TestFileHandlers_RecordTheEditor(t *testing.T) {
	for _, tt := range []struct {
		name, method, path, body string
		want                     handlerEdit
	}{
		{"write", http.MethodPut, "/api/v1/projects/proj-1/files/content", `{"path":"a.go","content":"new"}`,
			handlerEdit{userID: "test-admin", op: review.UserEditWrite, paths: []string{"a.go"}}},
		{"delete", http.MethodDelete, "/api/v1/projects/proj-1/files?path=a.go", "",
			handlerEdit{userID: "test-admin", op: review.UserEditDelete, paths: []string{"a.go"}}},
		{"rename", http.MethodPatch, "/api/v1/projects/proj-1/files/rename", `{"old_path":"a.go","new_path":"b.go"}`,
			handlerEdit{userID: "test-admin", op: review.UserEditRename, paths: []string{"a.go", "b.go"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := &handlerEditRecorder{}
			r, _ := fileEditRouter(t, rec)
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("%s %s = %d %s, want 200", tt.method, tt.path, w.Code, w.Body.String())
			}
			if len(rec.edits) != 1 || rec.edits[0].userID != tt.want.userID || rec.edits[0].op != tt.want.op ||
				!slices.Equal(rec.edits[0].paths, tt.want.paths) {
				t.Fatalf("recorded %+v, want %+v", rec.edits, tt.want)
			}
		})
	}
}

// A request whose account is gone cannot have its change recorded: the
// change is refused as unauthenticated and the file is left alone.
func TestFileHandlers_UnrecordableEditIsRefused(t *testing.T) {
	rec := &handlerEditRecorder{err: user.ErrAccountGone}
	r, dir := fileEditRouter(t, rec)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/proj-1/files/content",
		strings.NewReader(`{"path":"a.go","content":"new"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("PUT = %d %s, want 401", w.Code, w.Body.String())
	}
	if data, err := os.ReadFile(filepath.Join(dir, "a.go")); err != nil || string(data) != "old" { //nolint:gosec // test reads from t.TempDir()
		t.Fatalf("a.go = %q, %v, want it unchanged", data, err)
	}
}
