//go:build unix

package service

// S7-B review round: delivery's patch writing never blocks on a FIFO and the
// attributes filter check fails closed.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

// S1: a FIFO swapped in for .git after git.OpenRepo looked at it must not
// block the patch writer (it holds a slot of the shared git pool).
func TestWritePatch_NeverBlocksOnASwappedGitDir(t *testing.T) {
	base := t.TempDir()
	fifo := filepath.Join(base, "git-fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	err := within(t, func() error {
		_, err := writePatch(&git.Repo{Dir: base, GitDir: fifo}, "r1", "diff")
		return err
	})
	if err == nil {
		t.Fatal("writePatch into a FIFO must fail")
	}

	ws, out := symlinkWorkspace(t)
	gitDir := filepath.Join(ws, ".git")
	if err := os.Symlink(out, gitDir); err != nil {
		t.Fatal(err)
	}
	if _, err = writePatch(&git.Repo{Dir: ws, GitDir: gitDir}, "r1", "diff"); !errors.Is(err, workspacefs.ErrLeavesWorkspace) {
		t.Fatalf("writePatch into a symlinked .git = %v", err)
	}
	assertOutsideUnchanged(t, out)

	if err := os.Remove(gitDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(gitDir, 0o750); err != nil {
		t.Fatal(err)
	}
	patch, err := writePatch(&git.Repo{Dir: ws, GitDir: gitDir}, "r1", "diff")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(patch); string(got) != "diff" { //nolint:gosec // test file
		t.Fatalf("patch = %q", got)
	}
}

// C8: a working tree that cannot be opened counts as having filter attributes.
func TestHasFilterAttributes_FailsClosed(t *testing.T) {
	base := t.TempDir()
	realDir := filepath.Join(base, "real")
	if err := os.Mkdir(realDir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "T"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = realDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	repo, err := git.OpenRepo(context.Background(), realDir)
	if err != nil {
		t.Fatal(err)
	}
	if hasFilterAttributes(context.Background(), repo) {
		t.Fatal("a repository without attributes has no filters")
	}
	// The agent replaces the working tree by a symlink after OpenRepo.
	ws := filepath.Join(base, "ws")
	if err := os.Symlink("real", ws); err != nil {
		t.Fatal(err)
	}
	repo.Dir, repo.GitDir = ws, filepath.Join(ws, ".git")
	if !hasFilterAttributes(context.Background(), repo) {
		t.Fatal("a working tree that cannot be opened must count as filtered (renormalize)")
	}
}

// S7-B round 3: when git cannot list the attributes files, the repository
// counts as having filter attributes too.
func TestHasFilterAttributes_FailsClosedWhenListingFails(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "T"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	repo, err := git.OpenRepo(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if hasFilterAttributes(context.Background(), repo) {
		t.Fatal("a repository without attributes has no filters")
	}
	// A corrupt index makes ls-files fail.
	if err := os.WriteFile(filepath.Join(dir, ".git", "index"), []byte("not an index"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Run(context.Background(), nil, "ls-files"); err == nil {
		t.Fatal("ls-files must fail on a corrupt index")
	}
	if !hasFilterAttributes(context.Background(), repo) {
		t.Fatal("a repository whose attributes cannot be listed must count as filtered (renormalize)")
	}
}
