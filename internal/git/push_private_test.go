package git_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/proctemp"
)

// S10-D review: PushBranch read the workspace's .git/config when it pushed,
// so keys that change what a push sends (push.pushOption: GitLab's
// merge_request.create, ci.variable, ...; push.followTags: the agent's
// annotated tags) or where it goes (url.<x>.insteadOf written by a
// concurrent run after OpenRepo's check) reached the remote. The push now
// runs from a private repository of the Go Core that borrows the
// workspace's objects: no workspace config takes part.

// recordPushOptions makes the bare repository advertise push options and
// record what its pre-receive hook receives; it returns the record file.
func recordPushOptions(t *testing.T, bare string) string {
	t.Helper()
	plainGit(t, bare, "config", "receive.advertisePushOptions", "true")
	record := filepath.Join(t.TempDir(), "push-options")
	writeFile(t, filepath.Join(bare, "hooks", "pre-receive"),
		"#!/bin/sh\necho \"count=${GIT_PUSH_OPTION_COUNT:-none} ${GIT_PUSH_OPTION_0:-}\" >> "+record+"\n", 0o755)
	return record
}

func readRecord(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test file in a temp dir
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(data)
}

func TestPushBranch_SendsNoPushOptions(t *testing.T) {
	ctx := context.Background()
	remotes := sshRemotes(t)
	dir, tip := pushWorkspace(t, remotes)
	victim := filepath.Join(remotes, "victim.git")
	record := recordPushOptions(t, victim)

	// The hook sees push options a client sends.
	plainGit(t, dir, "push", "-q", "-o", "ci.skip", "origin", "main:refs/heads/probe")
	if got := readRecord(t, record); !strings.Contains(got, "count=1 ci.skip") {
		t.Fatalf("setup: the hook did not see the probe's push option: %q", got)
	}
	if err := os.Remove(record); err != nil {
		t.Fatal(err)
	}

	t.Run("push.pushOption is refused for network operations", func(t *testing.T) {
		ws := filepath.Join(t.TempDir(), "ws")
		plainGit(t, filepath.Dir(ws), "clone", "-q", dir, ws)
		plainGit(t, ws, "config", "push.pushOption", "merge_request.create")
		repo, err := git.OpenRepo(ctx, ws)
		if err != nil {
			t.Fatalf("OpenRepo: %v", err)
		}
		if err := repo.PushBranch(ctx, victimURL, "codeforge/abc"); !errors.Is(err, git.ErrUnsafeRepository) {
			t.Fatalf("PushBranch = %v, want ErrUnsafeRepository", err)
		}
	})

	t.Run("push.pushOption written after OpenRepo", func(t *testing.T) {
		repo, err := git.OpenRepo(ctx, dir)
		if err != nil {
			t.Fatalf("OpenRepo: %v", err)
		}
		plainGit(t, dir, "config", "--add", "push.pushOption", "merge_request.create")
		plainGit(t, dir, "config", "--add", "push.pushOption", "ci.variable=EVIL=1")
		if err := repo.PushBranch(ctx, victimURL, "codeforge/abc"); err != nil {
			t.Fatalf("PushBranch: %v", err)
		}
		if got := refAt(t, victim, "refs/heads/codeforge/abc"); got != tip {
			t.Fatalf("victim codeforge/abc = %q, want %s", got, tip)
		}
		// git announces an empty list (count=0) or none at all.
		if got := strings.TrimSpace(readRecord(t, record)); got != "count=0" && got != "count=none" {
			t.Fatalf("the remote's pre-receive hook saw push options (or did not run): %q", got)
		}
	})
}

func TestPushBranch_PushesNoTags(t *testing.T) {
	ctx := context.Background()
	remotes := sshRemotes(t)
	dir, tip := pushWorkspace(t, remotes)
	victim := filepath.Join(remotes, "victim.git")
	plainGit(t, dir, "tag", "-a", "-m", "agent tag", "v9.9.9", "codeforge/abc")
	plainGit(t, dir, "tag", "-a", "-m", "agent tag on main", "v0.0.1", "main")
	plainGit(t, dir, "config", "push.followTags", "true")

	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	if err := repo.PushBranch(ctx, victimURL, "codeforge/abc"); err != nil {
		t.Fatalf("PushBranch: %v", err)
	}
	if got := refAt(t, victim, "refs/heads/codeforge/abc"); got != tip {
		t.Fatalf("victim codeforge/abc = %q, want %s", got, tip)
	}
	for _, tag := range []string{"refs/tags/v9.9.9", "refs/tags/v0.0.1"} {
		if got := refAt(t, victim, tag); got != "" {
			t.Fatalf("the agent's tag %s reached the remote: %s", tag, got)
		}
	}
}

// A concurrent run can rewrite the workspace config after OpenRepo's check;
// git applies url.<x>.insteadOf and pushInsteadOf even to a URL on the
// command line.
func TestPushBranch_IgnoresConfigWrittenAfterOpenRepo(t *testing.T) {
	ctx := context.Background()
	remotes := sshRemotes(t)
	dir, tip := pushWorkspace(t, remotes)
	victim := filepath.Join(remotes, "victim.git")
	other := filepath.Join(remotes, "other.git")

	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	plainGit(t, dir, "config", "url."+otherURL+".insteadOf", victimURL)
	plainGit(t, dir, "config", "url."+otherURL+".pushInsteadOf", victimURL)
	plainGit(t, dir, "config", "remote.origin.push", "+refs/heads/codeforge/abc:refs/heads/main")

	if err := repo.PushBranch(ctx, victimURL, "codeforge/abc"); err != nil {
		t.Fatalf("PushBranch: %v", err)
	}
	if got := refAt(t, victim, "refs/heads/codeforge/abc"); got != tip {
		t.Fatalf("victim codeforge/abc = %q, want %s", got, tip)
	}
	if got := refAt(t, other, "refs/heads/codeforge/abc"); got != "" {
		t.Fatalf("the push was redirected to the other repository: %s", got)
	}
}

// The private repository is removed after the push, whatever its outcome.
func TestPushBranch_RemovesItsPrivateRepository(t *testing.T) {
	ctx := context.Background()
	remotes := sshRemotes(t)
	dir, _ := pushWorkspace(t, remotes)
	tmp, err := proctemp.Dir()
	if err != nil {
		t.Fatal(err)
	}
	leftovers := func() []string {
		matches, err := filepath.Glob(filepath.Join(tmp, "git-push-*"))
		if err != nil {
			t.Fatal(err)
		}
		return matches
	}
	before := len(leftovers())

	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	if err := repo.PushBranch(ctx, victimURL, "codeforge/abc"); err != nil {
		t.Fatalf("PushBranch: %v", err)
	}
	if err := repo.PushBranch(ctx, "ssh://example.invalid/missing", "codeforge/abc"); err == nil {
		t.Fatal("push to a missing repository succeeded")
	}
	if got := leftovers(); len(got) != before {
		t.Fatalf("private push repositories left behind: %v", got)
	}
}
