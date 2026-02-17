package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/service"
)

func TestBuildContextPack_WithMatchingFiles(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n\nfunc handleAuth() {}"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Project Docs\nSome unrelated content."), 0o644)

	store := &runtimeMockStore{
		projects: []project.Project{
			{ID: "proj-1", Name: "test", WorkspacePath: dir},
		},
		tasks: []task.Task{
			{ID: "task-1", ProjectID: "proj-1", Prompt: "Implement authentication handler"},
		},
	}

	orchCfg := &config.Orchestrator{DefaultContextBudget: 8192, PromptReserve: 1024}
	svc := service.NewContextOptimizerService(store, orchCfg)

	pack, err := svc.BuildContextPack(context.Background(), "task-1", "proj-1", "")
	if err != nil {
		t.Fatalf("BuildContextPack failed: %v", err)
	}
	if pack == nil {
		t.Fatal("expected non-nil pack")
	}
	if len(pack.Entries) == 0 {
		t.Fatal("expected at least one entry")
	}

	// handler.go should match better (contains "handler" and "auth").
	foundHandler := false
	for _, e := range pack.Entries {
		if e.Path == "handler.go" {
			foundHandler = true
			if e.Priority == 0 {
				t.Error("expected non-zero priority for handler.go")
			}
		}
	}
	if !foundHandler {
		t.Error("expected handler.go in pack entries")
	}
}

func TestBuildContextPack_WithSharedContext(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)

	store := &runtimeMockStore{
		projects: []project.Project{
			{ID: "proj-1", Name: "test", WorkspacePath: dir},
		},
		tasks: []task.Task{
			{ID: "task-1", ProjectID: "proj-1", Prompt: "Add main function"},
		},
		sharedContexts: []cfcontext.SharedContext{
			{
				ID:        "sc-1",
				TeamID:    "team-1",
				ProjectID: "proj-1",
				Version:   1,
				Items: []cfcontext.SharedContextItem{
					{ID: "sci-1", SharedID: "sc-1", Key: "step-1-output", Value: "Created base module structure", Tokens: 8, Author: "agent-1"},
				},
			},
		},
	}

	orchCfg := &config.Orchestrator{DefaultContextBudget: 8192, PromptReserve: 1024}
	svc := service.NewContextOptimizerService(store, orchCfg)

	pack, err := svc.BuildContextPack(context.Background(), "task-1", "proj-1", "team-1")
	if err != nil {
		t.Fatalf("BuildContextPack failed: %v", err)
	}
	if pack == nil {
		t.Fatal("expected non-nil pack")
	}

	foundShared := false
	for _, e := range pack.Entries {
		if e.Kind == cfcontext.EntryShared {
			foundShared = true
			if e.Path != "step-1-output" {
				t.Errorf("expected shared entry path 'step-1-output', got %q", e.Path)
			}
		}
	}
	if !foundShared {
		t.Error("expected shared context entry in pack")
	}
}

func TestBuildContextPack_RespectsTokenBudget(t *testing.T) {
	dir := t.TempDir()
	// Create a file that is large relative to the budget.
	bigContent := make([]byte, 2000) // ~500 tokens
	for i := range bigContent {
		bigContent[i] = 'a'
	}
	_ = os.WriteFile(filepath.Join(dir, "big.go"), bigContent, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "small.go"), []byte("package small"), 0o644)

	store := &runtimeMockStore{
		projects: []project.Project{
			{ID: "proj-1", Name: "test", WorkspacePath: dir},
		},
		tasks: []task.Task{
			// Prompt that matches both files.
			{ID: "task-1", ProjectID: "proj-1", Prompt: "work with big and small package"},
		},
	}

	// Set a very tight budget: only room for ~50 tokens of context.
	orchCfg := &config.Orchestrator{DefaultContextBudget: 100, PromptReserve: 50}
	svc := service.NewContextOptimizerService(store, orchCfg)

	pack, err := svc.BuildContextPack(context.Background(), "task-1", "proj-1", "")
	if err != nil {
		t.Fatalf("BuildContextPack failed: %v", err)
	}

	if pack != nil && pack.TokensUsed > 50 {
		t.Errorf("tokens_used %d exceeds available budget 50", pack.TokensUsed)
	}
}

func TestBuildContextPack_EmptyWorkspace(t *testing.T) {
	store := &runtimeMockStore{
		projects: []project.Project{
			{ID: "proj-1", Name: "test", WorkspacePath: ""},
		},
		tasks: []task.Task{
			{ID: "task-1", ProjectID: "proj-1", Prompt: "do something"},
		},
	}

	orchCfg := &config.Orchestrator{DefaultContextBudget: 4096, PromptReserve: 1024}
	svc := service.NewContextOptimizerService(store, orchCfg)

	pack, err := svc.BuildContextPack(context.Background(), "task-1", "proj-1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pack != nil {
		t.Fatalf("expected nil pack for empty workspace, got %d entries", len(pack.Entries))
	}
}

func TestEstimateTokens_Basic(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"ab", 1},
		{"abcdefgh", 2},
		{"abcdefghijklmnop", 4}, // 16 / 4 = 4
	}
	for _, tc := range tests {
		got := cfcontext.EstimateTokens(tc.input)
		if got != tc.expected {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}

func TestScoreFileRelevance(t *testing.T) {
	prompt := "implement authentication handler for users"
	// File with matching keywords should score > 0.
	score := service.ScoreFileRelevance(prompt, "auth_handler.go", "func handleAuth(user User) {}")
	if score == 0 {
		t.Error("expected non-zero score for file with matching keywords")
	}

	// File with no matching keywords should score 0.
	scoreZero := service.ScoreFileRelevance(prompt, "readme.txt", "welcome to the project")
	if scoreZero != 0 {
		t.Errorf("expected 0 score for unrelated file, got %d", scoreZero)
	}
}
