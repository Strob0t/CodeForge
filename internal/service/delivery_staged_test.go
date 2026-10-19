package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/service"
)

// S3-F review C6: commit delivery set the user's index entry of every path
// it committed to the commit's content. A change the user had staged
// before the run, in a file the run also edited, was unstaged by that.

// stagedFixture commits doc.txt, stages a change of its first line and
// takes the run's first checkpoint; worktree is the file's content in the
// working tree before the run.
func stagedFixture(t *testing.T, worktree string) (dir string, d *service.DeliverService) {
	t.Helper()
	dir = initCheckpointTestRepo(t)
	writeRepoFile(t, dir, "doc.txt", "l1\nl2\nl3\nl4\nl5\nl6\n")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "doc")
	writeRepoFile(t, dir, "doc.txt", "staged1\nl2\nl3\nl4\nl5\nl6\n")
	gitRun(t, dir, "add", "doc.txt")
	writeRepoFile(t, dir, "doc.txt", worktree)

	pool := git.NewPool(1)
	cp := service.NewCheckpointService(pool)
	d = service.NewDeliverService(&deliverMockStore{proj: &project.Project{ID: "proj-1", WorkspacePath: dir}},
		&config.Runtime{DeliveryCommitPrefix: "codeforge:"}, pool)
	if err := cp.CreateCheckpoint(context.Background(), checkpointRunID, dir, "Edit", "call-1"); err != nil {
		t.Fatal(err)
	}
	return dir, d
}

func deliverCommitLocal(t *testing.T, d *service.DeliverService) *service.DeliveryResult {
	t.Helper()
	result, err := d.Deliver(context.Background(), &run.Run{ID: checkpointRunID, ProjectID: "proj-1", DeliverMode: run.DeliverModeCommitLocal}, "task")
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	return result
}

func TestDelivery_KeepsTheUsersStagedChangeInAFileTheRunEdited(t *testing.T) {
	dir, d := stagedFixture(t, "staged1\nl2\nl3\nl4\nl5\nl6\n")
	writeRepoFile(t, dir, "doc.txt", "staged1\nl2\nl3\nl4\nl5\nrun6\n") // the run's change

	result := deliverCommitLocal(t, d)
	if got := gitRun(t, dir, "show", result.CommitHash+":doc.txt"); got != "l1\nl2\nl3\nl4\nl5\nrun6" {
		t.Fatalf("delivered doc.txt = %q, want only the run's change", got)
	}
	if got := gitRun(t, dir, "show", ":doc.txt"); got != "staged1\nl2\nl3\nl4\nl5\nrun6" {
		t.Fatalf("index doc.txt = %q, want the user's staged change on top of the delivered commit", got)
	}
	if staged := gitRun(t, dir, "diff", "--cached", "--name-only"); staged != "doc.txt" {
		t.Fatalf("staged files = %q, want doc.txt still staged", staged)
	}
	if unstaged := gitRun(t, dir, "diff", "--name-only"); unstaged != "" {
		t.Fatalf("unstaged files = %q, want none", unstaged)
	}
}

// When the staged change cannot be merged with the delivered content, the
// index entry takes the delivered content (as before) - a warning names the
// path.
func TestDelivery_AnUnmergeableStagedChangeFallsBackToTheCommit(t *testing.T) {
	// The user staged line 1, then reverted the working tree to HEAD's
	// content; the run changes line 2, next to the staged change.
	dir, d := stagedFixture(t, "l1\nl2\nl3\nl4\nl5\nl6\n")
	writeRepoFile(t, dir, "doc.txt", "l1\nrun2\nl3\nl4\nl5\nl6\n")

	result := deliverCommitLocal(t, d)
	if got := gitRun(t, dir, "show", ":doc.txt"); got != gitRun(t, dir, "show", result.CommitHash+":doc.txt") {
		t.Fatalf("index doc.txt = %q, want the delivered content", got)
	}
}
