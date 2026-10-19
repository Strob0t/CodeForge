package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/service"
)

// S3-F review C1: in a repository with filter attributes, renormalizing a
// private index seeded from the user's index failed ("unable to stat") when
// a tracked file was missing from the working tree - every first
// checkpoint after a pre-run deletion, every delivery of a run that
// deleted a file.

func removeRepoFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpoint_FilteredRepositoryWithAPreRunDeletion(t *testing.T) {
	ctx := context.Background()
	dir := initFilteredRepo(t)
	removeRepoFile(t, dir, "initial.txt") // the user's unstaged deletion
	cp := service.NewCheckpointService(git.NewPool(1))

	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Bash", "call-1"); err != nil {
		t.Fatalf("first checkpoint: %v", err)
	}
	writeRepoFile(t, dir, "big.bin", "agent v2\n")
	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Bash", "call-2"); err != nil {
		t.Fatalf("second checkpoint: %v", err)
	}
	if err := cp.RewindToFirst(ctx, checkpointRunID, dir); err != nil {
		t.Fatalf("RewindToFirst: %v", err)
	}
	if got := readRepoFile(t, dir, "big.bin"); got != "real v1\n" {
		t.Fatalf("big.bin after rollback = %q, want its content before the run", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "initial.txt")); !os.IsNotExist(err) {
		t.Fatalf("initial.txt after rollback: %v, want it still deleted (as before the run)", err)
	}
}

func TestDelivery_FilteredRepositoryWithDeletedFiles(t *testing.T) {
	newEnv := func(t *testing.T) (string, *service.CheckpointService, *service.DeliverService) {
		dir := initFilteredRepo(t)
		pool := git.NewPool(1)
		return dir, service.NewCheckpointService(pool), service.NewDeliverService(
			&deliverMockStore{proj: &project.Project{ID: "proj-1", WorkspacePath: dir}},
			&config.Runtime{DeliveryCommitPrefix: "codeforge:"}, pool)
	}
	deliver := func(t *testing.T, d *service.DeliverService) *service.DeliveryResult {
		t.Helper()
		result, err := d.Deliver(context.Background(), &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: run.DeliverModeCommitLocal}, "task")
		if err != nil {
			t.Fatalf("Deliver: %v", err)
		}
		return result
	}

	t.Run("the run deletes a file", func(t *testing.T) {
		dir, cp, d := newEnv(t)
		if err := cp.CreateCheckpoint(context.Background(), checkpointRunID, dir, "Bash", "call-1"); err != nil {
			t.Fatal(err)
		}
		removeRepoFile(t, dir, "initial.txt")
		result := deliver(t, d)
		if files := gitRun(t, dir, "diff-tree", "--no-commit-id", "--name-status", "-r", result.CommitHash); files != "D\tinitial.txt" {
			t.Fatalf("delivered change = %q, want the deletion of initial.txt", files)
		}
	})

	t.Run("a pre-run deletion stays the user's", func(t *testing.T) {
		dir, cp, d := newEnv(t)
		removeRepoFile(t, dir, "initial.txt")
		if err := cp.CreateCheckpoint(context.Background(), checkpointRunID, dir, "Bash", "call-1"); err != nil {
			t.Fatal(err)
		}
		writeRepoFile(t, dir, "notes.txt", "run change\n")
		result := deliver(t, d)
		if files := gitRun(t, dir, "diff-tree", "--no-commit-id", "--name-status", "-r", result.CommitHash); files != "A\tnotes.txt" {
			t.Fatalf("delivered change = %q, want only the run's notes.txt", files)
		}
		if _, err := os.Stat(filepath.Join(dir, "initial.txt")); !os.IsNotExist(err) {
			t.Fatal("the user's deletion was undone")
		}
	})
}
