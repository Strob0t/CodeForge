package gitlocal_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// KI-189 (R6-13): the branch of POST /projects/{id}/git/checkout reached
// `git checkout <branch>` unchecked: "-f", "." or a file name silently
// discarded uncommitted changes of the agent and the user. Only valid branch
// names reach git, which reads them as nothing but a branch.
func TestCheckout_RefusesNamesGitReadsAsOptionsOrPaths(t *testing.T) {
	ctx := context.Background()
	p, err := gitprovider.New("local", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		branch  string
		invalid bool // refused before git runs
	}{
		{"-f", true}, {".", true}, {"--force", true}, {"a..b", true}, {"HEAD", true}, {"@{-1}", true},
		{"-", true}, {"x y", true}, {"feature~1", true},
		{"hello.txt", false}, // a valid name, but no branch: never the file
	} {
		t.Run(tt.branch, func(t *testing.T) {
			dir := initTestRepo(t)
			runGitCmd(t, dir, "branch", "other")
			if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("uncommitted work"), 0o644); err != nil {
				t.Fatal(err)
			}
			err := p.Checkout(ctx, dir, tt.branch)
			if err == nil || tt.invalid != errors.Is(err, domain.ErrValidation) {
				t.Fatalf("Checkout(%q) = %v, want an error (a validation error: %v)", tt.branch, err, tt.invalid)
			}
			if data, _ := os.ReadFile(filepath.Join(dir, "hello.txt")); string(data) != "uncommitted work" { //nolint:gosec // test file
				t.Fatalf("Checkout(%q) discarded the uncommitted change: %q", tt.branch, data)
			}
		})
	}
}

// A branch that exists only on the remote is still created from it.
func TestCheckout_RemoteOnlyBranch(t *testing.T) {
	ctx := context.Background()
	src := initTestRepo(t)
	runGitCmd(t, src, "branch", "remote-only")
	clone := filepath.Join(t.TempDir(), "clone")
	runGitCmd(t, "", "clone", "-q", src, clone)

	p, err := gitprovider.New("local", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Checkout(ctx, clone, "remote-only"); err != nil {
		t.Fatalf("Checkout of a remote-only branch: %v", err)
	}
	if got := currentBranch(t, clone); got != "remote-only" {
		t.Fatalf("current branch %q, want remote-only", got)
	}
}
