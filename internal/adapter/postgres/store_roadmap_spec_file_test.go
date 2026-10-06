package postgres_test

import (
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
)

// KI-203: the roadmap records the content hash of each spec file when it
// imported it or wrote it back, so "Sync to file" can tell that a file
// changed since. The record is per roadmap and path, overwritten by each
// import or sync, and only ever read or written in the roadmap's tenant.
func TestStore_SpecFileHash(t *testing.T) {
	store := setupStore(t)
	ctx := ctxWithTenant(t, createTestTenant(t, store))
	otherCtx := ctxWithTenant(t, createTestTenant(t, store))

	proj, err := store.CreateProject(ctx, &project.CreateRequest{Name: "spec-hash-project", Provider: "local"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteProject(ctx, proj.ID) })
	rm, err := store.CreateRoadmap(ctx, roadmap.CreateRoadmapRequest{ProjectID: proj.ID, Title: "spec hashes"})
	if err != nil {
		t.Fatalf("CreateRoadmap: %v", err)
	}

	if _, err := store.GetSpecFileHash(ctx, rm.ID, "TODO.md"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetSpecFileHash before any record = %v, want ErrNotFound", err)
	}
	for _, sum := range []string{"aaaa", "bbbb"} { // the second call overwrites
		if err := store.SetSpecFileHash(ctx, rm.ID, "TODO.md", sum); err != nil {
			t.Fatalf("SetSpecFileHash(%s): %v", sum, err)
		}
		if got, err := store.GetSpecFileHash(ctx, rm.ID, "TODO.md"); err != nil || got != sum {
			t.Fatalf("GetSpecFileHash = %q, %v; want %q", got, err, sum)
		}
	}
	if _, err := store.GetSpecFileHash(ctx, rm.ID, "docs/todo.md"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("another path = %v, want ErrNotFound", err)
	}

	// Another tenant neither reads nor writes the roadmap's records.
	if _, err := store.GetSpecFileHash(otherCtx, rm.ID, "TODO.md"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetSpecFileHash from another tenant = %v, want ErrNotFound", err)
	}
	if err := store.SetSpecFileHash(otherCtx, rm.ID, "TODO.md", "cccc"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SetSpecFileHash from another tenant = %v, want ErrNotFound", err)
	}
	if err := store.SetSpecFileHash(otherCtx, rm.ID, "ROADMAP.md", "cccc"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SetSpecFileHash of a new path from another tenant = %v, want ErrNotFound", err)
	}
	if got, err := store.GetSpecFileHash(ctx, rm.ID, "TODO.md"); err != nil || got != "bbbb" {
		t.Fatalf("after another tenant's write: %q, %v", got, err)
	}

	// The records go with the roadmap.
	if err := store.DeleteRoadmap(ctx, rm.ID); err != nil {
		t.Fatalf("DeleteRoadmap: %v", err)
	}
	if _, err := store.GetSpecFileHash(ctx, rm.ID, "TODO.md"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetSpecFileHash after the roadmap was deleted = %v, want ErrNotFound", err)
	}
}
