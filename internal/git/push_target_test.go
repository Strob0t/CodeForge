package git_test

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/git"
)

// KI-188 (R6-2): delivery pushed with `git push -u origin <branch>`, which
// follows the agent-writable remote.origin.url, pushurl and push refspecs:
// the agent could force-push the run's branch over the remote main, or push
// to any repository the operator's credentials reach. PushBranch pushes to
// the URL the Go Core names (the project's repository URL) with a full
// refspec, so no remote config takes part.

// sshRemotes serves the bare repositories <dir>/<name>.git as
// ssh://example.invalid/<name> through the operator's GIT_SSH_COMMAND
// (kept by workspace git): remotes the hardened git may push to.
func sshRemotes(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	program := filepath.Join(t.TempDir(), "fake-ssh")
	writeFile(t, program, "#!/bin/sh\nshift\ncmd=$(echo \"$*\" | tr -d \"'\")\nprog=${cmd%% *}\nrepo=${cmd#* }\n"+
		"exec \"$prog\" \""+dir+"/${repo#/}.git\"\n", 0o755)
	t.Setenv("GIT_SSH_COMMAND", program)
	t.Setenv("GIT_SSH_VARIANT", "simple")
	for _, name := range []string{"victim", "other"} {
		plainGit(t, dir, "init", "-q", "--bare", "-b", "main", name+".git")
	}
	return dir
}

const (
	victimURL = "ssh://example.invalid/victim"
	otherURL  = "ssh://example.invalid/other"
)

// refAt returns the commit of ref in the bare repository, "" if it has none.
func refAt(t *testing.T, bare, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--verify", "-q", ref)
	cmd.Dir = bare
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

// pushWorkspace is a workspace whose origin is the victim, with main pushed
// there and a branch codeforge/abc one commit ahead of main.
func pushWorkspace(t *testing.T, remotes string) (dir, branchTip string) {
	t.Helper()
	dir = newRepo(t)
	plainGit(t, dir, "remote", "add", "origin", victimURL)
	plainGit(t, dir, "push", "-q", "origin", "main")
	plainGit(t, dir, "checkout", "-q", "-b", "codeforge/abc")
	writeFile(t, filepath.Join(dir, "b.txt"), "b\n", 0o644)
	plainGit(t, dir, "add", "-A")
	plainGit(t, dir, "commit", "-q", "-m", "run change")
	if refAt(t, filepath.Join(remotes, "victim.git"), "refs/heads/main") == "" {
		t.Fatal("setup: main not pushed")
	}
	return dir, plainGit(t, dir, "rev-parse", "HEAD")
}

func TestPushBranch_IgnoresTheAgentsRemoteConfig(t *testing.T) {
	tests := []struct {
		name   string
		config [][2]string
	}{
		{"push refspec onto main (the reviewer's reproduction)", [][2]string{{"remote.origin.push", "+refs/heads/codeforge/abc:refs/heads/main"}}},
		{"pushurl to another repository", [][2]string{{"remote.origin.pushurl", otherURL}}},
		{"origin moved to another repository", [][2]string{{"remote.origin.url", otherURL}}},
		{"push default and upstream", [][2]string{{"push.default", "upstream"}, {"branch.codeforge/abc.merge", "refs/heads/main"}, {"branch.codeforge/abc.remote", "origin"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			remotes := sshRemotes(t)
			dir, tip := pushWorkspace(t, remotes)
			victim := filepath.Join(remotes, "victim.git")
			mainBefore := refAt(t, victim, "refs/heads/main")
			for _, kv := range tt.config {
				if kv[0] == "remote.origin.url" {
					plainGit(t, dir, "config", kv[0], kv[1])
					continue
				}
				plainGit(t, dir, "config", "--add", kv[0], kv[1])
			}

			repo, err := git.OpenRepo(ctx, dir)
			if err != nil {
				t.Fatalf("OpenRepo: %v", err)
			}
			if err := repo.PushBranch(ctx, victimURL, "codeforge/abc"); err != nil {
				t.Fatalf("PushBranch: %v", err)
			}
			if got := refAt(t, victim, "refs/heads/codeforge/abc"); got != tip {
				t.Fatalf("victim codeforge/abc = %q, want the run's commit %s", got, tip)
			}
			if got := refAt(t, victim, "refs/heads/main"); got != mainBefore {
				t.Fatalf("victim main moved from %s to %s", mainBefore, got)
			}
			other := filepath.Join(remotes, "other.git")
			for _, ref := range []string{"refs/heads/codeforge/abc", "refs/heads/main"} {
				if got := refAt(t, other, ref); got != "" {
					t.Fatalf("the other repository got %s = %s", ref, got)
				}
			}
		})
	}
}

// git resolves a URL argument through a remote named like it, so a remote
// "<url>" in the config would redirect the push and the fetch: refused.
func TestPushBranchAndFetchFrom_RefuseARemoteNamedLikeTheURL(t *testing.T) {
	ctx := context.Background()
	remotes := sshRemotes(t)
	dir, _ := pushWorkspace(t, remotes)
	plainGit(t, dir, "config", "remote."+victimURL+".pushurl", otherURL)
	plainGit(t, dir, "config", "remote."+victimURL+".url", otherURL)
	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	if err := repo.PushBranch(ctx, victimURL, "codeforge/abc"); !errors.Is(err, git.ErrUnsafeRepository) {
		t.Fatalf("PushBranch = %v, want ErrUnsafeRepository", err)
	}
	if err := repo.FetchFrom(ctx, victimURL); !errors.Is(err, git.ErrUnsafeRepository) {
		t.Fatalf("FetchFrom = %v, want ErrUnsafeRepository", err)
	}
	if got := refAt(t, filepath.Join(remotes, "other.git"), "refs/heads/codeforge/abc"); got != "" {
		t.Fatalf("the other repository got the branch: %s", got)
	}
}

// The refspec has no "+": a branch that moved on the remote is never
// overwritten.
func TestPushBranch_NeverForces(t *testing.T) {
	ctx := context.Background()
	remotes := sshRemotes(t)
	dir, _ := pushWorkspace(t, remotes)
	victim := filepath.Join(remotes, "victim.git")
	// The remote branch has a commit the run's branch does not contain.
	plainGit(t, dir, "checkout", "-q", "-b", "side", "main")
	writeFile(t, filepath.Join(dir, "c.txt"), "c\n", 0o644)
	plainGit(t, dir, "add", "-A")
	plainGit(t, dir, "commit", "-q", "-m", "remote side")
	plainGit(t, dir, "push", "-q", "origin", "side:refs/heads/codeforge/abc")
	plainGit(t, dir, "checkout", "-q", "codeforge/abc")
	remoteTip := refAt(t, victim, "refs/heads/codeforge/abc")

	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.PushBranch(ctx, victimURL, "codeforge/abc"); err == nil {
		t.Fatal("PushBranch overwrote a diverged remote branch")
	}
	if got := refAt(t, victim, "refs/heads/codeforge/abc"); got != remoteTip {
		t.Fatalf("remote branch moved from %s to %s", remoteTip, got)
	}
}

func TestPushBranch_RefusesInvalidArguments(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ url, branch string }{
		{"", "main"},
		{"-oProxyCommand=evil", "main"},
		{victimURL, ""},
		{victimURL, "-f"},
		{victimURL, "a:b"},
		{victimURL, "a..b"},
		{victimURL, "HEAD"},
	} {
		if err := repo.PushBranch(ctx, tt.url, tt.branch); err == nil {
			t.Errorf("PushBranch(%q, %q) accepted", tt.url, tt.branch)
		}
	}
}
