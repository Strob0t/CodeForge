package service_test

import (
	"context"
	"errors"
	"fmt"
	"net/http/cgi" //nolint:gosec // G504: serves git http-backend to the test only (Go >= 1.6.3 is not affected)
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// gitHTTPServer serves the bare repositories below its root over git's
// smart HTTP protocol (git http-backend), pushes included: a remote the
// hardened workspace git may push to (local paths are refused).
func gitHTTPServer(t *testing.T) (root, baseURL string) {
	t.Helper()
	execPath, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		t.Fatalf("git --exec-path: %v", err)
	}
	backend := filepath.Join(strings.TrimSpace(string(execPath)), "git-http-backend")
	if _, err := os.Stat(backend); err != nil {
		t.Skipf("git-http-backend not available: %v", err)
	}
	root = t.TempDir()
	srv := httptest.NewServer(&cgi.Handler{
		Path:       backend,
		Env:        []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1"},
		InheritEnv: []string{"PATH"},
	})
	t.Cleanup(srv.Close)
	return root, srv.URL
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
	return strings.TrimSpace(string(out))
}

// prWorkspace is a workspace whose origin is a fresh bare repository of the
// git HTTP server, with a run's checkpoint and a change after it.
func prWorkspace(t *testing.T, root, baseURL, runID string) (dir, bare string) {
	t.Helper()
	bare = filepath.Join(root, "app.git")
	runGit(t, root, "init", "-q", "--bare", bare)
	runGit(t, bare, "config", "http.receivepack", "true")
	dir = initDeliverTestRepo(t)
	runGit(t, dir, "remote", "add", "origin", baseURL+"/app.git")
	checkpointBeforeChange(t, dir, runID)
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("pull request"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, bare
}

// fakePullRequests records the provider configs and pull requests it gets.
type fakePullRequests struct {
	provider string
	cfg      map[string]string
	pr       *gitprovider.PullRequest
	err      error
}

func (f *fakePullRequests) build(name string, cfg map[string]string) (gitprovider.PullRequestCreator, error) {
	f.provider, f.cfg = name, cfg
	return f, nil
}

func (f *fakePullRequests) CreatePullRequest(_ context.Context, pr *gitprovider.PullRequest) (string, error) {
	f.pr = pr
	if f.err != nil {
		return "", f.err
	}
	return "https://github.com/" + pr.Repo + "/pull/7", nil
}

// KI-117: PR delivery pushes the run's branch and opens the pull request
// through the provider's REST API (no gh): with a github-api project's own
// token, or with the operator's github.token for a github.com repository in
// the default tenant only (KI-85). Without a pull request the delivery
// stays a branch delivery and says why.
func TestDeliver_PROpensPullRequestThroughProvider(t *testing.T) {
	const runID = "run-abcd1234"
	tests := []struct {
		name          string
		tenant        string
		proj          project.Project
		operatorToken string
		createErr     error
		wantToken     string // "" when no pull request is opened
		wantBaseURL   string
		wantPRError   string
	}{
		{
			name:      "project token",
			tenant:    "tenant-b",
			proj:      project.Project{RepoURL: "https://github.com/acme/app.git", Provider: "github-api", Config: map[string]string{"token": "ghp_project"}},
			wantToken: "ghp_project",
		},
		{
			name:        "project token and enterprise API",
			tenant:      "tenant-b",
			proj:        project.Project{RepoURL: "https://ghe.example.com/acme/app", Provider: "github-api", Config: map[string]string{"token": "ghp_project", "base_url": "https://ghe.example.com/api/v3"}},
			wantToken:   "ghp_project",
			wantBaseURL: "https://ghe.example.com/api/v3",
		},
		{
			name:          "operator token in the default tenant",
			tenant:        tenantctx.DefaultTenantID,
			proj:          project.Project{RepoURL: "git@github.com:acme/app.git", Provider: "github"},
			operatorToken: "ghp_operator",
			wantToken:     "ghp_operator",
		},
		{
			name:          "operator token not for another tenant",
			tenant:        "tenant-b",
			proj:          project.Project{RepoURL: "https://github.com/acme/app", Provider: "github"},
			operatorToken: "ghp_operator",
			wantPRError:   "serves only the default tenant",
		},
		{
			name:        "no token",
			tenant:      tenantctx.DefaultTenantID,
			proj:        project.Project{RepoURL: "https://github.com/acme/app", Provider: "github"},
			wantPRError: "no GitHub token",
		},
		{
			name:          "operator token only for github.com",
			tenant:        tenantctx.DefaultTenantID,
			proj:          project.Project{RepoURL: "https://gitlab.com/acme/app", Provider: "gitlab"},
			operatorToken: "ghp_operator",
			wantPRError:   "gitlab.com",
		},
		{
			name:          "the API refuses",
			tenant:        tenantctx.DefaultTenantID,
			proj:          project.Project{RepoURL: "https://github.com/acme/app", Provider: "github"},
			operatorToken: "ghp_operator",
			createErr:     fmt.Errorf("GitHub API answered 422 (A pull request already exists): %w", domain.ErrValidation),
			wantToken:     "ghp_operator",
			wantPRError:   "A pull request already exists",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, baseURL := gitHTTPServer(t)
			dir, bare := prWorkspace(t, root, baseURL, runID)
			proj := tt.proj
			proj.ID, proj.WorkspacePath = "proj-1", dir
			svc := service.NewDeliverService(&deliverMockStore{proj: &proj}, &config.Runtime{DeliveryCommitPrefix: "codeforge:"}, git.NewPool(5))
			svc.SetOperatorGitHubToken(tt.operatorToken)
			fake := &fakePullRequests{err: tt.createErr}
			svc.SetPullRequestProvider(fake.build)
			// The branch goes to the project's repository (KI-188), served
			// here by the local git server.
			var pushedFor string
			svc.SetPushURL(func(repoURL string) string { pushedFor = repoURL; return baseURL + "/app.git" })

			ctx := tenantctx.WithTenant(context.Background(), tt.tenant)
			result, err := svc.Deliver(ctx, &run.Run{ID: runID, ProjectID: "proj-1", DeliverMode: run.DeliverModePR}, "add feature")
			if err != nil {
				t.Fatalf("Deliver: %v", err)
			}
			if result.PushError != "" {
				t.Fatalf("push failed: %s", result.PushError)
			}
			if pushedFor != proj.RepoURL {
				t.Fatalf("pushed to the URL for %q, want the project's repository URL %q", pushedFor, proj.RepoURL)
			}
			if pushed := runGit(t, bare, "rev-parse", "refs/heads/codeforge/run-abcd"); pushed != result.CommitHash {
				t.Fatalf("pushed %s, delivered %s", pushed, result.CommitHash)
			}

			if tt.wantToken == "" {
				if fake.pr != nil {
					t.Fatalf("a pull request was opened: %+v", fake.pr)
				}
			} else {
				if fake.provider != "github-api" || fake.cfg["token"] != tt.wantToken || fake.cfg["base_url"] != tt.wantBaseURL {
					t.Fatalf("provider %q config %v, want github-api with token %q, base_url %q", fake.provider, fake.cfg, tt.wantToken, tt.wantBaseURL)
				}
				want := gitprovider.PullRequest{Repo: "acme/app", Head: "codeforge/run-abcd", Title: "codeforge: add feature", Body: "Automated delivery from CodeForge run " + runID}
				if *fake.pr != want {
					t.Fatalf("pull request %+v, want %+v", *fake.pr, want)
				}
			}

			if tt.wantPRError != "" {
				if result.Mode != run.DeliverModeBranch || result.PRURL != "" || !strings.Contains(result.PRError, tt.wantPRError) {
					t.Fatalf("result %+v, want a branch delivery whose PR error mentions %q", result, tt.wantPRError)
				}
				if strings.Contains(result.PRError, "ghp_") {
					t.Fatalf("PR error %q names a token", result.PRError)
				}
				return
			}
			if result.Mode != run.DeliverModePR || result.PRURL != "https://github.com/acme/app/pull/7" || result.PRError != "" {
				t.Fatalf("result %+v", result)
			}
		})
	}
}

// A failed push leaves no branch to open a pull request from.
func TestDeliver_PRSkippedWhenPushFails(t *testing.T) {
	_, baseURL := gitHTTPServer(t)
	dir := initDeliverTestRepo(t)
	checkpointBeforeChange(t, dir, "run-abcd1234")
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("no remote"), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := project.Project{ID: "proj-1", WorkspacePath: dir, RepoURL: "https://github.com/acme/app", Provider: "github-api", Config: map[string]string{"token": "t"}}
	svc := service.NewDeliverService(&deliverMockStore{proj: &proj}, &config.Runtime{}, git.NewPool(5))
	fake := &fakePullRequests{err: errors.New("must not be called")}
	svc.SetPullRequestProvider(fake.build)
	svc.SetPushURL(func(string) string { return baseURL + "/missing.git" }) // no such repository

	result, err := svc.Deliver(context.Background(), &run.Run{ID: "run-abcd1234", ProjectID: "proj-1", DeliverMode: run.DeliverModePR}, "t")
	if err != nil {
		t.Fatal(err)
	}
	if result.PushError == "" || fake.pr != nil || result.Mode != run.DeliverModeBranch {
		t.Fatalf("result %+v, pull request %+v", result, fake.pr)
	}
}
