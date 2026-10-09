package github

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
)

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
	return strings.TrimSpace(string(out))
}

// KI-189 (R6-13): the checkout branch reached `git checkout <branch>`
// unchecked, so "-f", "." or a file name discarded uncommitted changes.
func TestCheckout_OnlyBranchNamesReachGit(t *testing.T) {
	ctx := context.Background()
	p := NewProvider("", "")
	for _, tt := range []struct {
		branch  string
		invalid bool
	}{
		{"-f", true}, {".", true}, {"--force", true}, {"HEAD", true}, {"a..b", true},
		{"hello.txt", false}, // valid, but no such branch: never the file
	} {
		t.Run(tt.branch, func(t *testing.T) {
			dir := t.TempDir()
			gitCmd(t, dir, "init", "-q", "-b", "main")
			if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0o644); err != nil {
				t.Fatal(err)
			}
			gitCmd(t, dir, "add", "-A")
			gitCmd(t, dir, "commit", "-q", "-m", "init")
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

	dir := t.TempDir()
	gitCmd(t, dir, "init", "-q", "-b", "main")
	gitCmd(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	gitCmd(t, dir, "branch", "feature")
	if err := p.Checkout(ctx, dir, "feature"); err != nil {
		t.Fatalf("Checkout(feature): %v", err)
	}
	if got := gitCmd(t, dir, "symbolic-ref", "--short", "HEAD"); got != "feature" {
		t.Fatalf("current branch %q, want feature", got)
	}
}
