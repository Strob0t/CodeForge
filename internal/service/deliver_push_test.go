package service_test

import (
	"context"
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

// KI-188 (R6-2): branch and PR delivery pushed with `git push -u origin
// <branch>`, which follows the agent-writable remote config: a push refspec
// force-overwrote the remote main, a pushurl sent the branch (with the
// operator's credentials) to another repository. Delivery now pushes to the
// project's repository URL with a full refspec.

// refIn returns the commit of ref in the bare repository, "" if none.
func refIn(t *testing.T, bare, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--verify", "-q", ref)
	cmd.Dir = bare
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

// bareRepo creates a bare repository below the git HTTP server's root that
// accepts pushes.
func bareRepo(t *testing.T, root, name string) string {
	t.Helper()
	bare := filepath.Join(root, name)
	runGit(t, root, "init", "-q", "--bare", bare)
	runGit(t, bare, "config", "http.receivepack", "true")
	return bare
}

func TestDeliver_BranchPushesOnlyToTheProjectsRepository(t *testing.T) {
	const runID = "run-abcd1234"
	const branch = "refs/heads/codeforge/run-abcd"
	tests := []struct {
		name   string
		config func(baseURL string) [][2]string
	}{
		{"push refspec onto main (the reviewer's reproduction)", func(string) [][2]string {
			return [][2]string{{"remote.origin.push", "+refs/heads/codeforge/run-abcd:refs/heads/main"}}
		}},
		{"pushurl to another repository", func(baseURL string) [][2]string {
			return [][2]string{{"remote.origin.pushurl", baseURL + "/other.git"}}
		}},
		{"origin moved to another repository", func(baseURL string) [][2]string {
			return [][2]string{{"remote.origin.url", baseURL + "/other.git"}}
		}},
		{"no agent config", func(string) [][2]string { return nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, baseURL := gitHTTPServer(t)
			victim := bareRepo(t, root, "victim.git")
			other := bareRepo(t, root, "other.git")
			dir := initDeliverTestRepo(t)
			runGit(t, dir, "remote", "add", "origin", baseURL+"/victim.git")
			runGit(t, dir, "push", "-q", "origin", "HEAD:refs/heads/main")
			mainBefore := refIn(t, victim, "refs/heads/main")
			checkpointBeforeChange(t, dir, runID)
			if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("delivered"), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, kv := range tt.config(baseURL) {
				if kv[0] == "remote.origin.url" {
					runGit(t, dir, "config", kv[0], kv[1])
				} else {
					runGit(t, dir, "config", "--add", kv[0], kv[1])
				}
			}

			proj := project.Project{ID: "proj-1", WorkspacePath: dir, RepoURL: baseURL + "/victim.git"}
			svc := service.NewDeliverService(&deliverMockStore{proj: &proj}, &config.Runtime{DeliveryCommitPrefix: "codeforge:"}, git.NewPool(1))
			result, err := svc.Deliver(context.Background(), &run.Run{ID: runID, ProjectID: "proj-1", DeliverMode: run.DeliverModeBranch}, "fix")
			if err != nil {
				t.Fatalf("Deliver: %v", err)
			}
			if result.PushError != "" {
				t.Fatalf("push failed: %s", result.PushError)
			}
			if got := refIn(t, victim, branch); got != result.CommitHash {
				t.Fatalf("project repository %s = %q, want the delivered commit %s", branch, got, result.CommitHash)
			}
			if got := refIn(t, victim, "refs/heads/main"); got != mainBefore {
				t.Fatalf("project repository main moved from %s to %s", mainBefore, got)
			}
			for _, ref := range []string{branch, "refs/heads/main"} {
				if got := refIn(t, other, ref); got != "" {
					t.Fatalf("the other repository got %s = %s", ref, got)
				}
			}
		})
	}
}

// A project without a repository URL has nowhere to push to: the delivery
// says so instead of pushing where the workspace's config points.
func TestDeliver_BranchWithoutAProjectURLPushesNowhere(t *testing.T) {
	const runID = "run-abcd1234"
	root, baseURL := gitHTTPServer(t)
	victim := bareRepo(t, root, "victim.git")
	dir := initDeliverTestRepo(t)
	runGit(t, dir, "remote", "add", "origin", baseURL+"/victim.git")
	checkpointBeforeChange(t, dir, runID)
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("delivered"), 0o644); err != nil {
		t.Fatal(err)
	}

	proj := project.Project{ID: "proj-1", WorkspacePath: dir}
	svc := service.NewDeliverService(&deliverMockStore{proj: &proj}, &config.Runtime{}, git.NewPool(1))
	result, err := svc.Deliver(context.Background(), &run.Run{ID: runID, ProjectID: "proj-1", DeliverMode: run.DeliverModeBranch}, "fix")
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if !strings.Contains(result.PushError, "repository URL") {
		t.Fatalf("push error %q, want one naming the missing repository URL", result.PushError)
	}
	if result.CommitHash == "" || result.BranchName != "codeforge/run-abcd" {
		t.Fatalf("result %+v, want the local branch and commit", result)
	}
	if got := refIn(t, victim, "refs/heads/codeforge/run-abcd"); got != "" {
		t.Fatalf("pushed to the workspace's origin: %s", got)
	}
}
