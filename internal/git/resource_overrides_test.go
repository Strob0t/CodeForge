package git_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/git"
)

// KI-187 (R6-6): the repository config is agent-writable data, but some of
// its keys size the processes of the Go Core (worker processes, threads,
// memory windows). Every git process of the Go Core overrides them on the
// command line, so repositories that set them keep working without sizing
// anything.

var sizingOverrides = map[string]string{
	"checkout.workers":         "1",
	"index.threads":            "1",
	"pack.threads":             "1",
	"pack.window":              "10",
	"pack.depth":               "50",
	"pack.windowmemory":        "256m",
	"pack.deltacachesize":      "256m",
	"core.packedgitlimit":      "256m",
	"core.packedgitwindowsize": "32m",
	"core.deltabasecachesize":  "96m",
	"core.bigfilethreshold":    "512m",
}

func TestResourceSizingKeysAreOverridden(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	for key := range sizingOverrides {
		plainGit(t, dir, "config", key, "300000")
	}
	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	for key, want := range sizingOverrides {
		got, err := repo.Run(ctx, nil, "config", "--get", key)
		if err != nil || strings.TrimSpace(got) != want {
			t.Errorf("workspace git: %s = %q (%v), want %q", key, strings.TrimSpace(got), err, want)
		}
		got, err = git.Run(ctx, "", "config", "--get", key)
		if err != nil || strings.TrimSpace(got) != want {
			t.Errorf("git outside a workspace: %s = %q (%v), want %q", key, strings.TrimSpace(got), err, want)
		}
	}
}

// The reviewer's reproduction: with checkout.workers=300 one rewind
// (read-tree -u --reset) of 300 files started 300 checkout workers.
func TestCheckoutWorkersStartNoExtraProcesses(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	for i := range 300 {
		writeFile(t, filepath.Join(dir, "files", fmt.Sprintf("f%03d.txt", i)), fmt.Sprintf("%d\n", i), 0o644)
	}
	plainGit(t, dir, "add", "-A")
	plainGit(t, dir, "commit", "-q", "-m", "files")
	plainGit(t, dir, "config", "checkout.workers", "300")
	plainGit(t, dir, "config", "checkout.thresholdForParallelism", "1")
	if err := os.RemoveAll(filepath.Join(dir, "files")); err != nil {
		t.Fatal(err)
	}

	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	trace := filepath.Join(t.TempDir(), "trace")
	if _, err := repo.Run(ctx, []string{"GIT_TRACE=" + trace}, "read-tree", "-u", "--reset", "HEAD"); err != nil {
		t.Fatalf("read-tree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "files", "f299.txt")); err != nil {
		t.Fatalf("read-tree restored nothing: %v", err)
	}
	data, err := os.ReadFile(trace) //nolint:gosec // test trace
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), "checkout--worker"); n > 0 {
		t.Fatalf("read-tree started %d checkout workers, want none", n)
	}
}
