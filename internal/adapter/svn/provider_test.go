package svn

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// Compile-time interface check.
var _ gitprovider.Provider = (*Provider)(nil)

func TestSVN_ProviderName(t *testing.T) {
	p := NewProvider(nil)
	if p.Name() != "svn" {
		t.Fatalf("expected 'svn', got %q", p.Name())
	}
}

func TestSVN_Capabilities(t *testing.T) {
	p := NewProvider(nil)
	caps := p.Capabilities()
	if !caps.Clone {
		t.Fatal("expected Clone=true")
	}
	if caps.Push {
		t.Fatal("expected Push=false")
	}
	if caps.PullRequest {
		t.Fatal("expected PullRequest=false")
	}
	if caps.Webhook {
		t.Fatal("expected Webhook=false")
	}
}

func TestSVN_CloneURL(t *testing.T) {
	p := NewProvider(nil)
	url, err := p.CloneURL(context.Background(), "svn://example.com/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "svn://example.com/repo" {
		t.Fatalf("expected passthrough URL, got %q", url)
	}
}

func TestListReposUnsupported(t *testing.T) {
	p := NewProvider(nil)
	_, err := p.ListRepos(context.Background())
	if err == nil {
		t.Fatal("expected error for unsupported ListRepos")
	}
}

func TestResolveBranchURL(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		branch string
		want   string
	}{
		{"empty branch", "svn://host/repo/trunk", "", "svn://host/repo/trunk"},
		{"trunk branch", "svn://host/repo/trunk", "trunk", "svn://host/repo/trunk"},
		{"branch with trunk suffix", "svn://host/repo/trunk", "feature-x", "svn://host/repo/branches/feature-x"},
		{"branch without trunk suffix", "svn://host/repo", "feature-x", "svn://host/repo/branches/feature-x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveBranchURL(tt.url, tt.branch)
			if got != tt.want {
				t.Fatalf("resolveBranchURL(%q, %q) = %q, want %q", tt.url, tt.branch, got, tt.want)
			}
		})
	}
}

func TestSVN_AuthFlagsInjected(t *testing.T) {
	var capturedArgs []string
	p := NewProvider(nil)
	p.username = "alice"
	p.password = "secret"
	p.execCommand = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		capturedArgs = args
		return exec.CommandContext(ctx, "echo", "ok")
	}

	_, _ = p.runSVN(context.Background(), "", "info")

	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "--username alice") {
		t.Fatalf("expected --username alice in args, got %v", capturedArgs)
	}
	if !strings.Contains(joined, "--password secret") {
		t.Fatalf("expected --password secret in args, got %v", capturedArgs)
	}
	if !strings.Contains(joined, "--no-auth-cache") {
		t.Fatalf("expected --no-auth-cache in args, got %v", capturedArgs)
	}
	// The original command should still be present after auth flags.
	if capturedArgs[len(capturedArgs)-1] != "info" {
		t.Fatalf("expected 'info' as last arg, got %q", capturedArgs[len(capturedArgs)-1])
	}
}

func TestSVN_AuthFlagsOmittedWhenEmpty(t *testing.T) {
	var capturedArgs []string
	p := NewProvider(nil)
	p.execCommand = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		capturedArgs = args
		return exec.CommandContext(ctx, "echo", "ok")
	}

	_, _ = p.runSVN(context.Background(), "", "status")

	if len(capturedArgs) != 1 || capturedArgs[0] != "status" {
		t.Fatalf("expected only ['status'] without auth flags, got %v", capturedArgs)
	}
}

// mockExecCommand creates a mock that returns the given output.
func mockExecCommand(output string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, "echo", output) //nolint:gosec // test only
		return cmd
	}
}

