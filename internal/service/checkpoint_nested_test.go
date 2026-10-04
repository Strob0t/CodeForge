package service_test

import (
	"context"
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

// Security review of the S3-F fix round: with a gitlink in the index whose
// directory holds a nested repository, `git add -A` / `add --renormalize`
// run git's submodule dirty check inside it - a child git that reads the
// nested repository's config and attributes (filter drivers). The Go Core
// does not run git on a workspace with a nested repository (KI-77).

// plantNestedRepo creates a nested repository sub/ in dir with a committed
// file; when staged, the outer index records it as a gitlink. Then the
// nested repository gets a clean filter that writes marker, and the file a
// same-size change (the dirty check must re-read it through the filter).
func plantNestedRepo(t *testing.T, dir string, staged bool) (marker string) {
	t.Helper()
	sub := filepath.Join(dir, "sub")
	gitRun(t, dir, "init", "-q", "sub")
	gitRun(t, sub, "config", "user.email", "n@example.invalid")
	gitRun(t, sub, "config", "user.name", "n")
	writeRepoFile(t, sub, "x.txt", "aaaa\n")
	gitRun(t, sub, "add", "x.txt")
	gitRun(t, sub, "commit", "-q", "-m", "nested")
	if staged {
		gitRun(t, dir, "add", "sub")
	}

	marker = filepath.Join(t.TempDir(), "marker")
	program := filepath.Join(t.TempDir(), "evil.sh")
	writeRepoFile(t, filepath.Dir(program), filepath.Base(program), "#!/bin/sh\necho ran >> "+marker+"\ncat\n")
	if err := os.Chmod(program, 0o755); err != nil { //nolint:gosec // test program
		t.Fatal(err)
	}
	gitRun(t, sub, "config", "filter.evil.clean", program)
	writeRepoFile(t, sub, ".gitattributes", "* filter=evil\n")
	writeRepoFile(t, sub, "x.txt", "bbbb\n")
	return marker
}

func assertNoMarker(t *testing.T, marker string) {
	t.Helper()
	if data, err := os.ReadFile(marker); err == nil { //nolint:gosec // test marker
		t.Fatalf("the nested repository's filter ran in the Go Core: %s", data)
	}
}

func assertNestedRefusal(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "nested repository at sub") {
		t.Fatalf("error = %v, want a refusal naming the nested repository at sub", err)
	}
}

func TestCheckpoint_RefusesNestedRepositories(t *testing.T) {
	for _, staged := range []bool{true, false} {
		name := "only in the working tree"
		if staged {
			name = "gitlink in the user's index"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			dir := initCheckpointTestRepo(t)
			marker := plantNestedRepo(t, dir, staged)
			cp := service.NewCheckpointService(git.NewPool(1))

			// Twice: a second checkpoint's `add -A` would find the gitlink
			// the first one added to the run's index.
			for _, call := range []string{"call-1", "call-2"} {
				assertNestedRefusal(t, cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Edit", call))
				assertNoMarker(t, marker)
			}
		})
	}
}

// The agent creates the nested repository during the run: delivery and
// rollback are refused, and nothing runs.
func TestDeliveryAndRewind_RefuseANestedRepositoryCreatedMidRun(t *testing.T) {
	for _, mode := range []run.DeliverMode{run.DeliverModeCommitLocal, run.DeliverModeBranch, run.DeliverModePatch} {
		t.Run(string(mode), func(t *testing.T) {
			ctx := context.Background()
			dir := initCheckpointTestRepo(t)
			pool := git.NewPool(1)
			cp := service.NewCheckpointService(pool)
			deliverer := service.NewDeliverService(&deliverMockStore{proj: &project.Project{ID: "proj-1", WorkspacePath: dir}},
				&config.Runtime{DeliveryCommitPrefix: "codeforge:"}, pool)
			if err := cp.CreateCheckpoint(ctx, checkpointRunID, dir, "Edit", "call-1"); err != nil {
				t.Fatal(err)
			}
			writeRepoFile(t, dir, "notes.txt", "run change\n")
			marker := plantNestedRepo(t, dir, false)

			_, err := deliverer.Deliver(ctx, &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: mode}, "task")
			assertNestedRefusal(t, err)
			assertNoMarker(t, marker)

			assertNestedRefusal(t, cp.RewindToFirst(ctx, checkpointRunID, dir))
			assertNoMarker(t, marker)
		})
	}
}
