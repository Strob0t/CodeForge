package git_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/git"
)

// KI-77: the Go Core runs git in agent-writable workspaces. These tests plant
// the attacks an agent can write into .git and check that no planted program
// ever runs and no repository outside the workspace is touched.

// plainGit runs git for test setup, outside the hardened path.
func plainGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil { //nolint:gosec // test files in a temp dir
		t.Fatal(err)
	}
}

// newRepo creates a repository with one commit and returns its directory.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ws")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	plainGit(t, dir, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(dir, "a.txt"), "a\n", 0o644)
	plainGit(t, dir, "add", "-A")
	plainGit(t, dir, "commit", "-q", "-m", "init")
	return dir
}

// evilProgram writes a program that records its runs in a marker file and
// returns the program path and the marker path.
func evilProgram(t *testing.T) (program, marker string) {
	t.Helper()
	dir := t.TempDir()
	marker = filepath.Join(dir, "marker")
	program = filepath.Join(dir, "evil.sh")
	writeFile(t, program, "#!/bin/sh\necho \"$0 $*\" >> "+marker+"\ncat\n", 0o755)
	return program, marker
}

func assertNotRun(t *testing.T, marker string) {
	t.Helper()
	if data, err := os.ReadFile(marker); err == nil { //nolint:gosec // test marker
		t.Fatalf("a planted program ran in the Go Core: %s", data)
	}
}

// plantEverything configures every code-running hook a local operation could
// reach: fsmonitor, a filter driver with attributes, hooks.
func plantEverything(t *testing.T, dir, program string) {
	t.Helper()
	plainGit(t, dir, "config", "core.fsmonitor", program)
	plainGit(t, dir, "config", "filter.x.clean", program+" clean")
	plainGit(t, dir, "config", "filter.x.smudge", program+" smudge")
	plainGit(t, dir, "config", "filter.x.process", program+" process")
	plainGit(t, dir, "config", "filter.x.required", "true")
	plainGit(t, dir, "config", "credential.helper", program)
	writeFile(t, filepath.Join(dir, ".gitattributes"), "* filter=x diff=x merge=x\n", 0o644)
	for _, hook := range []string{"reference-transaction", "pre-commit", "post-commit", "post-checkout", "post-index-change", "pre-push"} {
		writeFile(t, filepath.Join(dir, ".git", "hooks", hook), "#!/bin/sh\n"+program+" hook-"+hook+"\n", 0o755)
	}
	writeFile(t, filepath.Join(dir, "a.txt"), "changed\n", 0o644)
	writeFile(t, filepath.Join(dir, "new.txt"), "new\n", 0o644)
}

func TestRepo_PlantedProgramsNeverRun(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	program, marker := evilProgram(t)
	plantEverything(t, dir, program)
	// The Go Core's own environment and global config are not the agent's, but
	// workspace git must not depend on them either.
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".gitconfig"), "[core]\n\tfsmonitor = "+program+"\n", 0o644)
	t.Setenv("HOME", home)
	t.Setenv("GIT_EXTERNAL_DIFF", program)
	t.Setenv("GIT_ASKPASS", program)
	t.Setenv("SSH_ASKPASS", program)

	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	index := []string{"GIT_INDEX_FILE=" + filepath.Join(t.TempDir(), "index")}
	steps := [][]string{
		{"status", "--porcelain"},
		{"add", "-A"},
		{"diff", "--cached", "--binary", "HEAD"},
		{"write-tree"},
		{"update-ref", "refs/codeforge/test", "HEAD"},
		{"update-ref", "-d", "refs/codeforge/test"},
		{"commit", "-q", "-m", "delivery"},
		{"checkout", "-q", "-b", "other"},
		{"log", "-1", "-p"},
	}
	for _, args := range steps {
		env := index
		if args[0] == "commit" || args[0] == "checkout" || args[0] == "log" || args[0] == "status" {
			env = []string{"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid"}
		}
		if args[0] == "commit" {
			if _, err := repo.Run(ctx, nil, "add", "-A"); err != nil {
				t.Fatalf("add: %v", err)
			}
		}
		if _, err := repo.Run(ctx, env, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		assertNotRun(t, marker)
	}
	if _, err := repo.Run(ctx, index, "read-tree", "-u", "--reset", "HEAD~1"); err != nil {
		t.Fatalf("read-tree -u: %v", err)
	}
	assertNotRun(t, marker)

	fill := repo.Command(ctx, "git", "credential", "fill")
	fill.Stdin = strings.NewReader("protocol=https\nhost=example.invalid\n\n")
	_ = fill.Run() // no helper, no prompt: it fails, and nothing ran
	assertNotRun(t, marker)
}

