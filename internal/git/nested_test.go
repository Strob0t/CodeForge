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

// Nested repositories (security review of the S3-F fix round): git's
// submodule dirty check runs a git process in a nested repository, with its
// config, for every gitlink of the index `add` or `status` works on. The Go
// Core refuses workspaces with gitlinks or nested .git entries.

// nestedRepoAt creates a repository with one commit at dir/rel.
func nestedRepoAt(t *testing.T, dir, rel string) string {
	t.Helper()
	sub := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	plainGit(t, sub, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(sub, "x.txt"), "x\n", 0o644)
	plainGit(t, sub, "add", "x.txt")
	plainGit(t, sub, "commit", "-q", "-m", "nested")
	return sub
}

func assertNestedRefused(t *testing.T, dir, want string) {
	t.Helper()
	_, err := git.OpenRepo(context.Background(), dir)
	if !errors.Is(err, git.ErrUnsafeRepository) || !strings.Contains(err.Error(), "nested repository at "+want+" ") {
		t.Fatalf("OpenRepo = %v, want a refusal naming the nested repository at %s", err, want)
	}
}

func TestOpenRepo_RefusesNestedRepositories(t *testing.T) {
	t.Run("gitlink in the index", func(t *testing.T) {
		dir := newRepo(t)
		nestedRepoAt(t, dir, "sub")
		plainGit(t, dir, "add", "sub")
		assertNestedRefused(t, dir, "sub")
	})
	t.Run("gitlink only in HEAD", func(t *testing.T) {
		dir := newRepo(t)
		nestedRepoAt(t, dir, "sub")
		plainGit(t, dir, "add", "sub")
		plainGit(t, dir, "commit", "-q", "-m", "gitlink")
		plainGit(t, dir, "rm", "-q", "--cached", "sub")
		if err := os.RemoveAll(filepath.Join(dir, "sub")); err != nil {
			t.Fatal(err)
		}
		assertNestedRefused(t, dir, "sub")
	})
	t.Run("untracked nested .git directory", func(t *testing.T) {
		dir := newRepo(t)
		nestedRepoAt(t, dir, "a/b/sub")
		assertNestedRefused(t, dir, "a/b/sub")
	})
	t.Run("nested .git file", func(t *testing.T) {
		dir := newRepo(t)
		writeFile(t, filepath.Join(dir, "lib", ".git"), "gitdir: /elsewhere/.git\n", 0o644)
		assertNestedRefused(t, dir, "lib")
	})
}

func TestOpenRepo_NestedRepositoriesThatGitNeverEnters(t *testing.T) {
	ctx := context.Background()
	t.Run("inside an ignored directory", func(t *testing.T) {
		dir := newRepo(t)
		writeFile(t, filepath.Join(dir, ".gitignore"), "node_modules/\n", 0o644)
		nestedRepoAt(t, dir, "node_modules/pkg")
		if _, err := git.OpenRepo(ctx, dir); err != nil {
			t.Fatalf("OpenRepo with a repository in an ignored directory: %v", err)
		}
	})
	t.Run("behind a symbolic link", func(t *testing.T) {
		dir := newRepo(t)
		outside := nestedRepoAt(t, t.TempDir(), "other")
		if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
			t.Fatal(err)
		}
		if _, err := git.OpenRepo(ctx, dir); err != nil {
			t.Fatalf("OpenRepo with a link to a repository (not followed): %v", err)
		}
	})
}

// The nested-repository check skips ignored directories. A checkout of a
// tree with a gitlink at such a repository must not run the submodule dirty
// check there (checkout reports local changes through it unless quiet).
func TestRunIn_CheckoutNeverEntersAnIgnoredNestedRepository(t *testing.T) {
	for _, command := range []string{"checkout", "switch"} {
		t.Run(command, func(t *testing.T) {
			dir := newRepo(t)
			plainGit(t, dir, "checkout", "-q", "-b", "gitlink")
			nestedRepoAt(t, dir, "sub")
			writeFile(t, filepath.Join(dir, ".gitmodules"), "[submodule \"sub\"]\n\tpath = sub\n\turl = ./sub\n\tignore = none\n", 0o644)
			plainGit(t, dir, "add", ".gitmodules", "sub")
			plainGit(t, dir, "commit", "-q", "-m", "gitlink")
			plainGit(t, dir, "checkout", "-q", "main")
			writeFile(t, filepath.Join(dir, ".gitignore"), "sub/\n", 0o644)
			plainGit(t, dir, "add", ".gitignore")
			plainGit(t, dir, "commit", "-q", "-m", "ignore sub")

			sub := filepath.Join(dir, "sub")
			marker := filepath.Join(t.TempDir(), "marker")
			writeFile(t, filepath.Join(sub, "clean.sh"), "#!/bin/sh\ntouch "+marker+"\ncat\n", 0o755)
			plainGit(t, sub, "config", "filter.evil.clean", filepath.Join(sub, "clean.sh"))
			writeFile(t, filepath.Join(sub, ".gitattributes"), "* filter=evil\n", 0o644)
			writeFile(t, filepath.Join(sub, "x.txt"), "y\n", 0o644) // same size: only a filtered read tells

			if _, err := git.RunIn(context.Background(), dir, command, "gitlink"); err != nil {
				t.Fatalf("%s: %v", command, err)
			}
			assertNotRun(t, marker)
			assertNestedRefused(t, dir, "sub")
		})
	}
}

// The private index of a checkpoint is checked before every add.
func TestRefuseGitlinks_PrivateIndex(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(t.TempDir(), "index")
	env := []string{"GIT_INDEX_FILE=" + index}
	if err := repo.RefuseGitlinks(ctx, env); err != nil {
		t.Fatalf("empty private index: %v", err)
	}
	head := plainGit(t, dir, "rev-parse", "HEAD")
	if _, err := repo.Run(ctx, env, "update-index", "--add", "--cacheinfo", "160000,"+head+",vendor/lib"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RefuseGitlinks(ctx, env); !errors.Is(err, git.ErrUnsafeRepository) || !strings.Contains(err.Error(), "vendor/lib") {
		t.Fatalf("RefuseGitlinks = %v, want the gitlink vendor/lib refused", err)
	}
}
