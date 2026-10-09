package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/service"
)

// Scratch-repo tests for KI-27: checkpoints must never become part of the
// workspace's history, so every delivery mode sees the run's full change and
// nothing else, whether it runs before or after the checkpoint cleanup.

const checkpointRunID = "run-abcd1234-0000"

func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readRepoFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // test file in a temp dir
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func repoFileExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// runWithCheckpoints drives a run on the scratch repo the way the runtime
// does: a checkpoint before each file-modifying tool call, then the edit. The
// run adds new.txt and changes initial.txt twice; its last edit alone is not
// the run's change.
func runWithCheckpoints(ctx context.Context, t *testing.T, cp *service.CheckpointService, dir string) {
	t.Helper()
	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Write", "call-1"); err != nil {
		t.Fatalf("checkpoint 1: %v", err)
	}
	writeRepoFile(t, dir, "new.txt", "new file\n")
	writeRepoFile(t, dir, "initial.txt", "changed once\n")
	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Edit", "call-2"); err != nil {
		t.Fatalf("checkpoint 2: %v", err)
	}
	writeRepoFile(t, dir, "initial.txt", "changed twice\n")
}

func assertNoCheckpointHistory(t *testing.T, dir, rev string) {
	t.Helper()
	if log := gitRun(t, dir, "log", "--format=%s", rev); strings.Contains(log, "codeforge-checkpoint") {
		t.Fatalf("checkpoint commits in the history of %s:\n%s", rev, log)
	}
}

func assertCheckpointRefsGone(t *testing.T, dir string) {
	t.Helper()
	if refs := gitRun(t, dir, "for-each-ref", "refs/codeforge"); refs != "" {
		t.Fatalf("checkpoint refs left after cleanup:\n%s", refs)
	}
}

func TestCheckpointedRun_DeliversTheFullChange(t *testing.T) {
	orders := []struct {
		name          string
		deliverFirst  bool
		cleanupBefore bool
	}{
		{name: "deliver then cleanup", deliverFirst: true},
		{name: "cleanup then deliver", cleanupBefore: true},
	}
	modes := []run.DeliverMode{run.DeliverModeCommitLocal, run.DeliverModeBranch, run.DeliverModePatch}

	for _, order := range orders {
		for _, mode := range modes {
			t.Run(order.name+"/"+string(mode), func(t *testing.T) {
				dir := initCheckpointTestRepo(t)
				ctx := context.Background()
				base := gitRun(t, dir, "rev-parse", "HEAD")
				startBranch := gitRun(t, dir, "rev-parse", "--abbrev-ref", "HEAD")
				pool := git.NewPool(5)
				cp := service.NewCheckpointService(pool)
				deliverer := service.NewDeliverService(&deliverMockStore{proj: &project.Project{ID: "proj-1", WorkspacePath: dir}},
					&config.Runtime{DeliveryCommitPrefix: "codeforge:"}, pool)
				r := &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: mode}

				runWithCheckpoints(ctx, t, cp, dir)

				if order.cleanupBefore {
					if err := cp.CleanupCheckpoints(ctx, r.ID, dir); err != nil {
						t.Fatalf("cleanup: %v", err)
					}
				}
				result, err := deliverer.Deliver(ctx, r, "add feature")
				if order.cleanupBefore {
					// Every delivery is the change since the run's base
					// checkpoint; the runtime delivers before the cleanup.
					if !errors.Is(err, service.ErrNoCheckpoints) {
						t.Fatalf("patch after cleanup = %v, want ErrNoCheckpoints", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("deliver: %v", err)
				}
				if order.deliverFirst {
					if err := cp.CleanupCheckpoints(ctx, r.ID, dir); err != nil {
						t.Fatalf("cleanup: %v", err)
					}
				}
				assertCheckpointRefsGone(t, dir)

				switch mode {
				case run.DeliverModeCommitLocal:
					if branch := gitRun(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); branch != startBranch {
						t.Fatalf("commit-local moved to branch %q", branch)
					}
					assertDeliveredCommit(t, dir, base, "HEAD", result.CommitHash)
					if status := gitRun(t, dir, "status", "--porcelain"); status != "" {
						t.Fatalf("commit-local left changes outside the delivered commit:\n%s", status)
					}
				case run.DeliverModeBranch:
					assertDeliveredCommit(t, dir, base, result.BranchName, result.CommitHash)
				case run.DeliverModePatch:
					if head := gitRun(t, dir, "rev-parse", "HEAD"); head != base {
						t.Fatalf("patch delivery moved HEAD from %s to %s", base, head)
					}
					if staged := gitRun(t, dir, "diff", "--cached", "--name-only"); staged != "" {
						t.Fatalf("patch delivery changed the user's index:\n%s", staged)
					}
					assertPatchReproducesRun(t, dir, result.PatchPath)
				}
			})
		}
	}
}

// assertDeliveredCommit checks that rev is exactly one commit on top of base
// (no checkpoint commits) and holds the run's full change.
func assertDeliveredCommit(t *testing.T, dir, base, rev, commitHash string) {
	t.Helper()
	if got := gitRun(t, dir, "rev-list", "--count", base+".."+rev); got != "1" {
		t.Fatalf("%s is %s commits on top of the base, want 1:\n%s", rev, got, gitRun(t, dir, "log", "--format=%h %s", base+".."+rev))
	}
	if head := gitRun(t, dir, "rev-parse", rev); head != commitHash {
		t.Fatalf("delivered commit %s, %s is at %s", commitHash, rev, head)
	}
	assertNoCheckpointHistory(t, dir, rev)
	if got := gitRun(t, dir, "show", rev+":initial.txt"); got != "changed twice" {
		t.Fatalf("delivered initial.txt = %q, want the run's last content", got)
	}
	if got := gitRun(t, dir, "show", rev+":new.txt"); got != "new file" {
		t.Fatalf("delivered new.txt = %q, want the file the run added", got)
	}
}

// assertPatchReproducesRun applies the patch to a clone of the base and
// compares the result with the run's change.
func assertPatchReproducesRun(t *testing.T, dir, patchPath string) {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "clone")
	gitRun(t, dir, "clone", "-q", dir, clone)
	gitRun(t, clone, "apply", patchPath)
	if got := readRepoFile(t, clone, "initial.txt"); got != "changed twice\n" {
		t.Fatalf("patched initial.txt = %q, want the run's last content", got)
	}
	if got := readRepoFile(t, clone, "new.txt"); got != "new file\n" {
		t.Fatalf("patched new.txt = %q, want the file the run added", got)
	}
}

