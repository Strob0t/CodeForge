package service_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/service"
)

// Review of S3 (findings 1, 2, 3, 8, 10): the checkpoint chain in the
// workspace repository is the durable record of a run's starting state
// (HEAD, the user's index, the working tree); rollback restores all of it,
// works after a Go Core restart, and patch delivery holds exactly the run's
// change.

// gitTry runs git for test setup and reports whether it succeeded.
func gitTry(dir string, args ...string) (string, bool) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err == nil
}

// initEmptyRepo is `git init` without any commit or index (ProjectService's
// InitWorkspace).
func initEmptyRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main")
	gitRun(t, dir, "config", "user.email", "test@test.com")
	gitRun(t, dir, "config", "user.name", "Test")
	return dir
}

func TestCheckpoint_RepositoryWithoutIndexOrCommits(t *testing.T) {
	ctx := context.Background()
	dir := initEmptyRepo(t)
	writeRepoFile(t, dir, "existing.txt", "before the run\n")
	if _, err := os.Stat(filepath.Join(dir, ".git", "index")); !os.IsNotExist(err) {
		t.Fatal("the test repository must have no index")
	}
	pool := git.NewPool(1)
	cp := service.NewCheckpointService(pool)
	deliverer := service.NewDeliverService(&deliverMockStore{proj: &project.Project{ID: "proj-1", WorkspacePath: dir}},
		&config.Runtime{}, pool)

	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Write", "call-1"); err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}
	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Write", "call-2"); err != nil {
		t.Fatalf("second CreateCheckpoint: %v", err)
	}
	writeRepoFile(t, dir, "new.txt", "from the run\n")
	result, err := deliverer.Deliver(ctx, &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: run.DeliverModePatch}, "task")
	if err != nil {
		t.Fatalf("patch delivery: %v", err)
	}
	patch := readFileAt(t, result.PatchPath)
	if !strings.Contains(patch, "new.txt") || strings.Contains(patch, "existing.txt") {
		t.Fatalf("patch = %q, want only new.txt", patch)
	}
	if err := cp.RewindToFirst(ctx, checkpointRunID, dir); err != nil {
		t.Fatalf("RewindToFirst: %v", err)
	}
	if repoFileExists(dir, "new.txt") || readRepoFile(t, dir, "existing.txt") != "before the run\n" {
		t.Fatal("rollback did not restore the pre-run working tree")
	}
	if err := cp.CleanupCheckpoints(ctx, checkpointRunID, dir); err != nil {
		t.Fatalf("CleanupCheckpoints: %v", err)
	}
	assertCheckpointRefsGone(t, dir)
}

func TestRewindToFirst_RestoresTheUsersIndex(t *testing.T) {
	ctx := context.Background()
	dir := initCheckpointTestRepo(t)
	writeRepoFile(t, dir, "staged.txt", "staged by the user\n")
	gitRun(t, dir, "add", "staged.txt")
	writeRepoFile(t, dir, "initial.txt", "unstaged user change\n")
	cp := service.NewCheckpointService(git.NewPool(1))

	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Bash", "call-1"); err != nil {
		t.Fatal(err)
	}
	// The failed run stages everything it did.
	writeRepoFile(t, dir, "agent.txt", "agent\n")
	writeRepoFile(t, dir, "initial.txt", "agent change\n")
	gitRun(t, dir, "add", "-A")

	if err := cp.RewindToFirst(ctx, checkpointRunID, dir); err != nil {
		t.Fatalf("RewindToFirst: %v", err)
	}
	if staged := gitRun(t, dir, "diff", "--cached", "--name-only"); staged != "staged.txt" {
		t.Fatalf("staged after rollback = %q, want the user's pre-run staging (staged.txt)", staged)
	}
	if got := readRepoFile(t, dir, "initial.txt"); got != "unstaged user change\n" {
		t.Fatalf("initial.txt = %q, want the user's unstaged change", got)
	}
	if repoFileExists(dir, "agent.txt") {
		t.Fatal("agent.txt survived the rollback")
	}
}

