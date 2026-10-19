package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// recordingDeleter stands in for the WorkspaceDeletionService (KI-96 D11).
type recordingDeleter struct {
	deleted   []string
	discarded []string // project:dir
	err       error
}

func (d *recordingDeleter) Delete(_ context.Context, p *project.Project) error {
	d.deleted = append(d.deleted, p.ID)
	return d.err
}

func (d *recordingDeleter) Discard(_ context.Context, p *project.Project, dir string) error {
	d.discarded = append(d.discarded, p.ID+":"+dir)
	return d.err
}

// KI-189 (R6-15): a re-clone that replaces an existing workspace removes it
// through the worker when tool ACLs are required, like a project deletion.
func TestProjectServiceClone_WithToolACLsTheWorkerRemovesAReplacedWorkspace(t *testing.T) {
	svc, _, deleter, wsDir := deleteFixture(t, true)
	p := &project.Project{ID: "p1"}
	o := gitprovider.ApplyCloneOptions(svc.cloneOptions(p, "dev"))
	if o.Branch != "dev" || o.RemoveExisting == nil {
		t.Fatalf("clone options %+v, want branch dev and the worker's removal", o)
	}
	if err := o.RemoveDestination(context.Background(), wsDir); err != nil {
		t.Fatal(err)
	}
	if len(deleter.discarded) != 1 || deleter.discarded[0] != "p1:"+wsDir {
		t.Fatalf("discarded through the worker: %v", deleter.discarded)
	}
	if _, err := os.Stat(wsDir); err != nil {
		t.Fatalf("the Go Core removed the workspace itself: %v", err)
	}

	svc, _, _, _ = deleteFixture(t, false)
	if o := gitprovider.ApplyCloneOptions(svc.cloneOptions(p, "")); o.RemoveExisting != nil {
		t.Fatal("development (tool ACLs off) keeps the Go Core's own removal")
	}
}

func deleteFixture(t *testing.T, required bool) (svc *ProjectService, store *mockStore, deleter *recordingDeleter, wsDir string) {
	t.Helper()
	wsRoot := t.TempDir()
	wsDir = filepath.Join(wsRoot, "tenant", "p1")
	if err := os.MkdirAll(wsDir, 0o750); err != nil {
		t.Fatal(err)
	}
	store = &mockStore{projects: []project.Project{{ID: "p1", Name: "Alpha", WorkspacePath: wsDir}}}
	svc = NewProjectService(store, wsRoot)
	svc.SetToolUIDs(NewToolUIDService(&fakeToolUIDStore{next: 20000}, required))
	deleter = &recordingDeleter{}
	svc.SetWorkspaceDeletions(deleter)
	return svc, store, deleter, wsDir
}

func TestProjectServiceDelete_WithToolACLsTheWorkerRemovesTheWorkspace(t *testing.T) {
	svc, store, deleter, wsDir := deleteFixture(t, true)

	if err := svc.Delete(context.Background(), "p1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(deleter.deleted) != 1 || deleter.deleted[0] != "p1" {
		t.Fatalf("deleted through the worker: %v", deleter.deleted)
	}
	if _, err := os.Stat(wsDir); err != nil {
		t.Fatalf("the Go Core removed the workspace itself: %v", err)
	}
	if len(store.projects) != 1 {
		t.Fatal("the project row is the deletion service's to remove (one transaction)")
	}

	deleter.err = project.ErrProjectBusy
	if err := svc.Delete(context.Background(), "p1"); !errors.Is(err, project.ErrProjectBusy) {
		t.Fatalf("err = %v, want ErrProjectBusy", err)
	}
}

func TestProjectServiceDelete_WithToolACLsOffTheCoreRemovesTheWorkspace(t *testing.T) {
	svc, _, deleter, wsDir := deleteFixture(t, false)

	if err := svc.Delete(context.Background(), "p1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(deleter.deleted) != 0 {
		t.Fatal("development (tool ACLs off) keeps the Go Core's own removal")
	}
	if _, err := os.Stat(wsDir); !os.IsNotExist(err) {
		t.Fatalf("workspace not removed: %v", err)
	}
}
