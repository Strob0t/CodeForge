package svn

import (
	"context"
	"os/exec"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// Compile-time interface check.
var _ gitprovider.Provider = (*Provider)(nil)

func TestProviderName(t *testing.T) {
	p := NewProvider(nil)
	if p.Name() != "svn" {
		t.Fatalf("expected 'svn', got %q", p.Name())
	}
}

func TestCapabilities(t *testing.T) {
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

func TestCloneURL(t *testing.T) {
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

// mockExecCommand creates a mock that records calls but returns empty output.
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
