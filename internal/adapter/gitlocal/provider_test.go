package gitlocal_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/Strob0t/CodeForge/internal/adapter/gitlocal"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

func TestGitLocal_Registration(t *testing.T) {
	p, err := gitprovider.New("local", nil)
	if err != nil {
		t.Fatalf("expected local provider to be registered: %v", err)
	}
	if p.Name() != "local" {
		t.Fatalf("expected name 'local', got %q", p.Name())
	}
	caps := p.Capabilities()
	if !caps.Clone {
		t.Fatal("expected Clone capability")
	}
}

func TestCloneAndStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in test environment")
	}

	ctx := context.Background()

	// Create a source repo with one commit, then use it as source for clone
	srcDir := initTestRepo(t)

	// Now test the provider
	p, err := gitprovider.New("local", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Clone to a new directory
	cloneDir := filepath.Join(t.TempDir(), "cloned")
	if err := p.Clone(ctx, srcDir, cloneDir); err != nil {
		t.Fatalf("Clone failed: %v", err)
	}

	// Status on cloned repo
	status, err := p.Status(ctx, cloneDir)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status.Branch != "master" && status.Branch != "main" {
		t.Fatalf("expected branch master or main, got %q", status.Branch)
	}
	if status.CommitHash == "" {
		t.Fatal("expected non-empty commit hash")
	}
	if status.CommitMessage != "initial commit" {
		t.Fatalf("expected commit message 'initial commit', got %q", status.CommitMessage)
	}
	if status.Dirty {
		t.Fatal("expected clean repo")
	}
}

func TestCloneIdempotent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in test environment")
	}

	ctx := context.Background()
	srcDir := initTestRepo(t)

	p, err := gitprovider.New("local", nil)
	if err != nil {
		t.Fatal(err)
	}

	// First clone
	cloneDir := filepath.Join(t.TempDir(), "cloned")
	if err := p.Clone(ctx, srcDir, cloneDir); err != nil {
		t.Fatalf("first Clone failed: %v", err)
	}

	// Second clone to same destination should succeed (re-clone path)
	if err := p.Clone(ctx, srcDir, cloneDir); err != nil {
		t.Fatalf("second Clone (re-clone) failed: %v", err)
	}

	// Verify the repo is still valid after re-clone
	status, err := p.Status(ctx, cloneDir)
	if err != nil {
		t.Fatalf("Status after re-clone failed: %v", err)
	}
	if status.CommitHash == "" {
		t.Fatal("expected non-empty commit hash after re-clone")
	}
}

func TestListBranches(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in test environment")
	}

	ctx := context.Background()
	dir := initTestRepo(t)

	p, err := gitprovider.New("local", nil)
	if err != nil {
		t.Fatal(err)
	}

	branches, err := p.ListBranches(ctx, dir)
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}
	if len(branches) == 0 {
		t.Fatal("expected at least one branch")
	}

	foundCurrent := false
	for _, b := range branches {
		if b.Current {
			foundCurrent = true
		}
	}
	if !foundCurrent {
		t.Fatal("expected one branch marked as current")
	}
}

func TestCheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in test environment")
	}

	ctx := context.Background()
	dir := initTestRepo(t)

	p, err := gitprovider.New("local", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a new branch
	runGitCmd(t, dir, "branch", "feature-x")

	// Checkout the new branch
	if err := p.Checkout(ctx, dir, "feature-x"); err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}

	// Verify we're on the new branch
	status, err := p.Status(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if status.Branch != "feature-x" {
		t.Fatalf("expected branch 'feature-x', got %q", status.Branch)
	}
}

func TestDirtyStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in test environment")
	}

	ctx := context.Background()
	dir := initTestRepo(t)

	p, err := gitprovider.New("local", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create an untracked file
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := p.Status(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Dirty {
		t.Fatal("expected dirty status")
	}
	if len(status.Untracked) == 0 {
		t.Fatal("expected untracked files")
	}

	// Modify tracked file
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err = p.Status(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Modified) == 0 {
		t.Fatal("expected modified files")
	}
}

func TestGitLocal_CloneURL(t *testing.T) {
	p, err := gitprovider.New("local", nil)
	if err != nil {
		t.Fatal(err)
	}

	url, err := p.CloneURL(context.Background(), "https://github.com/example/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/example/repo.git" {
		t.Fatalf("expected URL pass-through, got %q", url)
	}
}

// --- Helpers ---

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, dir, "add", ".")
	runGitCmd(t, dir, "commit", "-m", "initial commit")
	return dir
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// TestProvider_PlantedGitConfigNeverRuns: the git status/branch/checkout API
// runs git in agent-writable workspaces; planted config, attributes and hooks
// must not run in the Go Core, and pull must not reach another workspace
// through an agent-set remote (KI-77).
func TestProvider_PlantedGitConfigNeverRuns(t *testing.T) {
	ctx := context.Background()
	dir := initTestRepo(t)
	other := initTestRepo(t)
	markerDir := t.TempDir()
	marker := filepath.Join(markerDir, "marker")
	program := filepath.Join(markerDir, "evil.sh")
	if err := os.WriteFile(program, []byte("#!/bin/sh\necho \"$*\" >> "+marker+"\ncat\n"), 0o755); err != nil { //nolint:gosec // executable test program
		t.Fatal(err)
	}
	branch := currentBranch(t, dir)
	runGitCmd(t, dir, "config", "core.fsmonitor", program)
	runGitCmd(t, dir, "config", "filter.x.clean", program)
	runGitCmd(t, dir, "config", "filter.x.smudge", program)
	runGitCmd(t, dir, "remote", "add", "origin", other)
	runGitCmd(t, dir, "config", "branch."+branch+".remote", "origin")
	runGitCmd(t, dir, "config", "branch."+branch+".merge", "refs/heads/"+currentBranch(t, other))
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("* filter=x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(dir, ".git", "hooks", "post-checkout")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\n"+program+" hook\n"), 0o755); err != nil { //nolint:gosec // executable hook
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := gitprovider.New("local", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Status(ctx, dir); err != nil {
		t.Fatalf("Status: %v", err)
	}
	if _, err := p.ListBranches(ctx, dir); err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	runGitCmd(t, dir, "branch", "feature")
	if err := p.Checkout(ctx, dir, "feature"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if err := p.Pull(ctx, dir); err == nil {
		t.Fatal("Pull from a local path (another workspace) through the agent-set remote succeeded")
	}
	if data, err := os.ReadFile(marker); err == nil { //nolint:gosec // test marker
		t.Fatalf("a program planted in the workspace ran: %s", data)
	}
}

func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