func TestStatusWithMock(t *testing.T) {
	p := NewProvider(nil)
	p.execCommand = mockExecCommand("42")

	dir := t.TempDir()
	status, err := p.Status(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status == nil {
		t.Fatal("expected non-nil status")
	}
}

func TestSVN_Registration(t *testing.T) {
	p, err := gitprovider.New("svn", nil)
	if err != nil {
		t.Fatalf("expected svn provider to be registered: %v", err)
	}
	if p.Name() != "svn" {
		t.Fatalf("expected name 'svn', got %q", p.Name())
	}
}

func TestSVN_RegistrationWithAuth(t *testing.T) {
	p, err := gitprovider.New("svn", map[string]string{
		"username": "testuser",
		"password": "testpass",
	})
	if err != nil {
		t.Fatalf("expected svn provider with auth config: %v", err)
	}
	svnP, ok := p.(*Provider)
	if !ok {
		t.Fatal("expected *Provider type")
	}
	if svnP.username != "testuser" {
		t.Fatalf("expected username 'testuser', got %q", svnP.username)
	}
	if svnP.password != "testpass" {
		t.Fatalf("expected password 'testpass', got %q", svnP.password)
	}
}

// --- Integration tests (require svnadmin + svn CLI) ---

func skipIfNoSVN(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("svnadmin"); err != nil {
		t.Skip("svnadmin not available in test environment")
	}
	if _, err := exec.LookPath("svn"); err != nil {
		t.Skip("svn not available in test environment")
	}
}

// initTestSVNRepo creates a local SVN repository with trunk/branches structure,
// imports a file, and returns the file:// URL to the repo.
func initTestSVNRepo(t *testing.T) string {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), "repo")
	runSVNCmd(t, "", "svnadmin", "create", repoDir)

	repoURL := "file://" + repoDir

	// Create trunk/branches layout.
	layoutDir := filepath.Join(t.TempDir(), "layout")
	if err := os.MkdirAll(filepath.Join(layoutDir, "trunk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(layoutDir, "branches"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layoutDir, "trunk", "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	runSVNCmd(t, "", "svn", "import", layoutDir, repoURL, "-m", "initial layout", "--non-interactive")

	// Create a branch.
	runSVNCmd(t, "", "svn", "copy",
		repoURL+"/trunk", repoURL+"/branches/feature-x",
		"-m", "create feature-x branch", "--non-interactive")

	return repoURL
}

func runSVNCmd(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

func TestSVN_CloneAndStatus(t *testing.T) {
	skipIfNoSVN(t)

	ctx := context.Background()
	repoURL := initTestSVNRepo(t)

	p := NewProvider(nil)

	cloneDir := filepath.Join(t.TempDir(), "wc")
	if err := p.Clone(ctx, repoURL+"/trunk", cloneDir); err != nil {
		t.Fatalf("Clone failed: %v", err)
	}

	status, err := p.Status(ctx, cloneDir)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status.CommitHash == "" {
		t.Fatal("expected non-empty revision")
	}
	if status.Dirty {
		t.Fatal("expected clean working copy")
	}
}

func TestSVN_CloneIdempotent(t *testing.T) {
	skipIfNoSVN(t)

	ctx := context.Background()
	repoURL := initTestSVNRepo(t)

	p := NewProvider(nil)
	cloneDir := filepath.Join(t.TempDir(), "wc")

	// First clone.
	if err := p.Clone(ctx, repoURL+"/trunk", cloneDir); err != nil {
		t.Fatalf("first Clone failed: %v", err)
	}

	// Second clone to same dir should succeed (reclone path: svn update).
	if err := p.Clone(ctx, repoURL+"/trunk", cloneDir); err != nil {
		t.Fatalf("second Clone (reclone) failed: %v", err)
	}

	// Verify it's still a valid working copy.
	status, err := p.Status(ctx, cloneDir)
	if err != nil {
		t.Fatalf("Status after reclone failed: %v", err)
	}
	if status.CommitHash == "" {
		t.Fatal("expected non-empty revision after reclone")
	}
}

func TestSVN_CloneRecloneDifferentURL(t *testing.T) {
	skipIfNoSVN(t)

	ctx := context.Background()
	repoURL := initTestSVNRepo(t)

	p := NewProvider(nil)
	cloneDir := filepath.Join(t.TempDir(), "wc")

	// Clone trunk.
	if err := p.Clone(ctx, repoURL+"/trunk", cloneDir); err != nil {
		t.Fatalf("Clone trunk failed: %v", err)
	}

	// Clone different URL to same dir — should remove and re-checkout.
	if err := p.Clone(ctx, repoURL+"/branches/feature-x", cloneDir); err != nil {
		t.Fatalf("Clone branch to same dir failed: %v", err)
	}

	status, err := p.Status(ctx, cloneDir)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if !strings.Contains(status.Branch, "feature-x") {
		t.Fatalf("expected branch URL containing 'feature-x', got %q", status.Branch)
	}
}

func TestSVN_CloneWithBranch(t *testing.T) {
	skipIfNoSVN(t)

	ctx := context.Background()
	repoURL := initTestSVNRepo(t)

	p := NewProvider(nil)
	cloneDir := filepath.Join(t.TempDir(), "wc")

	// Clone with branch option — should resolve to /branches/feature-x.
	if err := p.Clone(ctx, repoURL+"/trunk", cloneDir, gitprovider.WithBranch("feature-x")); err != nil {
		t.Fatalf("Clone with branch failed: %v", err)
	}

	status, err := p.Status(ctx, cloneDir)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if !strings.Contains(status.Branch, "feature-x") {
		t.Fatalf("expected branch URL containing 'feature-x', got %q", status.Branch)
	}
}

func TestSVN_ListBranches(t *testing.T) {
	skipIfNoSVN(t)

	ctx := context.Background()
	repoURL := initTestSVNRepo(t)

	p := NewProvider(nil)
	cloneDir := filepath.Join(t.TempDir(), "wc")
	if err := p.Clone(ctx, repoURL+"/trunk", cloneDir); err != nil {
		t.Fatal(err)
	}

	branches, err := p.ListBranches(ctx, cloneDir)
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}

	if len(branches) < 2 {
		t.Fatalf("expected at least 2 branches (trunk + feature-x), got %d", len(branches))
	}

	names := make(map[string]bool)
	for _, b := range branches {
		names[b.Name] = true
	}
	if !names["trunk"] {
		t.Fatal("expected 'trunk' in branches")
	}
	if !names["feature-x"] {
		t.Fatal("expected 'feature-x' in branches")
	}
}

func TestSVN_Checkout(t *testing.T) {
	skipIfNoSVN(t)

	ctx := context.Background()
	repoURL := initTestSVNRepo(t)

	p := NewProvider(nil)
	cloneDir := filepath.Join(t.TempDir(), "wc")
	if err := p.Clone(ctx, repoURL+"/trunk", cloneDir); err != nil {
		t.Fatal(err)
	}

	// Switch to feature-x branch.
	if err := p.Checkout(ctx, cloneDir, "feature-x"); err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}

	status, err := p.Status(ctx, cloneDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.Branch, "feature-x") {
		t.Fatalf("expected branch containing 'feature-x', got %q", status.Branch)
	}

	// Switch back to trunk.
	if err := p.Checkout(ctx, cloneDir, "trunk"); err != nil {
		t.Fatalf("Checkout trunk failed: %v", err)
	}
	status, err = p.Status(ctx, cloneDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.Branch, "trunk") {
		t.Fatalf("expected branch containing 'trunk', got %q", status.Branch)
	}
}

