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
