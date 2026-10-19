package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-77: checkpoints and delivery run git in the agent-writable workspace.
// Planted repository config, attributes and hooks must never run in the Go
// Core, and a repository that points outside the workspace is refused.

// plantedProgram writes a program that records its runs and returns its path
// and the marker file it writes.
func plantedProgram(t *testing.T) (program, marker string) {
	t.Helper()
	dir := t.TempDir()
	marker = filepath.Join(dir, "marker")
	program = filepath.Join(dir, "evil.sh")
	if err := os.WriteFile(program, []byte("#!/bin/sh\necho \"$0 $*\" >> "+marker+"\ncat\n"), 0o755); err != nil { //nolint:gosec // executable test program
		t.Fatal(err)
	}
	return program, marker
}

func assertProgramNotRun(t *testing.T, marker string) {
	t.Helper()
	if data, err := os.ReadFile(marker); err == nil { //nolint:gosec // test marker
		t.Fatalf("a program planted in the workspace ran in the Go Core: %s", data)
	}
}

// plantAttacks configures fsmonitor, a filter driver with attributes and
// hooks (incl. reference-transaction) in the repository at dir.
func plantAttacks(t *testing.T, dir, program string) {
	t.Helper()
	gitRun(t, dir, "config", "core.fsmonitor", program)
	gitRun(t, dir, "config", "filter.x.clean", program+" clean")
	gitRun(t, dir, "config", "filter.x.smudge", program+" smudge")
	gitRun(t, dir, "config", "filter.x.required", "true")
	// A filtered file the run does not change: commit delivery refuses
	// changed filtered files (security review P3).
	writeRepoFile(t, dir, ".gitattributes", "* diff=x\n*.dat filter=x\n")
	writeRepoFile(t, dir, "data.dat", "planted\n")
	for _, hook := range []string{"reference-transaction", "pre-commit", "post-commit", "post-checkout", "post-index-change"} {
		writeRepoFile(t, filepath.Join(dir, ".git", "hooks"), hook, "#!/bin/sh\n"+program+" hook-"+hook+"\n")
		if err := os.Chmod(filepath.Join(dir, ".git", "hooks", hook), 0o755); err != nil { //nolint:gosec // executable hook
			t.Fatal(err)
		}
	}
}

func TestCheckpointsAndDelivery_PlantedGitConfigNeverRuns(t *testing.T) {
	for _, mode := range []run.DeliverMode{run.DeliverModePatch, run.DeliverModeCommitLocal, run.DeliverModeBranch} {
		t.Run(string(mode), func(t *testing.T) {
			ctx := context.Background()
			dir := initCheckpointTestRepo(t)
			program, marker := plantedProgram(t)
			plantAttacks(t, dir, program)
			pool := git.NewPool(2)
			cp := service.NewCheckpointService(pool)
			deliverer := service.NewDeliverService(&deliverMockStore{proj: &project.Project{ID: "proj-1", WorkspacePath: dir}},
				&config.Runtime{DeliveryCommitPrefix: "codeforge:"}, pool)

			runWithCheckpoints(ctx, t, cp, dir)
			assertProgramNotRun(t, marker)
			if _, err := deliverer.Deliver(ctx, &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: mode}, "task"); err != nil {
				t.Fatalf("deliver: %v", err)
			}
			assertProgramNotRun(t, marker)
			if err := cp.RewindToFirst(ctx, checkpointRunID, dir); err != nil {
				t.Fatalf("rewind: %v", err)
			}
			assertProgramNotRun(t, marker)
			if err := cp.CleanupCheckpoints(ctx, checkpointRunID, dir); err != nil {
				t.Fatalf("cleanup: %v", err)
			}
			assertProgramNotRun(t, marker)
		})
	}
}

func TestCheckpoints_RepositoryPointingOutsideIsRefused(t *testing.T) {
	ctx := context.Background()
	other := initCheckpointTestRepo(t) // another tenant's workspace
	otherHead := gitRun(t, other, "rev-parse", "HEAD")
	tests := []struct {
		name  string
		plant func(t *testing.T, dir string)
	}{
		{"core.worktree", func(t *testing.T, dir string) { gitRun(t, dir, "config", "core.worktree", other) }},
		{"include.path", func(t *testing.T, dir string) {
			gitRun(t, dir, "config", "include.path", filepath.Join(other, ".git", "config"))
		}},
		{".git file", func(t *testing.T, dir string) {
			if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
				t.Fatal(err)
			}
			writeRepoFile(t, dir, ".git", "gitdir: "+filepath.Join(other, ".git")+"\n")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := initCheckpointTestRepo(t)
			tc.plant(t, dir)
			cp := service.NewCheckpointService(git.NewPool(1))

			err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Edit", "call-1")
			if !errors.Is(err, git.ErrUnsafeRepository) {
				t.Fatalf("CreateCheckpoint = %v, want ErrUnsafeRepository", err)
			}
			if got := gitRun(t, other, "rev-parse", "HEAD"); got != otherHead {
				t.Fatal("the other repository's HEAD changed")
			}
			if refs := gitRun(t, other, "for-each-ref", "refs/codeforge"); refs != "" {
				t.Fatalf("checkpoint refs were written into the other repository: %s", refs)
			}
		})
	}
}

func TestCheckpoints_LFSRepositoryKeepsWorking(t *testing.T) {
	ctx := context.Background()
	dir := initCheckpointTestRepo(t)
	// git lfs install --local, while the Go Core has no git-lfs.
	gitRun(t, dir, "config", "filter.lfs.clean", "git-lfs clean -- %f")
	gitRun(t, dir, "config", "filter.lfs.smudge", "git-lfs smudge -- %f")
	gitRun(t, dir, "config", "filter.lfs.process", "git-lfs filter-process")
	gitRun(t, dir, "config", "filter.lfs.required", "true")
	writeRepoFile(t, dir, ".gitattributes", "*.bin filter=lfs diff=lfs merge=lfs -text\n")
	cp := service.NewCheckpointService(git.NewPool(1))

	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Write", "call-1"); err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}
	writeRepoFile(t, dir, "model.bin", "weights\n")
	if err := cp.RewindToFirst(ctx, checkpointRunID, dir); err != nil {
		t.Fatalf("RewindToFirst: %v", err)
	}
	if repoFileExists(dir, "model.bin") {
		t.Fatal("model.bin, added by the run, survived the rollback")
	}
}
