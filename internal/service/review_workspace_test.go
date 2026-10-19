package service

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/boundary"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/git"
)

// KI-94: git quotes paths with non-ASCII bytes, quotes, backslashes or
// control characters in its diff output ("sch\303\244ma.proto") unless it
// writes NUL-separated output. The change of a refactoring must name such
// paths as they are, or a boundary file among them is never matched.

func TestChangeBetween_PathsAreNotQuoted(t *testing.T) {
	ctx := context.Background()
	dir := newReviewWorkspace(t)
	for _, name := range []string{"ä.go", "old ü.go", `with "quote".go`, `back\slash.go`} {
		writeLines(t, dir, name, 5, "line")
	}
	reviewGit(t, dir, "add", "-A")
	reviewGit(t, dir, "commit", "-q", "-m", "files")
	from := reviewGit(t, dir, "rev-parse", "HEAD")

	writeLines(t, dir, "ä.go", 7, "line")            // modified: 2 lines added
	reviewGit(t, dir, "mv", "old ü.go", "new ü.go")  // renamed
	writeLines(t, dir, "tab\tname.go", 1, "x")       // added
	reviewGit(t, dir, "rm", "-q", `with "quote".go`) // deleted
	writeLines(t, dir, `back\slash.go`, 6, "line")   // modified: 1 line added
	reviewGit(t, dir, "add", "-A")
	reviewGit(t, dir, "commit", "-q", "-m", "change")
	to := reviewGit(t, dir, "rev-parse", "HEAD")

	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	change, err := changeBetween(ctx, repo, from, to)
	if err != nil {
		t.Fatalf("changeBetween: %v", err)
	}
	want := []string{"ä.go", "old ü.go", "new ü.go", "tab\tname.go", `with "quote".go`, `back\slash.go`}
	for _, p := range want {
		if !slices.Contains(change.Paths, p) {
			t.Errorf("paths %q miss %q", change.Paths, p)
		}
	}
	if len(change.Paths) != len(want) {
		t.Errorf("paths = %q, want %q", change.Paths, want)
	}
	if s := change.Stats; s.FilesChanged != 5 || s.LinesAdded != 4 || s.LinesRemoved != 5 || !s.Structural {
		t.Errorf("stats = %+v, want 5 files, 4 lines added, 5 removed, structural", s)
	}
}

func TestChangeBetween_NoChange(t *testing.T) {
	ctx := context.Background()
	dir := newReviewWorkspace(t)
	head := reviewGit(t, dir, "rev-parse", "HEAD")
	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	change, err := changeBetween(ctx, repo, head, head)
	if err != nil {
		t.Fatalf("changeBetween: %v", err)
	}
	if len(change.Paths) != 0 || change.Stats != (DiffStats{}) {
		t.Fatalf("change = %+v, want none", change)
	}
}

func TestReviewPipeline_NonASCIIBoundaryCrossesLayers(t *testing.T) {
	const name = "schemä.proto"
	f, step := gateFixture(t, []boundary.BoundaryFile{{Path: name, Type: boundary.BoundaryTypeAPI}}, func(dir string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("message A {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	if got := gateNow(f, step); got != plan.StepStatusWaitingApproval {
		t.Fatalf("GateStep = %s, want waiting_approval", got)
	}
	events := f.hub.snapshot()
	if len(events) != 1 || events[0].EventType != event.EventReviewApprovalRequired || !events[0].Data.CrossLayer {
		t.Fatalf("events = %+v, want one approval request that crosses layers", events)
	}
}
