package service_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/service"
)

// Security review P3: the Go Core runs git without filter programs
// (git-lfs, git-crypt). The user's git stored filtered files in their clean
// form (an LFS pointer, ciphertext), the working tree holds the smudged
// content. Rollback must restore that content, not the clean form, and
// commit delivery must not commit filtered files in the wrong form (the
// plaintext of a git-crypt file, an LFS object as a plain blob).

// initFilteredRepo is a repository whose user's git (global config, like
// `git lfs install`) cleans *.bin to a constant pointer; the Go Core's git
// does not see that config.
func initFilteredRepo(t *testing.T) string {
	t.Helper()
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, []byte("[filter \"fake\"]\n\tclean = echo POINTER\n\tsmudge = cat\n\trequired = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	dir := initCheckpointTestRepo(t)
	writeRepoFile(t, dir, ".gitattributes", "*.bin filter=fake\n")
	writeRepoFile(t, dir, "big.bin", "real v1\n")
	// Older than the index, so git trusts the index entry (no racy re-hash).
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "big.bin"), old, old); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "filtered file")
	if got := gitRun(t, dir, "cat-file", "-p", "HEAD:big.bin"); got != "POINTER" {
		t.Fatalf("fixture: HEAD:big.bin = %q, want the clean form", got)
	}
	return dir
}

func TestRewindToFirst_RestoresTheContentOfFilteredFiles(t *testing.T) {
	ctx := context.Background()
	dir := initFilteredRepo(t)
	cp := service.NewCheckpointService(git.NewPool(1))

	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Bash", "call-1"); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, dir, "big.bin", "agent v2\n")
	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Bash", "call-2"); err != nil {
		t.Fatal(err)
	}

	if err := cp.RewindToFirst(ctx, checkpointRunID, dir); err != nil {
		t.Fatalf("RewindToFirst: %v", err)
	}
	if got := readRepoFile(t, dir, "big.bin"); got != "real v1\n" {
		t.Fatalf("big.bin after rollback = %q, want the content before the run (not its clean form)", got)
	}
}

func TestCommitDelivery_RefusesChangedFilteredFiles(t *testing.T) {
	for _, mode := range []run.DeliverMode{run.DeliverModeCommitLocal, run.DeliverModeBranch, run.DeliverModePR} {
		t.Run(string(mode), func(t *testing.T) {
			ctx := context.Background()
			dir := initFilteredRepo(t)
			head := gitRun(t, dir, "rev-parse", "HEAD")
			pool := git.NewPool(1)
			cp := service.NewCheckpointService(pool)
			deliverer := service.NewDeliverService(&deliverMockStore{proj: &project.Project{ID: "proj-1", WorkspacePath: dir}},
				&config.Runtime{DeliveryCommitPrefix: "codeforge:"}, pool)
			if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Bash", "call-1"); err != nil {
				t.Fatal(err)
			}
			writeRepoFile(t, dir, "big.bin", "agent v2\n")
			writeRepoFile(t, dir, "notes.txt", "plain change\n")

			_, err := deliverer.Deliver(ctx, &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: mode}, "task")
			if err == nil || !strings.Contains(err.Error(), "big.bin") || !strings.Contains(err.Error(), "filter") {
				t.Fatalf("Deliver = %v, want a refusal naming big.bin and its filter", err)
			}
			if got := gitRun(t, dir, "rev-parse", "HEAD"); got != head {
				t.Fatalf("HEAD moved to %s", got)
			}
		})
	}
}

func TestDelivery_UnfilteredChangeInAFilteredRepository(t *testing.T) {
	ctx := context.Background()
	dir := initFilteredRepo(t)
	pool := git.NewPool(1)
	cp := service.NewCheckpointService(pool)
	deliverer := service.NewDeliverService(&deliverMockStore{proj: &project.Project{ID: "proj-1", WorkspacePath: dir}},
		&config.Runtime{DeliveryCommitPrefix: "codeforge:"}, pool)
	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Write", "call-1"); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, dir, "notes.txt", "plain change\n")

	result, err := deliverer.Deliver(ctx, &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: run.DeliverModeCommitLocal}, "task")
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if files := gitRun(t, dir, "diff-tree", "--no-commit-id", "--name-only", "-r", result.CommitHash); files != "notes.txt" {
		t.Fatalf("delivered files = %q, want only notes.txt", files)
	}
	if got := gitRun(t, dir, "cat-file", "-p", result.CommitHash+":big.bin"); got != "POINTER" {
		t.Fatalf("big.bin in the delivered commit = %q, want its stored (clean) form kept", got)
	}

	// A patch of a filtered file holds its content, which the user's git
	// cleans again when the patch is applied and added.
	if err := cp.CleanupCheckpoints(ctx, checkpointRunID, dir); err != nil {
		t.Fatal(err)
	}
	if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Bash", "call-2"); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, dir, "big.bin", "agent v2\n")
	patch, err := deliverer.Deliver(ctx, &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: run.DeliverModePatch}, "task")
	if err != nil {
		t.Fatalf("patch delivery: %v", err)
	}
	data := readFileAt(t, patch.PatchPath)
	if !strings.Contains(data, "-real v1") || !strings.Contains(data, "+agent v2") {
		t.Fatalf("patch of big.bin = %q, want its content change", data)
	}
}