func TestRewindToFirst_RestoresTheHeadTheRunBeganOn(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		setup func(t *testing.T) string // returns the repository
		agent func(t *testing.T, dir string)
		check func(t *testing.T, dir, preHead string)
	}{
		{
			name: "unborn branch, the agent commits",
			setup: func(t *testing.T) string {
				dir := initEmptyRepo(t)
				writeRepoFile(t, dir, "existing.txt", "x\n")
				return dir
			},
			agent: func(t *testing.T, dir string) {
				writeRepoFile(t, dir, "agent.txt", "agent\n")
				gitRun(t, dir, "add", "-A")
				gitRun(t, dir, "commit", "-q", "-m", "agent")
			},
			check: func(t *testing.T, dir, _ string) {
				if out, ok := gitTry(dir, "rev-parse", "--verify", "-q", "HEAD"); ok {
					t.Fatalf("HEAD = %s, want the unborn branch back", out)
				}
				if ref := gitRun(t, dir, "symbolic-ref", "HEAD"); ref != "refs/heads/main" {
					t.Fatalf("HEAD -> %s, want refs/heads/main", ref)
				}
				if staged := gitRun(t, dir, "diff", "--cached", "--name-only"); staged != "" {
					t.Fatalf("staged after rollback = %q, want nothing (as before the run)", staged)
				}
			},
		},
		{
			name:  "branch, the agent switches branch and commits",
			setup: initCheckpointTestRepo,
			agent: func(t *testing.T, dir string) {
				gitRun(t, dir, "checkout", "-q", "-b", "feature")
				writeRepoFile(t, dir, "agent.txt", "agent\n")
				gitRun(t, dir, "add", "-A")
				gitRun(t, dir, "commit", "-q", "-m", "agent")
			},
			check: func(t *testing.T, dir, preHead string) {
				if ref := gitRun(t, dir, "symbolic-ref", "--short", "HEAD"); ref == "feature" {
					t.Fatal("HEAD is still on the agent's branch")
				}
				if head := gitRun(t, dir, "rev-parse", "HEAD"); head != preHead {
					t.Fatalf("HEAD = %s, want %s", head, preHead)
				}
			},
		},
		{
			name: "detached HEAD, the agent checks out a branch and commits",
			setup: func(t *testing.T) string {
				dir := initCheckpointTestRepo(t)
				gitRun(t, dir, "checkout", "-q", "--detach")
				return dir
			},
			agent: func(t *testing.T, dir string) {
				gitRun(t, dir, "checkout", "-q", "-b", "work")
				writeRepoFile(t, dir, "agent.txt", "agent\n")
				gitRun(t, dir, "add", "-A")
				gitRun(t, dir, "commit", "-q", "-m", "agent")
			},
			check: func(t *testing.T, dir, preHead string) {
				if _, ok := gitTry(dir, "symbolic-ref", "-q", "HEAD"); ok {
					t.Fatal("HEAD is on a branch, want it detached as before the run")
				}
				if head := gitRun(t, dir, "rev-parse", "HEAD"); head != preHead {
					t.Fatalf("HEAD = %s, want %s", head, preHead)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.setup(t)
			preHead, _ := gitTry(dir, "rev-parse", "HEAD")
			cp := service.NewCheckpointService(git.NewPool(1))
			if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Bash", "call-1"); err != nil {
				t.Fatal(err)
			}
			tc.agent(t, dir)

			if err := cp.RewindToFirst(ctx, checkpointRunID, dir); err != nil {
				t.Fatalf("RewindToFirst: %v", err)
			}
			tc.check(t, dir, preHead)
			if repoFileExists(dir, "agent.txt") {
				t.Fatal("agent.txt survived the rollback")
			}
		})
	}
}

func TestCheckpoints_SurviveAGoCoreRestart(t *testing.T) {
	ctx := context.Background()
	dir := initCheckpointTestRepo(t)
	writeRepoFile(t, dir, "notes.txt", "user\n")

	before := service.NewCheckpointService(git.NewPool(1))
	if err := before.CreateCheckpoint(ctx, checkpointRunID, dir, "Write", "call-1"); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, dir, "first.txt", "before the restart\n")

	// A new process (or another replica) continues the run.
	after := service.NewCheckpointService(git.NewPool(1))
	if err := after.CreateCheckpoint(ctx, checkpointRunID, dir, "Write", "call-2"); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, dir, "second.txt", "after the restart\n")

	if err := after.RewindToFirst(ctx, checkpointRunID, dir); err != nil {
		t.Fatalf("RewindToFirst after a restart: %v", err)
	}
	if repoFileExists(dir, "first.txt") || repoFileExists(dir, "second.txt") || readRepoFile(t, dir, "notes.txt") != "user\n" {
		t.Fatal("rollback after a restart did not restore the state before the run's first change")
	}
	if err := service.NewCheckpointService(git.NewPool(1)).CleanupCheckpoints(ctx, checkpointRunID, dir); err != nil {
		t.Fatalf("CleanupCheckpoints in a fresh process: %v", err)
	}
	assertCheckpointRefsGone(t, dir)
}

func TestRewindToFirst_NoCheckpointsOrBrokenChain(t *testing.T) {
	ctx := context.Background()
	dir := initCheckpointTestRepo(t)
	cp := service.NewCheckpointService(git.NewPool(1))

	if err := cp.RewindToFirst(ctx, checkpointRunID, dir); !errors.Is(err, service.ErrNoCheckpoints) {
		t.Fatalf("RewindToFirst without checkpoints = %v, want ErrNoCheckpoints", err)
	}
	// A chain without base (tampered or foreign ref) is an error, not "no
	// checkpoints": the run did change files.
	gitRun(t, dir, "update-ref", "refs/codeforge/checkpoints/"+checkpointRunID, "HEAD")
	err := cp.RewindToFirst(ctx, checkpointRunID, dir)
	if err == nil || errors.Is(err, service.ErrNoCheckpoints) {
		t.Fatalf("RewindToFirst with a broken chain = %v, want an error other than ErrNoCheckpoints", err)
	}
}