func TestCreateCheckpoint_LeavesTheWorkspaceHistoryAlone(t *testing.T) {
	dir := initCheckpointTestRepo(t)
	ctx := context.Background()
	cp := service.NewCheckpointService(git.NewPool(5))

	// The user's work in progress: a staged change, an unstaged one and an
	// untracked file.
	writeRepoFile(t, dir, "staged.txt", "staged\n")
	gitRun(t, dir, "add", "staged.txt")
	writeRepoFile(t, dir, "initial.txt", "user wip\n")
	writeRepoFile(t, dir, "notes.txt", "mine\n")
	// A failing pre-commit hook and no git identity must not stop checkpoints.
	writeRepoFile(t, filepath.Join(dir, ".git", "hooks"), "pre-commit", "#!/bin/sh\nexit 1\n")
	if err := os.Chmod(filepath.Join(dir, ".git", "hooks", "pre-commit"), 0o755); err != nil { //nolint:gosec // executable hook in a temp repo
		t.Fatal(err)
	}
	gitRun(t, dir, "config", "--unset", "user.name")
	gitRun(t, dir, "config", "--unset", "user.email")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "no-gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	head := gitRun(t, dir, "rev-parse", "HEAD")
	status := gitRun(t, dir, "status", "--porcelain")

	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Edit", "call-1"); err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}

	if got := gitRun(t, dir, "rev-parse", "HEAD"); got != head {
		t.Fatalf("checkpoint moved HEAD from %s to %s", head, got)
	}
	if got := gitRun(t, dir, "status", "--porcelain"); got != status {
		t.Fatalf("checkpoint changed the index or working tree:\nbefore:\n%s\nafter:\n%s", status, got)
	}
	assertNoCheckpointHistory(t, dir, "HEAD")

	cps := cp.GetCheckpoints(checkpointRunID)
	if len(cps) != 1 {
		t.Fatalf("checkpoints = %d, want 1", len(cps))
	}
	ref := gitRun(t, dir, "for-each-ref", "--format=%(objectname)", "refs/codeforge/checkpoints/"+checkpointRunID)
	if ref != cps[0].CommitHash {
		t.Fatalf("checkpoint ref = %q, want %s", ref, cps[0].CommitHash)
	}
	for name, want := range map[string]string{"initial.txt": "user wip", "notes.txt": "mine", "staged.txt": "staged"} {
		if got := gitRun(t, dir, "show", cps[0].CommitHash+":"+name); got != want {
			t.Errorf("checkpoint %s = %q, want %q", name, got, want)
		}
	}
}

