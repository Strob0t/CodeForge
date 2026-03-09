package service

import (
	"path/filepath"
	"testing"
)

func TestWorkspaceRootIsAbsolute(t *testing.T) {
	t.Parallel()
	ps := NewProjectService(nil, "data/workspaces")
	if !filepath.IsAbs(ps.workspaceRoot) {
		t.Errorf("expected absolute path, got %q", ps.workspaceRoot)
	}
}

func TestNewProjectServiceResolvesAbsoluteRoot(t *testing.T) {
	t.Parallel()
	ps := NewProjectService(nil, "relative/path")
	if !filepath.IsAbs(ps.workspaceRoot) {
		t.Fatalf("expected absolute, got %q", ps.workspaceRoot)
	}
	if ps.workspaceRoot == "relative/path" {
		t.Fatal("workspaceRoot was not resolved")
	}
}

func TestNewProjectServicePreservesAbsoluteRoot(t *testing.T) {
	t.Parallel()
	ps := NewProjectService(nil, "/tmp/test-workspace")
	if ps.workspaceRoot != "/tmp/test-workspace" {
		t.Errorf("expected /tmp/test-workspace, got %q", ps.workspaceRoot)
	}
}