func TestSVN_DirtyStatus(t *testing.T) {
	skipIfNoSVN(t)

	ctx := context.Background()
	repoURL := initTestSVNRepo(t)

	p := NewProvider(nil)
	cloneDir := filepath.Join(t.TempDir(), "wc")
	if err := p.Clone(ctx, repoURL+"/trunk", cloneDir); err != nil {
		t.Fatal(err)
	}

	// Create an untracked file.
	if err := os.WriteFile(filepath.Join(cloneDir, "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := p.Status(ctx, cloneDir)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Dirty {
		t.Fatal("expected dirty status")
	}
	if len(status.Untracked) == 0 {
		t.Fatal("expected untracked files")
	}

	// Modify a tracked file.
	if err := os.WriteFile(filepath.Join(cloneDir, "hello.txt"), []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err = p.Status(ctx, cloneDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Modified) == 0 {
		t.Fatal("expected modified files")
	}
}

func TestSVN_Pull(t *testing.T) {
	skipIfNoSVN(t)

	ctx := context.Background()
	repoURL := initTestSVNRepo(t)

	p := NewProvider(nil)
	cloneDir := filepath.Join(t.TempDir(), "wc")
	if err := p.Clone(ctx, repoURL+"/trunk", cloneDir); err != nil {
		t.Fatal(err)
	}

	// Pull should succeed (no new revisions, but no error either).
	if err := p.Pull(ctx, cloneDir); err != nil {
		t.Fatalf("Pull failed: %v", err)
	}
}
