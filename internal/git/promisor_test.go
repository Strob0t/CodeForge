package git_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/git"
)

// S3-F security review S1: with remote.<name>.promisor (or a partial clone)
// configured, a local command that needs a missing object (diff --cached,
// merge-tree, cat-file) lazily fetches it - with the repository's
// core.sshCommand - inside the Go Core. Keys that name a program or make a
// remote a promisor are refused in every repository, and lazy fetches are
// disabled (GIT_NO_LAZY_FETCH) for a config rewritten after OpenRepo.

// promisorRepo stages a file, deletes its blob and makes origin a promisor
// remote whose ssh command writes a marker.
func promisorRepo(t *testing.T) (dir, marker string) {
	t.Helper()
	dir = newRepo(t)
	writeFile(t, filepath.Join(dir, "f.txt"), "data\n", 0o644)
	plainGit(t, dir, "add", "f.txt")
	blob := plainGit(t, dir, "rev-parse", ":f.txt")
	// Without the blob and the worktree copy, git must fetch the object.
	if err := os.Remove(filepath.Join(dir, ".git", "objects", blob[:2], blob[2:])); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "f.txt")); err != nil {
		t.Fatal(err)
	}
	marker = filepath.Join(t.TempDir(), "marker")
	plainGit(t, dir, "config", "remote.origin.url", "ssh://example.invalid/x")
	return dir, marker
}

// unsetSSHCommand removes GIT_SSH_COMMAND (the go command sets it for test
// processes, and it would mask the repository's core.sshCommand).
func unsetSSHCommand(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_SSH_COMMAND", "")
	if err := os.Unsetenv("GIT_SSH_COMMAND"); err != nil {
		t.Fatal(err)
	}
}

func plantLazyFetch(t *testing.T, dir, marker string) {
	t.Helper()
	plainGit(t, dir, "config", "remote.origin.promisor", "true")
	plainGit(t, dir, "config", "core.sshCommand", "touch "+marker+"; false")
}

func TestOpenRepo_RefusesProgramAndPromisorKeysInEveryRepository(t *testing.T) {
	ctx := context.Background()
	for _, kv := range [][2]string{
		{"core.sshCommand", "/tmp/evil"},
		{"core.gitProxy", "/tmp/evil"},
		{"remote.origin.uploadpack", "/tmp/evil"},
		{"remote.origin.receivepack", "/tmp/evil"},
		{"remote.origin.vcs", "evil"},
		{"remote.origin.promisor", "true"},
		{"remote.origin.partialCloneFilter", "blob:none"},
		{"extensions.partialClone", "origin"},
	} {
		t.Run(kv[0], func(t *testing.T) {
			dir := newRepo(t)
			plainGit(t, dir, "config", kv[0], kv[1])
			_, err := git.OpenRepo(ctx, dir)
			if !errors.Is(err, git.ErrUnsafeRepository) {
				t.Fatalf("OpenRepo with %s = %v, want ErrUnsafeRepository", kv[0], err)
			}
			if !strings.Contains(err.Error(), strings.ToLower(kv[0])) {
				t.Fatalf("error %q does not name the key %s", err, kv[0])
			}
		})
	}
}

func TestPromisorLazyFetch_NeverRunsTheRepositorysSSHCommand(t *testing.T) {
	ctx := context.Background()
	unsetSSHCommand(t)

	t.Run("configured before OpenRepo", func(t *testing.T) {
		dir, marker := promisorRepo(t)
		plantLazyFetch(t, dir, marker)
		if _, err := git.OpenRepo(ctx, dir); !errors.Is(err, git.ErrUnsafeRepository) {
			t.Fatalf("OpenRepo = %v, want ErrUnsafeRepository", err)
		}
		assertNotRun(t, marker)
	})

	t.Run("written after OpenRepo", func(t *testing.T) {
		dir, marker := promisorRepo(t)
		repo, err := git.OpenRepo(ctx, dir)
		if err != nil {
			t.Fatalf("OpenRepo: %v", err)
		}
		// The agent rewrites the config between OpenRepo's check and git's
		// own read: lazy fetches stay disabled.
		plantLazyFetch(t, dir, marker)
		for _, args := range [][]string{
			{"diff", "--cached", "--stat"},
			{"cat-file", "-p", ":f.txt"},
		} {
			_, _ = repo.Run(ctx, nil, args...)
			assertNotRun(t, marker)
		}
	})
}
