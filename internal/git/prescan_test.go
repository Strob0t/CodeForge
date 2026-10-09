package git_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/git"
)

// S10-D review: a FIFO .gitignore in an untracked directory was only ended
// by the 2-minute command deadline, while OpenRepo held a slot of the git
// pool all tenants share. OpenRepo now pre-scans the working tree's ignore
// files before git lists it, and its checks end at a shorter deadline of
// their own.

func TestOpenRepo_PreScanRefusesSpecialIgnoreFilesInNewDirectories(t *testing.T) {
	tests := []struct{ name, path string }{
		{"FIFO .gitignore in a new directory", "new/.gitignore"},
		{"FIFO .gitignore deep in a new directory", "new/a/b/c/.gitignore"},
		{"FIFO .gitignore in a new directory beside ignored files", "fresh/.gitignore"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newRepo(t)
			mkfifo(t, filepath.Join(dir, filepath.FromSlash(tt.path)))
			assertSpecialRefused(t, dir, tt.path)
		})
	}
}

// Past the pre-scan's budget, the checks' own deadline ends a git process
// that blocks, long before the command deadline.
func TestOpenRepo_ChecksEndAtTheirOwnDeadline(t *testing.T) {
	setTimeouts(t, time.Hour, time.Hour)
	git.SetCheckTimeout(t, 500*time.Millisecond)
	git.SetPreScanEntries(t, 1)
	dir := newRepo(t)
	mkfifo(t, filepath.Join(dir, "new", ".gitignore"))

	start := time.Now()
	err := openWithin(context.Background(), t, dir)
	if !errors.Is(err, git.ErrGitTimeout) {
		t.Fatalf("OpenRepo = %v, want ErrGitTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("OpenRepo took %v, want about the checks' deadline", elapsed)
	}
}

func TestOpenRepo_RefusesSpecialFilesAmongObjects(t *testing.T) {
	tests := []struct{ name, path string }{
		{"FIFO loose object", ".git/objects/ab/cdef0123456789abcdef0123456789abcdef01"},
		{"FIFO commit-graph chain", ".git/objects/info/commit-graphs/commit-graph-chain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newRepo(t)
			mkfifo(t, filepath.Join(dir, filepath.FromSlash(tt.path)))
			assertSpecialRefused(t, dir, tt.path)
		})
	}
}
