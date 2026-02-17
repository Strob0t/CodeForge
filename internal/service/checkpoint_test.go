package service_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/service"
)

func initCheckpointTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v: %s: %v", args, out, err)
		}
	}

	// Create initial file and commit
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "initial commit")

	return dir
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
	return strings.TrimSpace(string(out))
}

func TestCheckpoint_CreateCreatesCommit(t *testing.T) {
	dir := initCheckpointTestRepo(t)
	ctx := context.Background()
	svc := service.NewCheckpointService()

	// Make a change
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content1"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := svc.CreateCheckpoint(ctx, "run-1", dir, "Edit", "call-1")
	if err != nil {
		t.Fatal(err)
	}

	// Verify commit exists
	log := gitRun(t, dir, "log", "--oneline", "-1")
	if !strings.Contains(log, "codeforge-checkpoint: call-1") {
		t.Fatalf("expected checkpoint commit, got: %s", log)
	}

	// Verify checkpoints list
	cps := svc.GetCheckpoints("run-1")
	if len(cps) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(cps))
	}
}

func TestCheckpoint_RewindToFirst(t *testing.T) {
	dir := initCheckpointTestRepo(t)
	ctx := context.Background()
	svc := service.NewCheckpointService()

	// Create two changes with checkpoints
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateCheckpoint(ctx, "run-1", dir, "Edit", "call-1"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateCheckpoint(ctx, "run-1", dir, "Edit", "call-2"); err != nil {
		t.Fatal(err)
	}

	// Both files should exist
	if _, err := os.Stat(filepath.Join(dir, "file1.txt")); err != nil {
		t.Fatal("file1 should exist before rewind")
	}

	// Rewind to before first checkpoint
	if err := svc.RewindToFirst(ctx, "run-1", dir); err != nil {
		t.Fatal(err)
	}

	// Both files should be gone
	if _, err := os.Stat(filepath.Join(dir, "file1.txt")); !os.IsNotExist(err) {
		t.Fatal("file1 should not exist after rewind to first")
	}
	if _, err := os.Stat(filepath.Join(dir, "file2.txt")); !os.IsNotExist(err) {
		t.Fatal("file2 should not exist after rewind to first")
	}
	// initial.txt should still exist
	if _, err := os.Stat(filepath.Join(dir, "initial.txt")); err != nil {
		t.Fatal("initial.txt should still exist")
	}
}

func TestCheckpoint_RewindToLast(t *testing.T) {
	dir := initCheckpointTestRepo(t)
	ctx := context.Background()
	svc := service.NewCheckpointService()

	// Create two changes
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateCheckpoint(ctx, "run-1", dir, "Edit", "call-1"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateCheckpoint(ctx, "run-1", dir, "Edit", "call-2"); err != nil {
		t.Fatal(err)
	}

	// Rewind to before last checkpoint only
	if err := svc.RewindToLast(ctx, "run-1", dir); err != nil {
		t.Fatal(err)
	}

	// file1 should still exist (first checkpoint), file2 should be gone
	if _, err := os.Stat(filepath.Join(dir, "file1.txt")); err != nil {
		t.Fatal("file1 should exist after rewind to last")
	}
	if _, err := os.Stat(filepath.Join(dir, "file2.txt")); !os.IsNotExist(err) {
		t.Fatal("file2 should not exist after rewind to last")
	}
}

func TestCheckpoint_CleanupKeepsWorkingState(t *testing.T) {
	dir := initCheckpointTestRepo(t)
	ctx := context.Background()
	svc := service.NewCheckpointService()

	// Create changes with checkpoints
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateCheckpoint(ctx, "run-1", dir, "Edit", "call-1"); err != nil {
		t.Fatal(err)
	}

	// Cleanup should remove shadow commits but keep files
	if err := svc.CleanupCheckpoints(ctx, "run-1", dir); err != nil {
		t.Fatal(err)
	}

	// file1 should still exist (soft reset keeps working tree)
	if _, err := os.Stat(filepath.Join(dir, "file1.txt")); err != nil {
		t.Fatal("file1 should exist after cleanup")
	}

	// No checkpoint commits in log
	log := gitRun(t, dir, "log", "--oneline")
	if strings.Contains(log, "codeforge-checkpoint") {
		t.Fatalf("checkpoint commits should be removed, got: %s", log)
	}

	// Checkpoints map should be cleared
	cps := svc.GetCheckpoints("run-1")
	if len(cps) != 0 {
		t.Fatalf("expected 0 checkpoints after cleanup, got %d", len(cps))
	}
}

func TestCheckpoint_GetCheckpointsOrdered(t *testing.T) {
	dir := initCheckpointTestRepo(t)
	ctx := context.Background()
	svc := service.NewCheckpointService()

	for i := 0; i < 3; i++ {
		fname := fmt.Sprintf("file%d.txt", i)
		if err := os.WriteFile(filepath.Join(dir, fname), []byte(fmt.Sprintf("v%d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := svc.CreateCheckpoint(ctx, "run-1", dir, "Edit", fmt.Sprintf("call-%d", i)); err != nil {
			t.Fatal(err)
		}
	}

	cps := svc.GetCheckpoints("run-1")
	if len(cps) != 3 {
		t.Fatalf("expected 3 checkpoints, got %d", len(cps))
	}

	for i, cp := range cps {
		expected := fmt.Sprintf("call-%d", i)
		if cp.CallID != expected {
			t.Fatalf("checkpoint %d: expected callID %s, got %s", i, expected, cp.CallID)
		}
	}
}
