package gitlocal_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// KI-189 (R6-15): a destination that is no clone of the URL is removed by
// the caller's remover (with tool ACLs required, the worker as the tenant's
// tool UID), never by the Go Core when one is given.
func TestClone_ReclonesThroughTheCallersRemover(t *testing.T) {
	ctx := context.Background()
	src := initTestRepo(t)
	dest := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(filepath.Join(dest, "tool-only"), 0o750); err != nil {
		t.Fatal(err)
	}
	p, err := gitprovider.New("local", nil)
	if err != nil {
		t.Fatal(err)
	}

	var removed []string
	aside := dest + ".discarded"
	remove := func(_ context.Context, dir string) error {
		removed = append(removed, dir)
		return os.Rename(dir, aside)
	}
	if err := p.Clone(ctx, src, dest, gitprovider.WithRemoveExisting(remove)); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if len(removed) != 1 || removed[0] != dest {
		t.Fatalf("remover called with %v, want [%s]", removed, dest)
	}
	if _, err := os.Stat(filepath.Join(aside, "tool-only")); err != nil {
		t.Fatalf("the Go Core removed the old workspace itself: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "hello.txt")); err != nil {
		t.Fatalf("not cloned: %v", err)
	}

	// The same remote is updated in place, nothing is removed.
	if err := p.Clone(ctx, src, dest, gitprovider.WithRemoveExisting(remove)); err != nil {
		t.Fatalf("second Clone: %v", err)
	}
	if len(removed) != 1 {
		t.Fatalf("an up-to-date clone was removed: %v", removed)
	}
}
