package svn

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// KI-189 (R6-15): a working copy of another URL is removed by the caller's
// remover (with tool ACLs required, the worker as the tenant's tool UID),
// never by the Go Core when one is given.
func TestSVN_RecloneThroughTheCallersRemover(t *testing.T) {
	skipIfNoSVN(t)
	ctx := context.Background()
	repoURL := initTestSVNRepo(t)
	p := newLocalRepoProvider()
	wc := filepath.Join(t.TempDir(), "wc")
	if err := p.Clone(ctx, repoURL+"/trunk", wc); err != nil {
		t.Fatalf("Clone trunk: %v", err)
	}

	var removed []string
	aside := wc + ".discarded"
	remove := func(_ context.Context, dir string) error {
		removed = append(removed, dir)
		return os.Rename(dir, aside)
	}
	if err := p.Clone(ctx, repoURL+"/branches/feature-x", wc, gitprovider.WithRemoveExisting(remove)); err != nil {
		t.Fatalf("Clone of another URL: %v", err)
	}
	if len(removed) != 1 || removed[0] != wc {
		t.Fatalf("remover called with %v, want [%s]", removed, wc)
	}
	if _, err := os.Stat(filepath.Join(aside, ".svn")); err != nil {
		t.Fatalf("the Go Core removed the old working copy itself: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wc, ".svn")); err != nil {
		t.Fatalf("not checked out: %v", err)
	}
}

// S10-D review: only a directory that is no working copy, or a working copy
// of another URL, is discarded. One whose URL could not be read (svn info
// failed: a broken wc.db, a deadline) keeps its uncommitted work and the
// error is returned.
func TestSVN_KeepsAWorkingCopyItCouldNotCheck(t *testing.T) {
	skipIfNoSVN(t)
	ctx := context.Background()
	repoURL := initTestSVNRepo(t)
	p := newLocalRepoProvider()
	wc := filepath.Join(t.TempDir(), "wc")
	if err := p.Clone(ctx, repoURL+"/trunk", wc); err != nil {
		t.Fatalf("Clone trunk: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wc, "uncommitted.txt"), []byte("work"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wc, ".svn", "wc.db"), []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}

	var removed []string
	remove := func(_ context.Context, dir string) error {
		removed = append(removed, dir)
		return os.RemoveAll(dir)
	}
	if err := p.Clone(ctx, repoURL+"/trunk", wc, gitprovider.WithRemoveExisting(remove)); err == nil {
		t.Fatal("Clone over a working copy svn cannot read succeeded")
	}
	if len(removed) != 0 {
		t.Fatalf("a working copy that could not be checked was removed: %v", removed)
	}
	if _, err := os.Stat(filepath.Join(wc, "uncommitted.txt")); err != nil {
		t.Fatalf("uncommitted work lost: %v", err)
	}
}

func TestSVN_ReclonesADirectoryThatIsNoWorkingCopy(t *testing.T) {
	skipIfNoSVN(t)
	ctx := context.Background()
	repoURL := initTestSVNRepo(t)
	p := newLocalRepoProvider()
	wc := filepath.Join(t.TempDir(), "wc")
	if err := os.MkdirAll(filepath.Join(wc, "leftover"), 0o750); err != nil {
		t.Fatal(err)
	}
	var removed []string
	remove := func(_ context.Context, dir string) error {
		removed = append(removed, dir)
		return os.RemoveAll(dir)
	}
	if err := p.Clone(ctx, repoURL+"/trunk", wc, gitprovider.WithRemoveExisting(remove)); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if len(removed) != 1 {
		t.Fatalf("remover called with %v, want the directory", removed)
	}
}
