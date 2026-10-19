package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/service"
)

// S3-F review C2: the renormalize decision was made only when the run's
// index was seeded; after a failed first checkpoint the seeded file stayed
// and later checkpoints skipped renormalization (the P3 bug again).
func TestCheckpoint_AFailedFirstCheckpointStillRenormalizesLater(t *testing.T) {
	ctx := context.Background()
	dir := initFilteredRepo(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	cp := service.NewCheckpointService(git.NewPool(1))

	// Another run creates the service's index directory.
	if err := cp.CreateCheckpoint(ctx, "run-other-0000", dir, "Bash", "call-0"); err != nil {
		t.Fatal(err)
	}
	indexes := runIndexes(t, tmp)
	if len(indexes) != 1 {
		t.Fatalf("private indexes = %v", indexes)
	}
	// A stale lock makes the run's first `git add` fail.
	lock := filepath.Join(filepath.Dir(indexes[0]), checkpointRunID+".index.lock")
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Bash", "call-1"); err == nil {
		t.Fatal("checkpoint with a locked index succeeded")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}

	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Bash", "call-1"); err != nil {
		t.Fatalf("checkpoint after the lock is gone: %v", err)
	}
	writeRepoFile(t, dir, "big.bin", "agent v2\n")
	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Bash", "call-2"); err != nil {
		t.Fatal(err)
	}
	if err := cp.RewindToFirst(ctx, checkpointRunID, dir); err != nil {
		t.Fatalf("RewindToFirst: %v", err)
	}
	if got := readRepoFile(t, dir, "big.bin"); got != "real v1\n" {
		t.Fatalf("big.bin after rollback = %q, want its content (not the clean form)", got)
	}
}
