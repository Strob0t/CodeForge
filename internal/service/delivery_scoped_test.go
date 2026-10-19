package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/service"
)

// S3 follow-up 1a: commit-local and branch delivery commit exactly the run's
// change (base checkpoint -> final working tree) on top of HEAD; the user's
// uncommitted pre-run work stays uncommitted in the working tree.

type scopedDeliveryEnv struct {
	dir       string
	base      string // HEAD before the run
	branch    string // branch checked out before the run
	cp        *service.CheckpointService
	deliverer *service.DeliverService
}

// newScopedDeliveryEnv prepares a repository with the user's work in
// progress (a modified tracked file, a staged change, an untracked file)
// and the run's first checkpoint.
func newScopedDeliveryEnv(t *testing.T) *scopedDeliveryEnv {
	t.Helper()
	dir := initCheckpointTestRepo(t)
	writeRepoFile(t, dir, "shared.txt", "l1\nl2\nl3\nl4\nl5\n")
	writeRepoFile(t, dir, "user.txt", "user base\n")
	writeRepoFile(t, dir, "staged.txt", "staged base\n")
	writeRepoFile(t, dir, "doomed.txt", "the run deletes me\n")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "fixture")

	// The user's uncommitted work before the run.
	writeRepoFile(t, dir, "user.txt", "user wip\n")
	writeRepoFile(t, dir, "shared.txt", "u1\nl2\nl3\nl4\nl5\n")
	writeRepoFile(t, dir, "staged.txt", "staged by the user\n")
	gitRun(t, dir, "add", "staged.txt")
	writeRepoFile(t, dir, "notes.txt", "untracked user notes\n")

	pool := git.NewPool(2)
	env := &scopedDeliveryEnv{
		dir:    dir,
		base:   gitRun(t, dir, "rev-parse", "HEAD"),
		branch: gitRun(t, dir, "symbolic-ref", "--short", "HEAD"),
		cp:     service.NewCheckpointService(pool),
		deliverer: service.NewDeliverService(&deliverMockStore{proj: &project.Project{ID: "proj-1", WorkspacePath: dir}},
			&config.Runtime{DeliveryCommitPrefix: "codeforge:"}, pool),
	}
	if err := env.cp.CreateCheckpoint(context.Background(), checkpointRunID, dir, "Edit", "call-1"); err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}
	return env
}

// runEdits is the run's change: one line of shared.txt, a new file and a
// deleted file.
func (e *scopedDeliveryEnv) runEdits(t *testing.T) {
	t.Helper()
	writeRepoFile(t, e.dir, "shared.txt", "u1\nl2\nl3\nl4\nr5\n")
	writeRepoFile(t, e.dir, "added.txt", "added by the run\n")
	if err := os.Remove(filepath.Join(e.dir, "doomed.txt")); err != nil {
		t.Fatal(err)
	}
}

func (e *scopedDeliveryEnv) deliver(mode run.DeliverMode) (*service.DeliveryResult, error) {
	return e.deliverer.Deliver(context.Background(), &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: mode}, "task")
}

func TestCommitDelivery_CommitsOnlyTheRunsChange(t *testing.T) {
	for _, mode := range []run.DeliverMode{run.DeliverModeCommitLocal, run.DeliverModeBranch} {
		t.Run(string(mode), func(t *testing.T) {
			env := newScopedDeliveryEnv(t)
			env.runEdits(t)

			result, err := env.deliver(mode)
			if err != nil {
				t.Fatalf("Deliver: %v", err)
			}
			dir, commit := env.dir, result.CommitHash

			if parent := gitRun(t, dir, "rev-parse", commit+"^"); parent != env.base {
				t.Fatalf("delivered commit's parent = %s, want the pre-run HEAD %s", parent, env.base)
			}
			changed := strings.Fields(gitRun(t, dir, "diff-tree", "--no-commit-id", "--name-only", "-r", "--no-renames", commit))
			if want := []string{"added.txt", "doomed.txt", "shared.txt"}; !slices.Equal(changed, want) {
				t.Fatalf("delivered commit changes %v, want exactly the run's change %v", changed, want)
			}
			if got := gitRun(t, dir, "show", commit+":shared.txt"); got != "l1\nl2\nl3\nl4\nr5" {
				t.Fatalf("delivered shared.txt = %q, want the run's line without the user's", got)
			}
			if got := gitRun(t, dir, "show", commit+":user.txt"); got != "user base" {
				t.Fatalf("delivered user.txt = %q, want it unchanged", got)
			}
			if got := gitRun(t, dir, "show", commit+":staged.txt"); got != "staged base" {
				t.Fatalf("delivered staged.txt = %q, want the user's staged change left out", got)
			}
			if _, ok := gitTry(dir, "cat-file", "-e", commit+":notes.txt"); ok {
				t.Fatal("the user's untracked file was committed")
			}

			// The user's work stays as it was: uncommitted, staged where staged.
			if got := readRepoFile(t, dir, "user.txt"); got != "user wip\n" {
				t.Fatalf("user.txt = %q", got)
			}
			if got := readRepoFile(t, dir, "shared.txt"); got != "u1\nl2\nl3\nl4\nr5\n" {
				t.Fatalf("shared.txt = %q", got)
			}
			if got := gitRun(t, dir, "diff", "--cached", "--name-only"); got != "staged.txt" {
				t.Fatalf("staged after delivery = %q, want the user's staged.txt", got)
			}
			unstaged := strings.Fields(gitRun(t, dir, "diff", "--name-only"))
			if want := []string{"shared.txt", "user.txt"}; !slices.Equal(unstaged, want) {
				t.Fatalf("unstaged after delivery = %v, want the user's work %v", unstaged, want)
			}
			if got := gitRun(t, dir, "diff", "shared.txt"); !strings.Contains(got, "+u1") || strings.Contains(got, "r5") {
				t.Fatalf("unstaged diff of shared.txt = %q, want only the user's line", got)
			}
			if got := gitRun(t, dir, "status", "--porcelain", "--", "notes.txt"); got != "?? notes.txt" {
				t.Fatalf("notes.txt status = %q, want untracked", got)
			}

			switch mode {
			case run.DeliverModeCommitLocal:
				if head := gitRun(t, dir, "rev-parse", "HEAD"); head != commit {
					t.Fatalf("HEAD = %s, want the delivered commit", head)
				}
				if branch := gitRun(t, dir, "symbolic-ref", "--short", "HEAD"); branch != env.branch {
					t.Fatalf("HEAD moved to branch %s", branch)
				}
			case run.DeliverModeBranch:
				if branch := gitRun(t, dir, "symbolic-ref", "--short", "HEAD"); branch != result.BranchName {
					t.Fatalf("HEAD on %s, want the delivery branch %s", branch, result.BranchName)
				}
				if tip := gitRun(t, dir, "rev-parse", env.branch); tip != env.base {
					t.Fatalf("original branch moved to %s", tip)
				}
			}
		})
	}
}