func TestOpenRepo_LFSConfigKeepsWorking(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	// git lfs install --local without git-lfs on the PATH of the Go Core.
	plainGit(t, dir, "config", "filter.lfs.clean", "git-lfs clean -- %f")
	plainGit(t, dir, "config", "filter.lfs.smudge", "git-lfs smudge -- %f")
	plainGit(t, dir, "config", "filter.lfs.process", "git-lfs filter-process")
	plainGit(t, dir, "config", "filter.lfs.required", "true")
	plainGit(t, dir, "config", "lfs.repositoryformatversion", "0")
	writeFile(t, filepath.Join(dir, ".gitattributes"), "*.bin filter=lfs diff=lfs merge=lfs -text\n", 0o644)
	writeFile(t, filepath.Join(dir, "big.bin"), "binary content\n", 0o644)

	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	index := []string{"GIT_INDEX_FILE=" + filepath.Join(t.TempDir(), "index")}
	if _, err := repo.Run(ctx, index, "add", "-A"); err != nil {
		t.Fatalf("add -A with an LFS filter the Go Core cannot run: %v", err)
	}
	tree, err := repo.Run(ctx, index, "write-tree")
	if err != nil {
		t.Fatalf("write-tree: %v", err)
	}
	if got := plainGit(t, dir, "cat-file", "-p", strings.TrimSpace(tree)+":big.bin"); got != "binary content" {
		t.Fatalf("big.bin stored as %q, want its plain content", got)
	}
}

