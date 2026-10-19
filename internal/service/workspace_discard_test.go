package service_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-189 (R6-15): a workspace a re-clone replaces is moved aside in the
// tenant directory (the Go Core renames one entry it owns, it never walks
// the tree) and removed by the worker as the tenant's tool UID; the
// project stays.
func TestWorkspaceDeletion_DiscardMovesTheWorkspaceAsideForTheWorker(t *testing.T) {
	store := &fakeDeletionStore{}
	svc, queue := newDeletions(t, store)
	ctx := tenantctx.WithTenant(context.Background(), delTenant)
	tenantDir := filepath.Join(t.TempDir(), delTenant)
	dir := filepath.Join(tenantDir, "proj-1")
	if err := os.MkdirAll(filepath.Join(dir, "tool-only"), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := svc.Discard(ctx, &project.Project{ID: "proj-1"}, dir); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatalf("the workspace is still in place for the fresh clone: %v", err)
	}
	if len(store.deleted) != 0 {
		t.Fatalf("the project was deleted: %v", store.deleted)
	}
	if len(store.recorded) != 1 {
		t.Fatalf("recorded %+v", store.recorded)
	}
	d := store.recorded[0]
	if d.ID == "" || d.TenantID != delTenant || d.ToolUID != 20031 || d.ProjectID != "proj-1" ||
		filepath.Dir(d.WorkspacePath) != tenantDir || !strings.HasPrefix(filepath.Base(d.WorkspacePath), "proj-1.discarded-") {
		t.Fatalf("deletion = %+v", d)
	}
	if _, err := os.Stat(filepath.Join(d.WorkspacePath, "tool-only")); err != nil {
		t.Fatalf("the Go Core removed the old workspace itself: %v", err)
	}
	msg, ok := queue.lastMessage(messagequeue.SubjectWorkspaceDeleteRequest)
	if !ok {
		t.Fatal("no workspace.delete.request published")
	}
	var payload messagequeue.WorkspaceDeleteRequestPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.DeletionID != d.ID || payload.WorkspacePath != d.WorkspacePath || payload.TenantID != delTenant || payload.ToolUID != 20031 {
		t.Fatalf("payload = %+v, deletion %+v", payload, d)
	}

	// A destination that is gone cannot be moved aside.
	if err := svc.Discard(ctx, &project.Project{ID: "proj-1"}, dir); err == nil {
		t.Fatal("Discard of a missing workspace succeeded")
	}
}
