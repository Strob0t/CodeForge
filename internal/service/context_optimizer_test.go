package service_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
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
	svc := service.NewContextOptimizerService(store, orchCfg, &config.Limits{MaxFiles: 50, MaxFileSize: 32768, SearchTimeout: 5 * time.Second})

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
	svc := service.NewContextOptimizerService(store, orchCfg, &config.Limits{MaxFiles: 50, MaxFileSize: 32768, SearchTimeout: 5 * time.Second})

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
	svc := service.NewContextOptimizerService(store, orchCfg, &config.Limits{MaxFiles: 50, MaxFileSize: 32768, SearchTimeout: 5 * time.Second})

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
	svc := service.NewContextOptimizerService(store, orchCfg, &config.Limits{MaxFiles: 50, MaxFileSize: 32768, SearchTimeout: 5 * time.Second})

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

// --- fetchRetrievalEntries integration tests (Phase 6C review) ---

// setupRetrievalContextTest creates a context optimizer wired to a real
// RetrievalService with a captureQueue, ready for BuildContextPack round-trips.
func setupRetrievalContextTest(t *testing.T, subAgentEnabled bool) (
	*service.ContextOptimizerService,
	*service.RetrievalService,
	*captureQueue,
) {
	t.Helper()
	store := &runtimeMockStore{
		projects: []project.Project{
			{ID: "proj-1", Name: "test", WorkspacePath: ""},
		},
		tasks: []task.Task{
			{ID: "task-1", ProjectID: "proj-1", Prompt: "find handler code"},
		},
	}
	q := &captureQueue{}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{
		DefaultContextBudget: 8192,
		PromptReserve:        1024,
		SubAgentEnabled:      subAgentEnabled,
		SubAgentModel:        "test-model",
		SubAgentMaxQueries:   3,
		SubAgentRerank:       false,
		SubAgentTimeout:      2 * time.Second,
		RetrievalTopK:        5,
	}
	retrievalSvc := service.NewRetrievalService(store, q, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})

	// Mark the index as "ready" so fetchRetrievalEntries is triggered.
	_ = retrievalSvc.HandleIndexResult(context.Background(), &messagequeue.RetrievalIndexResultPayload{
		ProjectID:      "proj-1",
		Status:         "ready",
		FileCount:      10,
		ChunkCount:     50,
		EmbeddingModel: "text-embedding-3-small",
	})

	ctxSvc := service.NewContextOptimizerService(store, orchCfg, &config.Limits{MaxFiles: 50, MaxFileSize: 32768, SearchTimeout: 5 * time.Second})
	ctxSvc.SetRetrieval(retrievalSvc)

	return ctxSvc, retrievalSvc, q
}

func TestFetchRetrievalEntries_SubAgentSuccess(t *testing.T) {
	ctxSvc, retrievalSvc, q := setupRetrievalContextTest(t, true)

	type buildResult struct {
		pack *cfcontext.ContextPack
		err  error
	}
	ch := make(chan buildResult, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		pack, err := ctxSvc.BuildContextPack(ctx, "task-1", "proj-1", "")
		ch <- buildResult{pack, err}
	}()

	// Wait for the sub-agent request to be published.
	time.Sleep(100 * time.Millisecond)

	subj, data := q.snapshot()
	if subj != messagequeue.SubjectSubAgentSearchRequest {
		t.Fatalf("expected subject %s, got %s", messagequeue.SubjectSubAgentSearchRequest, subj)
	}

	var reqPayload messagequeue.SubAgentSearchRequestPayload
	if err := json.Unmarshal(data, &reqPayload); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	// Deliver a sub-agent result.
	retrievalSvc.HandleSubAgentSearchResult(context.Background(), &messagequeue.SubAgentSearchResultPayload{
		ProjectID:       "proj-1",
		Query:           "find handler code",
		RequestID:       reqPayload.RequestID,
		Results:         []messagequeue.RetrievalSearchHitPayload{{Filepath: "handler.go", Content: "func Handle() {}", Score: 0.95}},
		ExpandedQueries: []string{"handler function", "request handler"},
		TotalCandidates: 5,
	})

	res := <-ch
	if res.err != nil {
		t.Fatalf("BuildContextPack failed: %v", res.err)
	}
	if res.pack == nil {
		t.Fatal("expected non-nil pack")
	}

	foundHybrid := false
	for _, e := range res.pack.Entries {
		if e.Kind == cfcontext.EntryHybrid && e.Path == "handler.go" {
			foundHybrid = true
		}
	}
	if !foundHybrid {
		t.Error("expected hybrid entry for handler.go from sub-agent search")
	}
}

func TestFetchRetrievalEntries_SubAgentDisabled_UsesSingleShot(t *testing.T) {
	ctxSvc, retrievalSvc, q := setupRetrievalContextTest(t, false)

	type buildResult struct {
		pack *cfcontext.ContextPack
		err  error
	}
	ch := make(chan buildResult, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		pack, err := ctxSvc.BuildContextPack(ctx, "task-1", "proj-1", "")
		ch <- buildResult{pack, err}
	}()

	// Wait for the single-shot search request to be published.
	time.Sleep(100 * time.Millisecond)

	subj, data := q.snapshot()
	if subj != messagequeue.SubjectRetrievalSearchRequest {
		t.Fatalf("expected subject %s (single-shot), got %s", messagequeue.SubjectRetrievalSearchRequest, subj)
	}

	var reqPayload messagequeue.RetrievalSearchRequestPayload
	if err := json.Unmarshal(data, &reqPayload); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	// Deliver a single-shot result.
	retrievalSvc.HandleSearchResult(context.Background(), &messagequeue.RetrievalSearchResultPayload{
		ProjectID: "proj-1",
		Query:     "find handler code",
		RequestID: reqPayload.RequestID,
		Results:   []messagequeue.RetrievalSearchHitPayload{{Filepath: "service.go", Content: "func Serve() {}", Score: 0.80}},
	})

	res := <-ch
	if res.err != nil {
		t.Fatalf("BuildContextPack failed: %v", res.err)
	}
	if res.pack == nil {
		t.Fatal("expected non-nil pack")
	}

	foundHybrid := false
	for _, e := range res.pack.Entries {
		if e.Kind == cfcontext.EntryHybrid && e.Path == "service.go" {
			foundHybrid = true
		}
	}
	if !foundHybrid {
		t.Error("expected hybrid entry for service.go from single-shot search")
	}
}

func TestFetchRetrievalEntries_BothFail_ReturnsNilPack(t *testing.T) {
	// Use a very short context so both sub-agent and single-shot timeout quickly.
	ctxSvc, _, _ := setupRetrievalContextTest(t, true)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	pack, err := ctxSvc.BuildContextPack(ctx, "task-1", "proj-1", "")
	// Workspace is empty, retrieval times out, so no candidates → nil pack.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pack != nil {
		// If there are entries, none should be hybrid.
		for _, e := range pack.Entries {
			if e.Kind == cfcontext.EntryHybrid {
				t.Error("expected no hybrid entries when both retrieval paths fail")
			}
		}
	}
}