func TestCommitDelivery_ChangeOverlappingTheUsersWorkFails(t *testing.T) {
	for _, mode := range []run.DeliverMode{run.DeliverModeCommitLocal, run.DeliverModeBranch} {
		t.Run(string(mode), func(t *testing.T) {
			env := newScopedDeliveryEnv(t)
			// The run rewrites the line the user changed before the run.
			writeRepoFile(t, env.dir, "shared.txt", "r1\nl2\nl3\nl4\nl5\n")
			statusBefore := gitRun(t, env.dir, "status", "--porcelain")

			if _, err := env.deliver(mode); err == nil || !strings.Contains(err.Error(), "shared.txt") {
				t.Fatalf("Deliver = %v, want an error naming shared.txt", err)
			}
			if head := gitRun(t, env.dir, "rev-parse", "HEAD"); head != env.base {
				t.Fatalf("HEAD moved to %s", head)
			}
			if branch := gitRun(t, env.dir, "symbolic-ref", "--short", "HEAD"); branch != env.branch {
				t.Fatalf("HEAD moved to branch %s", branch)
			}
			if got := gitRun(t, env.dir, "status", "--porcelain"); got != statusBefore {
				t.Fatalf("workspace changed:\nbefore:\n%s\nafter:\n%s", statusBefore, got)
			}
		})
	}
}

func TestCommitDelivery_WithoutCheckpointsOrChangeFails(t *testing.T) {
	for _, mode := range []run.DeliverMode{run.DeliverModeCommitLocal, run.DeliverModeBranch} {
		t.Run(string(mode)+"/no checkpoints", func(t *testing.T) {
			dir := initCheckpointTestRepo(t)
			writeRepoFile(t, dir, "initial.txt", "changed outside any run\n")
			deliverer := service.NewDeliverService(&deliverMockStore{proj: &project.Project{ID: "proj-1", WorkspacePath: dir}},
				&config.Runtime{}, git.NewPool(1))
			_, err := deliverer.Deliver(context.Background(), &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: mode}, "task")
			if !errors.Is(err, service.ErrNoCheckpoints) {
				t.Fatalf("Deliver = %v, want ErrNoCheckpoints", err)
			}
		})
		t.Run(string(mode)+"/no change", func(t *testing.T) {
			env := newScopedDeliveryEnv(t)
			if _, err := env.deliver(mode); err == nil || !strings.Contains(err.Error(), "nothing to commit") {
				t.Fatalf("Deliver without a change = %v, want nothing to commit", err)
			}
			if head := gitRun(t, env.dir, "rev-parse", "HEAD"); head != env.base {
				t.Fatalf("HEAD moved to %s", head)
			}
		})
	}
}

func TestCommitDelivery_UnbornBranch(t *testing.T) {
	ctx := context.Background()
	dir := initEmptyRepo(t)
	writeRepoFile(t, dir, "mine.txt", "the user's file\n")
	pool := git.NewPool(1)
	cp := service.NewCheckpointService(pool)
	deliverer := service.NewDeliverService(&deliverMockStore{proj: &project.Project{ID: "proj-1", WorkspacePath: dir}},
		&config.Runtime{DeliveryCommitPrefix: "codeforge:"}, pool)
	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Write", "call-1"); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, dir, "run.txt", "the run's file\n")

	result, err := deliverer.Deliver(ctx, &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: run.DeliverModeCommitLocal}, "task")
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if files := gitRun(t, dir, "ls-tree", "--name-only", result.CommitHash); files != "run.txt" {
		t.Fatalf("delivered files = %q, want only run.txt", files)
	}
	if _, ok := gitTry(dir, "rev-parse", "--verify", "-q", result.CommitHash+"^"); ok {
		t.Fatal("the first commit of the branch has a parent")
	}
	if got := gitRun(t, dir, "status", "--porcelain"); got != "?? mine.txt" {
		t.Fatalf("status = %q, want the user's file untracked and nothing else", got)
	}
}