func TestPatchDelivery_HoldsExactlyTheRunsChange(t *testing.T) {
	ctx := context.Background()
	dir := initCheckpointTestRepo(t)
	pool := git.NewPool(1)
	cp := service.NewCheckpointService(pool)
	deliverer := service.NewDeliverService(&deliverMockStore{proj: &project.Project{ID: "proj-1", WorkspacePath: dir}},
		&config.Runtime{DeliveryCommitPrefix: "codeforge:"}, pool)

	// Before the run: user work in progress and an earlier run's patch file.
	writeRepoFile(t, dir, "initial.txt", "user wip\n")
	writeRepoFile(t, dir, "notes.txt", "mine\n")
	writeRepoFile(t, dir, "run-old00000.patch", "an earlier delivery\n")
	statusBefore := gitRun(t, dir, "status", "--porcelain")

	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Write", "call-1"); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, dir, "new.txt", "from the run\n")
	writeRepoFile(t, dir, "initial.txt", "user wip\nand the run\n")

	result, err := deliverer.Deliver(ctx, &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: run.DeliverModePatch}, "task")
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	patch := readFileAt(t, result.PatchPath)
	for _, want := range []string{"new.txt", "+and the run"} {
		if !strings.Contains(patch, want) {
			t.Errorf("patch misses %q:\n%s", want, patch)
		}
	}
	for _, unwanted := range []string{"notes.txt", "run-old00000.patch", "+user wip"} {
		if strings.Contains(patch, unwanted) {
			t.Errorf("patch holds %q, which is not the run's change:\n%s", unwanted, patch)
		}
	}
	if !strings.HasPrefix(result.PatchPath, filepath.Join(dir, ".git")+string(filepath.Separator)) {
		t.Fatalf("patch written to %s, want it inside .git where git ignores it", result.PatchPath)
	}
	// The working tree shows no delivery artifact.
	statusAfter := gitRun(t, dir, "status", "--porcelain")
	if strings.Count(statusAfter, ".patch") != strings.Count(statusBefore, ".patch") {
		t.Fatalf("patch delivery added a file to the working tree:\n%s", statusAfter)
	}
	// The patch applies to the pre-run state and yields the run's result.
	clone := filepath.Join(t.TempDir(), "clone")
	gitRun(t, dir, "clone", "-q", dir, clone)
	writeRepoFile(t, clone, "initial.txt", "user wip\n")
	gitRun(t, clone, "apply", result.PatchPath)
	if got := readRepoFile(t, clone, "initial.txt"); got != "user wip\nand the run\n" {
		t.Fatalf("patched initial.txt = %q", got)
	}
}

func TestPatchDelivery_WithoutCheckpointsFails(t *testing.T) {
	dir := initCheckpointTestRepo(t)
	writeRepoFile(t, dir, "initial.txt", "changed outside any run\n")
	deliverer := service.NewDeliverService(&deliverMockStore{proj: &project.Project{ID: "proj-1", WorkspacePath: dir}},
		&config.Runtime{}, git.NewPool(1))

	_, err := deliverer.Deliver(context.Background(), &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: run.DeliverModePatch}, "task")
	if !errors.Is(err, service.ErrNoCheckpoints) {
		t.Fatalf("patch delivery without checkpoints = %v, want ErrNoCheckpoints (the run's change is unknown)", err)
	}
}

func TestCheckpoints_PrivateIndexKeptAcrossTheRun(t *testing.T) {
	ctx := context.Background()
	dir := initCheckpointTestRepo(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	cp := service.NewCheckpointService(git.NewPool(1))

	for i, call := range []string{"call-1", "call-2", "call-3"} {
		writeRepoFile(t, dir, call+".txt", call)
		if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Write", call); err != nil {
			t.Fatal(err)
		}
		indexes := runIndexes(t, tmp)
		if len(indexes) != 1 {
			t.Fatalf("after checkpoint %d: private indexes = %v, want the run's one", i+1, indexes)
		}
	}
	if err := cp.CleanupCheckpoints(ctx, checkpointRunID, dir); err != nil {
		t.Fatal(err)
	}
	if indexes := runIndexes(t, tmp); len(indexes) != 0 {
		t.Fatalf("private indexes after cleanup = %v, want none", indexes)
	}
}

// runIndexes lists the per-run private index files under tmp.
func runIndexes(t *testing.T, tmp string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(tmp, "codeforge-checkpoints-*", "*.index"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func readFileAt(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestPatchDelivery_RefusesASymlinkedPatchDirectory(t *testing.T) {
	ctx := context.Background()
	dir := initCheckpointTestRepo(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, ".git", "codeforge")); err != nil {
		t.Fatal(err)
	}
	pool := git.NewPool(1)
	cp := service.NewCheckpointService(pool)
	deliverer := service.NewDeliverService(&deliverMockStore{proj: &project.Project{ID: "proj-1", WorkspacePath: dir}},
		&config.Runtime{}, pool)
	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Write", "call-1"); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, dir, "new.txt", "x\n")

	if _, err := deliverer.Deliver(ctx, &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: run.DeliverModePatch}, "task"); err == nil {
		t.Fatal("patch delivery followed a symlink out of the repository")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("files written outside the repository: %v", entries)
	}
}
