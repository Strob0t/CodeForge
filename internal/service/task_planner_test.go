package service_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/service"
)

func TestGatherProjectContext_ValidWorkspace(t *testing.T) {
	// Create a temporary workspace directory with some files.
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "src", "handler.go"), []byte("package src"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".hidden"), []byte("secret"), 0o644)

	store := &runtimeMockStore{
		projects: []project.Project{
			{ID: "proj-1", Name: "test", WorkspacePath: dir},
		},
	}

	orchCfg := &config.Orchestrator{MaxTeamSize: 5}
	svc := service.NewTaskPlannerService(nil, nil, store, orchCfg)

	ctx := context.Background()
	result, err := svc.GatherProjectContextForTest(ctx, "proj-1")
	if err != nil {
		t.Fatalf("gatherProjectContext failed: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty context")
	}

	// Should contain the non-hidden files.
	if !strings.Contains(result, "main.go") {
		t.Error("expected 'main.go' in context")
	}
	if !strings.Contains(result, "README.md") {
		t.Error("expected 'README.md' in context")
	}
	if !strings.Contains(result, "src/") {
		t.Error("expected 'src/' in context")
	}
	if !strings.Contains(result, "handler.go") {
		t.Error("expected 'handler.go' in context")
	}
	// Should NOT contain hidden files.
	if strings.Contains(result, ".hidden") {
		t.Error("did not expect '.hidden' in context")
	}
}

func TestGatherProjectContext_EmptyWorkspace(t *testing.T) {
	store := &runtimeMockStore{
		projects: []project.Project{
			{ID: "proj-1", Name: "test", WorkspacePath: ""},
		},
	}

	orchCfg := &config.Orchestrator{MaxTeamSize: 5}
	svc := service.NewTaskPlannerService(nil, nil, store, orchCfg)

	ctx := context.Background()
	result, err := svc.GatherProjectContextForTest(ctx, "proj-1")
	if err != nil {
		t.Fatalf("gatherProjectContext failed: %v", err)
	}
	if result != "" {
		t.Fatalf("expected empty context for project without workspace, got %q", result)
	}
}

func TestEstimateComplexity(t *testing.T) {
	orchCfg := &config.Orchestrator{MaxTeamSize: 5}
	svc := service.NewTaskPlannerService(nil, nil, nil, orchCfg)

	tests := []struct {
		steps    int
		expected string
	}{
		{0, "single"},
		{1, "single"},
		{2, "pair"},
		{3, "team"},
		{10, "team"},
	}

	for _, tc := range tests {
		got := svc.EstimateComplexityForTest(tc.steps)
		if string(got) != tc.expected {
			t.Errorf("estimateComplexity(%d) = %s, want %s", tc.steps, got, tc.expected)
		}
	}
}