func TestOpenRepo_RefusesUnsafeRepositories(t *testing.T) {
	ctx := context.Background()
	program, marker := evilProgram(t)
	other := newRepo(t) // another tenant's repository
	tests := []struct {
		name  string
		plant func(t *testing.T, dir string)
	}{
		{"include.path", func(t *testing.T, dir string) {
			included := filepath.Join(t.TempDir(), "included")
			writeFile(t, included, "[core]\n\tfsmonitor = "+program+"\n", 0o644)
			plainGit(t, dir, "config", "include.path", included)
		}},
		{"includeIf", func(t *testing.T, dir string) {
			plainGit(t, dir, "config", "includeIf.gitdir:/.path", filepath.Join(t.TempDir(), "x"))
		}},
		{"core.worktree outside", func(t *testing.T, dir string) { plainGit(t, dir, "config", "core.worktree", other) }},
		{"diff driver command", func(t *testing.T, dir string) { plainGit(t, dir, "config", "diff.x.command", program) }},
		{"diff textconv", func(t *testing.T, dir string) { plainGit(t, dir, "config", "diff.x.textconv", program) }},
		{"external diff", func(t *testing.T, dir string) { plainGit(t, dir, "config", "diff.external", program) }},
		{"merge driver", func(t *testing.T, dir string) { plainGit(t, dir, "config", "merge.x.driver", program) }},
		{"gpg program", func(t *testing.T, dir string) { plainGit(t, dir, "config", "gpg.program", program) }},
		{"askpass", func(t *testing.T, dir string) { plainGit(t, dir, "config", "core.askPass", program) }},
		{"worktree config extension", func(t *testing.T, dir string) { plainGit(t, dir, "config", "extensions.worktreeConfig", "true") }},
		{"unknown key", func(t *testing.T, dir string) { plainGit(t, dir, "config", "something.new", "x") }},
		{"bare", func(t *testing.T, dir string) { plainGit(t, dir, "config", "core.bare", "true") }},
		{"commondir", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, ".git", "commondir"), filepath.Join(other, ".git")+"\n", 0o644)
		}},
		{"alternates", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, ".git", "objects", "info", "alternates"), filepath.Join(other, ".git", "objects")+"\n", 0o644)
		}},
		{"symlinked refs", func(t *testing.T, dir string) {
			refs := filepath.Join(dir, ".git", "refs", "heads")
			if err := os.RemoveAll(refs); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(other, ".git", "refs", "heads"), refs); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlinked config", func(t *testing.T, dir string) {
			cfg := filepath.Join(dir, ".git", "config")
			if err := os.Remove(cfg); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(other, ".git", "config"), cfg); err != nil {
				t.Fatal(err)
			}
		}},
		{".git is a gitdir file", func(t *testing.T, dir string) {
			if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(dir, ".git"), "gitdir: "+filepath.Join(other, ".git")+"\n", 0o644)
		}},
		{".git is a symlink", func(t *testing.T, dir string) {
			if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(other, ".git"), filepath.Join(dir, ".git")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := newRepo(t)
			tc.plant(t, dir)
			otherHead := plainGit(t, other, "rev-parse", "HEAD")

			_, err := git.OpenRepo(ctx, dir)
			if !errors.Is(err, git.ErrUnsafeRepository) {
				t.Fatalf("OpenRepo = %v, want ErrUnsafeRepository", err)
			}
			assertNotRun(t, marker)
			if got := plainGit(t, other, "rev-parse", "HEAD"); got != otherHead {
				t.Fatal("the other repository was changed")
			}
		})
	}
}

func TestOpenRepo_NetworkOnlyKeys(t *testing.T) {
	ctx := context.Background()
	// Program-valued transport keys (core.sshCommand, remote.*.uploadpack, ...)
	// are refused in every repository (promisor_test.go).
	for _, kv := range [][2]string{
		{"url.ext::sh.insteadOf", "https://"},
		{"protocol.ext.allow", "always"},
		{"http.proxy", "http://proxy.invalid"},
		{"remote.origin.proxy", "http://proxy.invalid"},
		{"remote.origin.serverOption", "x"},
	} {
		t.Run(kv[0], func(t *testing.T) {
			dir := newRepo(t)
			plainGit(t, dir, "config", kv[0], kv[1])
			repo, err := git.OpenRepo(ctx, dir)
			if err != nil {
				t.Fatalf("OpenRepo refused a key that only matters for network operations: %v", err)
			}
			if err := repo.RequireNetworkSafe(); !errors.Is(err, git.ErrUnsafeRepository) {
				t.Fatalf("RequireNetworkSafe = %v, want ErrUnsafeRepository", err)
			}
		})
	}
	repo, err := git.OpenRepo(ctx, newRepo(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.RequireNetworkSafe(); err != nil {
		t.Fatalf("RequireNetworkSafe on a plain clone: %v", err)
	}
}

func TestOpenRepo_NoRepository(t *testing.T) {
	if _, err := git.OpenRepo(context.Background(), t.TempDir()); !errors.Is(err, git.ErrNotRepository) {
		t.Fatalf("OpenRepo = %v, want ErrNotRepository", err)
	}
}

func TestRepo_PushToLocalPathRefused(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	other := newRepo(t)
	plainGit(t, dir, "remote", "add", "origin", other)
	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Run(ctx, nil, "push", "origin", "main:refs/heads/injected"); err == nil {
		t.Fatal("push to a local path (another workspace) succeeded")
	}
	if refs := plainGit(t, other, "for-each-ref", "refs/heads/injected"); refs != "" {
		t.Fatal("the other repository received a branch")
	}
}
