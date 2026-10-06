package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	bp "github.com/Strob0t/CodeForge/internal/domain/branchprotection"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/service"
)

// protectedDeliverStore is a delivery store with branch protection rules.
type protectedDeliverStore struct {
	deliverMockStore
	rules   []bp.ProtectionRule
	listErr error
}

func (m *protectedDeliverStore) ListBranchProtectionRules(_ context.Context, projectID string) ([]bp.ProtectionRule, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var rules []bp.ProtectionRule
	for i := range m.rules {
		if m.rules[i].ProjectID == projectID {
			rules = append(rules, m.rules[i])
		}
	}
	return rules, nil
}

// fakeGateProfiles serves policy profiles by name.
type fakeGateProfiles map[string]policy.PolicyProfile

func (f fakeGateProfiles) GetProfile(_ context.Context, name string) (policy.PolicyProfile, bool) {
	p, ok := f[name]
	return p, ok
}

// protectedWorkspace is a workspace on branch main (or detached when branch
// is "") with a run's checkpoint and a change after it.
func protectedWorkspace(t *testing.T, branch, runID string) (dir, before string) {
	t.Helper()
	dir = initDeliverTestRepo(t)
	runGit(t, dir, "branch", "-M", "main")
	switch branch {
	case "main":
	case "":
		runGit(t, dir, "checkout", "-q", "--detach")
	default:
		runGit(t, dir, "checkout", "-q", "-b", branch)
	}
	before = runGit(t, dir, "rev-parse", "HEAD")
	checkpointBeforeChange(t, dir, runID)
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("protected"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, before
}

func protectionRule(pattern string, edit func(*bp.ProtectionRule)) bp.ProtectionRule {
	r := bp.ProtectionRule{ID: "rule-" + pattern, ProjectID: "proj-1", BranchPattern: pattern, Enabled: true}
	if edit != nil {
		edit(&r)
	}
	return r
}

func withReviews(r *bp.ProtectionRule)  { r.RequireReviews = true }
func withTests(r *bp.ProtectionRule)    { r.RequireTests = true }
func ruleDisabled(r *bp.ProtectionRule) { r.Enabled = false }

// KI-205: the project's branch protection rules are evaluated on delivery.
// Commit-local delivery moves the checked-out branch with an unreviewed
// commit (a merge without review into that branch); its tests and lint
// passed only if the run's quality gate required them. Branch and PR
// delivery push the branch codeforge/<run>. With enabled rules, a branch no
// rule matches is refused (default deny, P1-4).
func TestDeliver_BranchProtection(t *testing.T) {
	const runID = "run-abcd1234"
	gated := fakeGateProfiles{
		"gated": {Name: "gated", QualityGate: policy.QualityGate{RequireTestsPass: true}},
		"plain": {Name: "plain"},
	}
	tests := []struct {
		name      string
		mode      run.DeliverMode
		branch    string // checked-out branch, "" for a detached HEAD
		rules     []bp.ProtectionRule
		listErr   error
		profile   string
		profiles  fakeGateProfiles
		wantError string // "" when the delivery goes ahead
	}{
		{name: "commit-local without rules", mode: run.DeliverModeCommitLocal, branch: "main"},
		{name: "commit-local with disabled rules", mode: run.DeliverModeCommitLocal, branch: "main",
			rules: []bp.ProtectionRule{protectionRule("main", func(r *bp.ProtectionRule) { withReviews(r); ruleDisabled(r) })}},
		{name: "commit-local onto a branch that requires reviews", mode: run.DeliverModeCommitLocal, branch: "main",
			rules:     []bp.ProtectionRule{protectionRule("main", withReviews)},
			wantError: "at least one review is required"},
		{name: "commit-local onto a branch without requirements", mode: run.DeliverModeCommitLocal, branch: "main",
			rules: []bp.ProtectionRule{protectionRule("main", nil)}},
		{name: "commit-local onto a branch no rule matches", mode: run.DeliverModeCommitLocal, branch: "feature",
			rules:     []bp.ProtectionRule{protectionRule("main", nil)},
			wantError: "default deny"},
		{name: "commit-local on a detached HEAD moves no branch", mode: run.DeliverModeCommitLocal, branch: "",
			rules: []bp.ProtectionRule{protectionRule("main", withReviews)}},
		{name: "commit-local requiring tests, run without a test gate", mode: run.DeliverModeCommitLocal, branch: "main",
			rules: []bp.ProtectionRule{protectionRule("main", withTests)}, profile: "plain", profiles: gated,
			wantError: "tests must pass"},
		{name: "commit-local requiring tests, run passed its test gate", mode: run.DeliverModeCommitLocal, branch: "main",
			rules: []bp.ProtectionRule{protectionRule("main", withTests)}, profile: "gated", profiles: gated},
		{name: "commit-local requiring tests, no profiles known", mode: run.DeliverModeCommitLocal, branch: "main",
			rules: []bp.ProtectionRule{protectionRule("main", withTests)}, profile: "gated",
			wantError: "tests must pass"},
		{name: "commit-local when the rules cannot be read", mode: run.DeliverModeCommitLocal, branch: "main",
			listErr: errors.New("db down"), wantError: "db down"},
		{name: "branch push a rule allows", mode: run.DeliverModeBranch, branch: "main",
			rules: []bp.ProtectionRule{protectionRule("main", withReviews), protectionRule("codeforge/*", nil)}},
		{name: "branch push no rule matches", mode: run.DeliverModeBranch, branch: "main",
			rules:     []bp.ProtectionRule{protectionRule("main", withReviews)},
			wantError: "default deny"},
		{name: "branch push when the rules cannot be read", mode: run.DeliverModeBranch, branch: "main",
			listErr: errors.New("db down"), wantError: "db down"},
		{name: "pr push no rule matches", mode: run.DeliverModePR, branch: "main",
			rules:     []bp.ProtectionRule{protectionRule("release/*", nil)},
			wantError: "default deny"},
		{name: "patch changes no branch", mode: run.DeliverModePatch, branch: "main",
			rules: []bp.ProtectionRule{protectionRule("release/*", nil)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, before := protectedWorkspace(t, tt.branch, runID)
			proj := project.Project{ID: "proj-1", WorkspacePath: dir, RepoURL: "https://github.com/acme/app",
				Provider: "github-api", Config: map[string]string{"token": "t"}}
			store := &protectedDeliverStore{deliverMockStore: deliverMockStore{proj: &proj}, rules: tt.rules, listErr: tt.listErr}
			svc := service.NewDeliverService(store, &config.Runtime{DeliveryCommitPrefix: "codeforge:"}, git.NewPool(5))
			fake := &fakePullRequests{err: errors.New("must not be called")}
			svc.SetPullRequestProvider(fake.build)
			if tt.profiles != nil {
				svc.SetPolicyProfiles(tt.profiles)
			}
			r := &run.Run{ID: runID, ProjectID: "proj-1", DeliverMode: tt.mode, PolicyProfile: tt.profile}

			result, err := svc.Deliver(context.Background(), r, "protected work")

			branches := runGit(t, dir, "branch", "--list", "codeforge/*")
			head := runGit(t, dir, "rev-parse", "HEAD")
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("Deliver() = %+v, %v; want an error containing %q", result, err, tt.wantError)
				}
				if tt.listErr == nil && !errors.Is(err, service.ErrBranchProtected) {
					t.Errorf("error %v is not ErrBranchProtected", err)
				}
				if head != before || branches != "" || fake.pr != nil {
					t.Fatalf("refused delivery changed the repository: HEAD %s (was %s), branches %q, pull request %+v", head, before, branches, fake.pr)
				}
				if got := runGit(t, dir, "status", "--porcelain"); !strings.Contains(got, "hello.txt") {
					t.Fatalf("refused delivery lost the run's change: status %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Deliver() error = %v", err)
			}
			switch tt.mode {
			case run.DeliverModeCommitLocal:
				if head == before || result.CommitHash != head {
					t.Fatalf("commit-local did not move HEAD: HEAD %s, before %s, result %+v", head, before, result)
				}
			case run.DeliverModeBranch:
				if result.BranchName != "codeforge/run-abcd" || branches == "" {
					t.Fatalf("branch delivery: result %+v, branches %q", result, branches)
				}
			case run.DeliverModePatch:
				if result.PatchPath == "" || head != before {
					t.Fatalf("patch delivery: result %+v, HEAD %s (was %s)", result, head, before)
				}
			}
		})
	}
}
