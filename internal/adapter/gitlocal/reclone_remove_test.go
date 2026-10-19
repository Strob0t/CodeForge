package gitlocal_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/git"
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

// S10-D review: only a destination that is no repository, or a clone of
// another URL, is discarded. A repository that could not be checked (a git
// deadline, an unreadable .git) keeps its uncommitted work and the error is
// returned.
func TestClone_KeepsARepositoryItCouldNotCheck(t *testing.T) {
	ctx := context.Background()
	src := initTestRepo(t)
	dest := filepath.Join(t.TempDir(), "ws")
	runGitCmd(t, "", "clone", "-q", src, dest)
	if err := os.WriteFile(filepath.Join(dest, "uncommitted.txt"), []byte("work"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := gitprovider.New("local", nil)
	if err != nil {
		t.Fatal(err)
	}
	var removed []string
	remove := func(_ context.Context, dir string) error {
		removed = append(removed, dir)
		return os.RemoveAll(dir)
	}

	git.SetTimeouts(time.Nanosecond, time.Nanosecond) // every git command ends at its deadline
	t.Cleanup(func() { git.SetTimeouts(git.DefaultCommandTimeout, git.DefaultNetworkTimeout) })
	err = p.Clone(ctx, src, dest, gitprovider.WithRemoveExisting(remove))
	if !errors.Is(err, git.ErrGitTimeout) {
		t.Fatalf("Clone = %v, want the git deadline error", err)
	}
	if len(removed) != 0 {
		t.Fatalf("a repository that could not be checked was removed: %v", removed)
	}
	if _, err := os.Stat(filepath.Join(dest, "uncommitted.txt")); err != nil {
		t.Fatalf("uncommitted work lost: %v", err)
	}
}

func TestClone_ReclonesACloneOfAnotherURL(t *testing.T) {
	ctx := context.Background()
	src, other := initTestRepo(t), initTestRepo(t)
	dest := filepath.Join(t.TempDir(), "ws")
	runGitCmd(t, "", "clone", "-q", other, dest)
	p, err := gitprovider.New("local", nil)
	if err != nil {
		t.Fatal(err)
	}
	var removed []string
	remove := func(_ context.Context, dir string) error {
		removed = append(removed, dir)
		return os.RemoveAll(dir)
	}
	if err := p.Clone(ctx, src, dest, gitprovider.WithRemoveExisting(remove)); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if len(removed) != 1 {
		t.Fatalf("remover called with %v, want the clone of the other URL", removed)
	}
}
