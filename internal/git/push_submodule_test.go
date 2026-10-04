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

// S3-F security review S2: with push.recurseSubmodules=on-demand (the push
// section was allowed whole), a push first runs `git push` inside a nested
// repository whose gitlink changed - and that repository's config, never
// checked by OpenRepo, runs its core.sshCommand in the Go Core.

// sshToLocal makes ssh remotes reachable without a network: the operator's
// GIT_SSH_COMMAND (kept by workspace git) runs the receive-pack of a local
// bare repository named after the remote path, and logs every connection.
func sshToLocal(t *testing.T) (bareDir, log string) {
	t.Helper()
	bareDir = t.TempDir()
	log = filepath.Join(t.TempDir(), "ssh.log")
	program := filepath.Join(t.TempDir(), "fake-ssh")
	writeFile(t, program, "#!/bin/sh\necho \"$*\" >> "+log+"\nshift\ncase \"$*\" in\n"+
		"  *git-receive-pack*outer*) exec git-receive-pack "+filepath.Join(bareDir, "outer.git")+" ;;\n"+
		"  *git-receive-pack*sub*) exec git-receive-pack "+filepath.Join(bareDir, "sub.git")+" ;;\n"+
		"esac\nexit 1\n", 0o755)
	t.Setenv("GIT_SSH_COMMAND", program)
	t.Setenv("GIT_SSH_VARIANT", "simple")
	plainGit(t, bareDir, "init", "-q", "--bare", "outer.git")
	plainGit(t, bareDir, "init", "-q", "--bare", "sub.git")
	return bareDir, log
}

// plantSubmodule makes the last commit of the workspace dir move the gitlink
// of a nested repository sub/ to a commit its remote does not have. A push
// in sub/ would run its pre-push hook, which writes marker.
func plantSubmodule(t *testing.T, dir string) (marker string) {
	t.Helper()
	sub := filepath.Join(dir, "sub")
	plainGit(t, dir, "init", "-q", "-b", "main", "sub")
	writeFile(t, filepath.Join(sub, "s.txt"), "s\n", 0o644)
	plainGit(t, sub, "add", "-A")
	plainGit(t, sub, "commit", "-q", "-m", "sub")
	writeFile(t, filepath.Join(dir, ".gitmodules"), "[submodule \"sub\"]\n\tpath = sub\n\turl = ssh://example.invalid/sub\n", 0o644)
	plainGit(t, dir, "add", ".gitmodules", "sub")
	plainGit(t, dir, "commit", "-q", "-m", "add sub")

	marker = filepath.Join(t.TempDir(), "marker")
	plainGit(t, sub, "config", "remote.origin.url", "ssh://example.invalid/sub")
	// git pushes only submodules that track a remote.
	plainGit(t, sub, "update-ref", "refs/remotes/origin/main", "HEAD")
	writeFile(t, filepath.Join(sub, ".git", "hooks", "pre-push"), "#!/bin/sh\ntouch "+marker+"\n", 0o755)
	writeFile(t, filepath.Join(sub, "s.txt"), "s2\n", 0o644)
	plainGit(t, sub, "commit", "-q", "-am", "sub change")
	plainGit(t, dir, "add", "sub")
	plainGit(t, dir, "commit", "-q", "-m", "move gitlink")

	plainGit(t, dir, "config", "remote.origin.url", "ssh://example.invalid/outer")
	return marker
}

func assertNoConnection(t *testing.T, log, path string) {
	t.Helper()
	data, _ := os.ReadFile(log) //nolint:gosec // test log
	if strings.Contains(string(data), path) {
		t.Fatalf("a push connected to %s: %s", path, data)
	}
}

// OpenRepo refuses a workspace with a nested repository; a handle opened
// before one appeared still never pushes into it.
func TestPush_NeverRecursesIntoNestedRepositories(t *testing.T) {
	ctx := context.Background()
	for _, value := range []string{"on-demand", "only"} {
		t.Run("push.recurseSubmodules="+value, func(t *testing.T) {
			bareDir, log := sshToLocal(t)
			dir := newRepo(t)
			repo, err := git.OpenRepo(ctx, dir)
			if err != nil {
				t.Fatalf("OpenRepo: %v", err)
			}
			marker := plantSubmodule(t, dir)
			plainGit(t, dir, "config", "push.recurseSubmodules", value)
			assertNestedRefused(t, dir, "sub")
			if err := repo.Push(ctx, "--no-verify", "-u", "origin", "main"); err != nil {
				t.Fatalf("Push: %v", err)
			}
			assertNotRun(t, marker)
			assertNoConnection(t, log, "/sub")
			if got := plainGit(t, filepath.Join(bareDir, "outer.git"), "rev-parse", "main"); got != plainGit(t, dir, "rev-parse", "HEAD") {
				t.Fatalf("outer remote main = %s, want the pushed HEAD", got)
			}
			// A plain push gets the command-line override too.
			plainGit(t, dir, "commit", "-q", "--allow-empty", "-m", "again")
			if _, err := repo.Run(ctx, nil, "push", "origin", "main"); err != nil {
				t.Fatalf("push: %v", err)
			}
			assertNotRun(t, marker)
			assertNoConnection(t, log, "/sub")
		})
	}
}

func TestPush_RefusedInNetworkUnsafeRepository(t *testing.T) {
	dir := newRepo(t)
	plainGit(t, dir, "config", "url.ext::sh.insteadOf", "https://")
	repo, err := git.OpenRepo(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Push(context.Background(), "origin", "main"); !errors.Is(err, git.ErrUnsafeRepository) {
		t.Fatalf("Push = %v, want ErrUnsafeRepository", err)
	}
}

// Sections of commands the Go Core runs are allowed key by key.
func TestOpenRepo_PushFetchPullCheckoutKeysOneByOne(t *testing.T) {
	ctx := context.Background()
	for _, kv := range [][2]string{
		{"push.default", "simple"},
		{"push.autoSetupRemote", "true"},
		{"push.followTags", "true"},
		{"push.recurseSubmodules", "check"},
		{"fetch.prune", "true"},
		{"fetch.fsckObjects", "true"},
		{"pull.rebase", "true"},
		{"checkout.defaultRemote", "origin"},
	} {
		dir := newRepo(t)
		plainGit(t, dir, "config", kv[0], kv[1])
		if _, err := git.OpenRepo(ctx, dir); err != nil {
			t.Errorf("OpenRepo with %s: %v", kv[0], err)
		}
	}
	for _, key := range []string{"push.somethingNew", "fetch.somethingNew", "pull.somethingNew", "checkout.somethingNew"} {
		dir := newRepo(t)
		plainGit(t, dir, "config", key, "x")
		if _, err := git.OpenRepo(ctx, dir); !errors.Is(err, git.ErrUnsafeRepository) || !strings.Contains(err.Error(), strings.ToLower(key)) {
			t.Errorf("OpenRepo with %s = %v, want it refused and named", key, err)
		}
	}
}
