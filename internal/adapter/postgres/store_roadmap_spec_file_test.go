package postgres_test

import (
	"errors"
	"maps"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
)

// KI-203: the roadmap records the content hash of each spec file and the
// state of each imported checkbox when it imported the file or wrote it
// back, so "Sync to file" can tell that a file changed since and an import
// which boxes did. The record is per roadmap and path, overwritten by each
// import or sync, and only ever read or written in the roadmap's tenant.
func TestStore_SpecFile(t *testing.T) {
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

	if _, err := store.GetSpecFile(ctx, rm.ID, "TODO.md"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetSpecFile before any record = %v, want ErrNotFound", err)
	}
	// The second call overwrites; a nil Checked is stored as no boxes.
	for _, want := range []roadmap.SpecFile{
		{RoadmapID: rm.ID, Path: "TODO.md", ContentSHA256: "aaaa", Checked: map[string]bool{"f-1": true, "f-2": false}},
		{RoadmapID: rm.ID, Path: "TODO.md", ContentSHA256: "bbbb"},
	} {
		if err := store.SetSpecFile(ctx, &want); err != nil {
			t.Fatalf("SetSpecFile(%+v): %v", want, err)
		}
		got, err := store.GetSpecFile(ctx, rm.ID, "TODO.md")
		if err != nil || got.ContentSHA256 != want.ContentSHA256 || !maps.Equal(got.Checked, want.Checked) || got.RoadmapID != rm.ID || got.Path != "TODO.md" {
			t.Fatalf("GetSpecFile = %+v, %v; want %+v", got, err, want)
		}
	}
	if _, err := store.GetSpecFile(ctx, rm.ID, "docs/todo.md"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("another path = %v, want ErrNotFound", err)
	}

	// Another tenant neither reads nor writes the roadmap's records.
	if _, err := store.GetSpecFile(otherCtx, rm.ID, "TODO.md"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetSpecFile from another tenant = %v, want ErrNotFound", err)
	}
	for _, path := range []string{"TODO.md", "ROADMAP.md"} {
		if err := store.SetSpecFile(otherCtx, &roadmap.SpecFile{RoadmapID: rm.ID, Path: path, ContentSHA256: "cccc"}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("SetSpecFile(%s) from another tenant = %v, want ErrNotFound", path, err)
		}
	}
	if got, err := store.GetSpecFile(ctx, rm.ID, "TODO.md"); err != nil || got.ContentSHA256 != "bbbb" {
		t.Fatalf("after another tenant's write: %+v, %v", got, err)
	}

	// The records go with the roadmap.
	if err := store.DeleteRoadmap(ctx, rm.ID); err != nil {
		t.Fatalf("DeleteRoadmap: %v", err)
	}
	if _, err := store.GetSpecFile(ctx, rm.ID, "TODO.md"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetSpecFile after the roadmap was deleted = %v, want ErrNotFound", err)
	}
}