func TestRewindToFirst_RestoresThePreRunWorkspace(t *testing.T) {
	tests := []struct {
		name         string
		agentCommits bool
	}{
		{name: "agent edits"},
		{name: "agent edits and commits", agentCommits: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := initCheckpointTestRepo(t)
			ctx := context.Background()
			cp := service.NewCheckpointService(git.NewPool(5))
			base := gitRun(t, dir, "rev-parse", "HEAD")

			// The user's uncommitted work before the run.
			writeRepoFile(t, dir, "initial.txt", "user wip\n")
			writeRepoFile(t, dir, "notes.txt", "mine\n")

			if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Write", "call-1"); err != nil {
				t.Fatal(err)
			}
			writeRepoFile(t, dir, "new.txt", "agent\n")
			writeRepoFile(t, dir, "initial.txt", "agent\n")
			if err := os.Remove(filepath.Join(dir, "notes.txt")); err != nil {
				t.Fatal(err)
			}
			if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Bash", "call-2"); err != nil {
				t.Fatal(err)
			}
			writeRepoFile(t, dir, "new.txt", "agent again\n")
			if tc.agentCommits {
				gitRun(t, dir, "add", "-A")
				gitRun(t, dir, "commit", "-q", "-m", "agent commit")
			}

			if err := cp.RewindToFirst(ctx, checkpointRunID, dir); err != nil {
				t.Fatalf("RewindToFirst: %v", err)
			}

			if got := readRepoFile(t, dir, "initial.txt"); got != "user wip\n" {
				t.Errorf("initial.txt = %q, want the user's pre-run content", got)
			}
			if got := readRepoFile(t, dir, "notes.txt"); got != "mine\n" {
				t.Errorf("notes.txt = %q, want the user's untracked file back", got)
			}
			if repoFileExists(dir, "new.txt") {
				t.Error("new.txt, added by the run, still exists")
			}
			if head := gitRun(t, dir, "rev-parse", "HEAD"); head != base {
				t.Errorf("HEAD = %s, want the pre-run commit %s", head, base)
			}
			assertNoCheckpointHistory(t, dir, "HEAD")

			if err := cp.CleanupCheckpoints(ctx, checkpointRunID, dir); err != nil {
				t.Fatalf("cleanup: %v", err)
			}
			assertCheckpointRefsGone(t, dir)
			if got := readRepoFile(t, dir, "initial.txt"); got != "user wip\n" {
				t.Errorf("cleanup changed initial.txt to %q", got)
			}
		})
	}
}

func TestCreateCheckpoint_WorkspaceWithoutRepository(t *testing.T) {
	dir := t.TempDir()
	writeRepoFile(t, dir, "file.txt", "content\n")
	cp := service.NewCheckpointService(git.NewPool(1))
	ctx := context.Background()

	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Edit", "call-1"); err == nil {
		t.Fatal("CreateCheckpoint succeeded in a directory without a repository")
	}
	if cps := cp.GetCheckpoints(checkpointRunID); len(cps) != 0 {
		t.Fatalf("checkpoints = %d, want none", len(cps))
	}
	if err := cp.RewindToFirst(ctx, checkpointRunID, dir); err == nil {
		t.Fatal("RewindToFirst succeeded without checkpoints")
	}
	if err := cp.CleanupCheckpoints(ctx, checkpointRunID, dir); err != nil {
		t.Fatalf("cleanup without checkpoints: %v", err)
	}
	if got := readRepoFile(t, dir, "file.txt"); got != "content\n" {
		t.Fatalf("file.txt = %q, want it untouched", got)
	}
}
