//go:build !smoke

package messagequeue_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/trust"
	mq "github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// --- Sample data factories ---

func sampleConversationToolCallFunction() mq.ConversationToolCallFunction {
	return mq.ConversationToolCallFunction{
		Name:      "read_file",
		Arguments: `{"path": "/src/main.go", "lines": 50}`,
	}
}

func sampleConversationToolCall() mq.ConversationToolCall {
	return mq.ConversationToolCall{
		ID:       "call_abc123",
		Type:     "function",
		Function: sampleConversationToolCallFunction(),
	}
}

func sampleConversationMessagePayload() mq.ConversationMessagePayload {
	return mq.ConversationMessagePayload{
		Role:       "assistant",
		Content:    "I will read the file for you.",
		ToolCalls:  []mq.ConversationToolCall{sampleConversationToolCall()},
		ToolCallID: "call_abc123",
		Name:       "read_file",
	}
}

func sampleModePayload() *mq.ModePayload {
	return &mq.ModePayload{
		ID:               "coder",
		PromptPrefix:     "You are an expert Go developer.",
		Tools:            []string{"read_file", "write_file", "bash"},
		DeniedTools:      []string{"deploy"},
		DeniedActions:    []string{"rm -rf /"},
		RequiredArtifact: "code",
		LLMScenario:      "default",
		OutputSchema:     `{"type":"object"}`,
	}
}

func sampleTerminationPayload() mq.TerminationPayload {
	return mq.TerminationPayload{
		MaxSteps:       50,
		TimeoutSeconds: 600,
		MaxCost:        5.0,
	}
}

func sampleContextEntryPayload() mq.ContextEntryPayload {
	return mq.ContextEntryPayload{
		Kind:     "file",
		Path:     "/src/main.go",
		Content:  "package main\n\nfunc main() {}",
		Tokens:   42,
		Priority: 1,
	}
}

func sampleMCPServerDefPayload() mq.MCPServerDefPayload {
	return mq.MCPServerDefPayload{
		ID:          "550e8400-e29b-41d4-a716-446655440010",
		Name:        "filesystem-server",
		Description: "MCP server for filesystem operations",
		Transport:   "stdio",
		Command:     "/usr/local/bin/mcp-filesystem",
		Args:        []string{"--root", "/workspace"},
		URL:         "http://localhost:3001",
		Env:         map[string]string{"MCP_LOG_LEVEL": "debug"},
		Headers:     map[string]string{"Authorization": "Bearer tok_sample"},
		Enabled:     true,
	}
}

func sampleTrustAnnotation() *trust.Annotation {
	return &trust.Annotation{
		Origin:     "internal",
		TrustLevel: trust.LevelFull,
		SourceID:   "agent-550e8400",
		Signature:  "abcdef1234567890",
		Timestamp:  "2026-03-08T12:00:00Z",
	}
}

func sampleBenchmarkTaskResult() mq.BenchmarkTaskResult {
	return mq.BenchmarkTaskResult{
		TaskID:         "task-001",
		TaskName:       "lru-cache-implementation",
		Scores:         map[string]float64{"correctness": 0.95, "style": 0.88},
		ActualOutput:   "class LRUCache:\n    pass",
		ExpectedOutput: "class LRUCache:\n    def __init__(self, capacity): ...",
		ToolCalls:      []map[string]string{{"tool": "write_file", "path": "/src/lru.py"}},
		CostUSD:        0.042,
		TokensIn:       1500,
		TokensOut:      800,
		DurationMs:     12345,
		EvaluatorScores: map[string]map[string]float64{
			"llm_judge":       {"relevance": 0.9, "accuracy": 0.85},
			"functional_test": {"pass_rate": 0.96},
		},
		FilesChanged:         []string{"/src/lru.py", "/tests/test_lru.py"},
		FunctionalTestOutput: "25/25 tests passed",
		RolloutID:            1,
		RolloutCount:         3,
		IsBestRollout:        true,
		DiversityScore:       0.72,
	}
}

func sampleBenchmarkSummary() mq.BenchmarkSummary {
	return mq.BenchmarkSummary{
		TaskCount:      10,
		AvgScore:       0.87,
		TotalCostUSD:   1.25,
		TotalTokensIn:  15000,
		TotalTokensOut: 8000,
		ElapsedMs:      120000,
	}
}

func sampleGemmasAgentMessagePayload() mq.GemmasAgentMessagePayload {
	return mq.GemmasAgentMessagePayload{
		AgentID:       "agent-alpha",
		Content:       "I propose we use a hash map for O(1) lookups.",
		Round:         2,
		ParentAgentID: "agent-orchestrator",
	}
}

func sampleRetrievalSearchHitPayload() mq.RetrievalSearchHitPayload {
	return mq.RetrievalSearchHitPayload{
		Filepath:     "/src/cache/lru.go",
		StartLine:    10,
		EndLine:      45,
		Content:      "func NewLRU(cap int) *LRU {",
		Language:     "go",
		SymbolName:   "NewLRU",
		Score:        0.92,
		BM25Rank:     1,
		SemanticRank: 2,
		ProjectID:    "550e8400-e29b-41d4-a716-446655440001",
	}
}

func sampleGraphSearchHitPayload() mq.GraphSearchHitPayload {
	return mq.GraphSearchHitPayload{
		Filepath:   "/src/cache/lru.go",
		SymbolName: "LRU.Get",
		Kind:       "method",
		StartLine:  50,
		EndLine:    65,
		Distance:   2,
		Score:      0.88,
		EdgePath:   []string{"LRU", "LRU.Get", "LRU.cache"},
		ProjectID:  "550e8400-e29b-41d4-a716-446655440001",
	}
}

// --- Top-level sample payload factories ---

func sampleConversationRunStartPayload() mq.ConversationRunStartPayload {
	return mq.ConversationRunStartPayload{
		RunID:             "550e8400-e29b-41d4-a716-446655440001",
		ConversationID:    "550e8400-e29b-41d4-a716-446655440002",
		SessionID:         "550e8400-e29b-41d4-a716-446655440003",
		ProjectID:         "550e8400-e29b-41d4-a716-446655440004",
		Messages:          []mq.ConversationMessagePayload{sampleConversationMessagePayload()},
		SystemPrompt:      "You are a helpful coding assistant.",
		Model:             "anthropic/claude-sonnet-4-20250514",
		PolicyProfile:     "standard",
		WorkspacePath:     "/workspaces/my-project",
		Mode:              sampleModePayload(),
		Termination:       sampleTerminationPayload(),
		Context:           []mq.ContextEntryPayload{sampleContextEntryPayload()},
		MCPServers:        []mq.MCPServerDefPayload{sampleMCPServerDefPayload()},
		Tools:             []string{"read_file", "write_file", "bash"},
		MicroagentPrompts: []string{"When working with Go, always run gofmt."},
		Trust:             sampleTrustAnnotation(),
		RoutingEnabled:    true,
		Agentic:           true,
		ProviderAPIKey:    "sk-user-key-abc123",
	}
}

func sampleConversationRunCompletePayload() mq.ConversationRunCompletePayload {
	return mq.ConversationRunCompletePayload{
		RunID:            "550e8400-e29b-41d4-a716-446655440001",
		ConversationID:   "550e8400-e29b-41d4-a716-446655440002",
		SessionID:        "550e8400-e29b-41d4-a716-446655440003",
		AssistantContent: "I have successfully implemented the LRU cache.",
		ToolMessages:     []mq.ConversationMessagePayload{sampleConversationMessagePayload()},
		Status:           "completed",
		Error:            "",
		CostUSD:          0.035,
		TokensIn:         2500,
		TokensOut:        1200,
		StepCount:        7,
		Model:            "anthropic/claude-sonnet-4-20250514",
	}
}

func sampleBenchmarkRunRequestPayload() mq.BenchmarkRunRequestPayload {
	return mq.BenchmarkRunRequestPayload{
		RunID:              "550e8400-e29b-41d4-a716-446655440005",
		TenantID:           "550e8400-e29b-41d4-a716-446655440006",
		DatasetPath:        "/datasets/basic-coding",
		Model:              "mistral/mistral-large-latest",
		Metrics:            []string{"correctness", "style", "efficiency"},
		BenchmarkType:      "coding",
		SuiteID:            "550e8400-e29b-41d4-a716-446655440007",
		ExecMode:           "sandbox",
		Evaluators:         []string{"llm_judge", "functional_test"},
		HybridVerification: true,
		RolloutCount:       3,
		RolloutStrategy:    "best",
	}
}

func sampleBenchmarkRunResultPayload() mq.BenchmarkRunResultPayload {
	return mq.BenchmarkRunResultPayload{
		RunID:           "550e8400-e29b-41d4-a716-446655440005",
		TenantID:        "550e8400-e29b-41d4-a716-446655440006",
		Status:          "completed",
		Results:         []mq.BenchmarkTaskResult{sampleBenchmarkTaskResult()},
		Summary:         sampleBenchmarkSummary(),
		TotalCost:       1.25,
		TotalTokens:     23000,
		TotalDurationMs: 120000,
		Error:           "",
	}
}

func sampleBenchmarkTaskStartedPayload() mq.BenchmarkTaskStartedPayload {
	return mq.BenchmarkTaskStartedPayload{
		RunID:    "550e8400-e29b-41d4-a716-446655440005",
		TaskID:   "task-001",
		TaskName: "fibonacci",
		Index:    0,
		Total:    5,
	}
}

func sampleBenchmarkTaskProgressPayload() mq.BenchmarkTaskProgressPayload {
	return mq.BenchmarkTaskProgressPayload{
		RunID:          "550e8400-e29b-41d4-a716-446655440005",
		TaskID:         "task-001",
		TaskName:       "fibonacci",
		Score:          0.85,
		CostUSD:        0.02,
		CompletedTasks: 1,
		TotalTasks:     5,
		AvgScore:       0.85,
		TotalCostUSD:   0.02,
	}
}

func sampleGemmasEvalRequestPayload() mq.GemmasEvalRequestPayload {
	return mq.GemmasEvalRequestPayload{
		PlanID:   "550e8400-e29b-41d4-a716-446655440008",
		Messages: []mq.GemmasAgentMessagePayload{sampleGemmasAgentMessagePayload()},
	}
}

func sampleGemmasEvalResultPayload() mq.GemmasEvalResultPayload {
	return mq.GemmasEvalResultPayload{
		PlanID:                    "550e8400-e29b-41d4-a716-446655440008",
		InformationDiversityScore: 0.78,
		UnnecessaryPathRatio:      0.12,
		Error:                     "",
	}
}

func sampleRepoMapRequestPayload() mq.RepoMapRequestPayload {
	return mq.RepoMapRequestPayload{
		ProjectID:     "550e8400-e29b-41d4-a716-446655440001",
		WorkspacePath: "/workspaces/my-project",
		TokenBudget:   4096,
		ActiveFiles:   []string{"/src/main.go", "/src/handler.go"},
	}
}

func sampleRepoMapResultPayload() mq.RepoMapResultPayload {
	return mq.RepoMapResultPayload{
		ProjectID:   "550e8400-e29b-41d4-a716-446655440001",
		MapText:     "src/main.go: main, init\nsrc/handler.go: Handler, ServeHTTP",
		TokenCount:  256,
		FileCount:   15,
		SymbolCount: 42,
		Languages:   []string{"go", "python"},
		Error:       "",
	}
}

func sampleRetrievalIndexRequestPayload() mq.RetrievalIndexRequestPayload {
	return mq.RetrievalIndexRequestPayload{
		ProjectID:      "550e8400-e29b-41d4-a716-446655440001",
		WorkspacePath:  "/workspaces/my-project",
		EmbeddingModel: "text-embedding-3-small",
		FileExtensions: []string{".go", ".py", ".ts"},
	}
}

func sampleRetrievalIndexResultPayload() mq.RetrievalIndexResultPayload {
	return mq.RetrievalIndexResultPayload{
		ProjectID:      "550e8400-e29b-41d4-a716-446655440001",
		Status:         "ready",
		FileCount:      120,
		ChunkCount:     850,
		EmbeddingModel: "text-embedding-3-small",
		Error:          "",
		Incremental:    true,
		FilesChanged:   5,
		FilesUnchanged: 115,
	}
}

func sampleRetrievalSearchRequestPayload() mq.RetrievalSearchRequestPayload {
	return mq.RetrievalSearchRequestPayload{
		ProjectID:      "550e8400-e29b-41d4-a716-446655440001",
		Query:          "LRU cache implementation",
		RequestID:      "550e8400-e29b-41d4-a716-446655440009",
		TopK:           10,
		BM25Weight:     0.4,
		SemanticWeight: 0.6,
		ScopeID:        "scope-backend",
	}
}

func sampleRetrievalSearchResultPayload() mq.RetrievalSearchResultPayload {
	return mq.RetrievalSearchResultPayload{
		ProjectID: "550e8400-e29b-41d4-a716-446655440001",
		Query:     "LRU cache implementation",
		RequestID: "550e8400-e29b-41d4-a716-446655440009",
		Results:   []mq.RetrievalSearchHitPayload{sampleRetrievalSearchHitPayload()},
		Error:     "",
	}
}

func sampleSubAgentSearchRequestPayload() mq.SubAgentSearchRequestPayload {
	return mq.SubAgentSearchRequestPayload{
		ProjectID:       "550e8400-e29b-41d4-a716-446655440001",
		Query:           "How is caching handled?",
		RequestID:       "550e8400-e29b-41d4-a716-446655440011",
		TopK:            10,
		MaxQueries:      5,
		Model:           "openai/gpt-4o",
		Rerank:          true,
		ScopeID:         "scope-backend",
		ExpansionPrompt: "Generate related search queries for code retrieval.",
	}
}

func sampleSubAgentSearchResultPayload() mq.SubAgentSearchResultPayload {
	return mq.SubAgentSearchResultPayload{
		ProjectID:       "550e8400-e29b-41d4-a716-446655440001",
		Query:           "How is caching handled?",
		RequestID:       "550e8400-e29b-41d4-a716-446655440011",
		Results:         []mq.RetrievalSearchHitPayload{sampleRetrievalSearchHitPayload()},
		ExpandedQueries: []string{"cache eviction policy", "LRU implementation", "cache invalidation"},
		TotalCandidates: 42,
		Error:           "",
		Model:           "openai/gpt-4o",
		TokensIn:        500,
		TokensOut:       250,
		CostUSD:         0.008,
	}
}

func sampleGraphBuildRequestPayload() mq.GraphBuildRequestPayload {
	return mq.GraphBuildRequestPayload{
		ProjectID:     "550e8400-e29b-41d4-a716-446655440001",
		WorkspacePath: "/workspaces/my-project",
		ScopeID:       "scope-backend",
	}
}

func sampleGraphBuildResultPayload() mq.GraphBuildResultPayload {
	return mq.GraphBuildResultPayload{
		ProjectID: "550e8400-e29b-41d4-a716-446655440001",
		Status:    "ready",
		NodeCount: 350,
		EdgeCount: 1200,
		Languages: []string{"go", "python"},
		Error:     "",
	}
}

func sampleGraphSearchRequestPayload() mq.GraphSearchRequestPayload {
	return mq.GraphSearchRequestPayload{
		ProjectID:   "550e8400-e29b-41d4-a716-446655440001",
		RequestID:   "550e8400-e29b-41d4-a716-446655440012",
		SeedSymbols: []string{"LRU", "Cache", "Get"},
		MaxHops:     3,
		TopK:        10,
		ScopeID:     "scope-backend",
	}
}

func sampleGraphSearchResultPayload() mq.GraphSearchResultPayload {
	return mq.GraphSearchResultPayload{
		ProjectID: "550e8400-e29b-41d4-a716-446655440001",
		RequestID: "550e8400-e29b-41d4-a716-446655440012",
		Results:   []mq.GraphSearchHitPayload{sampleGraphSearchHitPayload()},
		Error:     "",
	}
}

func sampleA2ATaskCreatedPayload() mq.A2ATaskCreatedPayload {
	return mq.A2ATaskCreatedPayload{
		TaskID:   "550e8400-e29b-41d4-a716-446655440013",
		TenantID: "550e8400-e29b-41d4-a716-446655440006",
		SkillID:  "code-review",
		Prompt:   "Review the authentication module for security issues.",
	}
}

func sampleA2ATaskCompletePayload() mq.A2ATaskCompletePayload {
	return mq.A2ATaskCompletePayload{
		TaskID:   "550e8400-e29b-41d4-a716-446655440013",
		TenantID: "550e8400-e29b-41d4-a716-446655440006",
		State:    "COMPLETED",
		Error:    "",
	}
}

// --- Fixture generation and round-trip tests ---

// fixtureEntry maps a NATS subject to its sample payload for fixture generation.
type fixtureEntry struct {
	Subject string
	Payload any
}

func allFixtures() []fixtureEntry {
	return []fixtureEntry{
		{mq.SubjectConversationRunStart, sampleConversationRunStartPayload()},
		{mq.SubjectConversationRunComplete, sampleConversationRunCompletePayload()},
		{mq.SubjectBenchmarkRunRequest, sampleBenchmarkRunRequestPayload()},
		{mq.SubjectBenchmarkRunResult, sampleBenchmarkRunResultPayload()},
		{mq.SubjectBenchmarkTaskStarted, sampleBenchmarkTaskStartedPayload()},
		{mq.SubjectBenchmarkTaskProgress, sampleBenchmarkTaskProgressPayload()},
		{mq.SubjectEvalGemmasRequest, sampleGemmasEvalRequestPayload()},
		{mq.SubjectEvalGemmasResult, sampleGemmasEvalResultPayload()},
		{mq.SubjectRepoMapRequest, sampleRepoMapRequestPayload()},
		{mq.SubjectRepoMapResult, sampleRepoMapResultPayload()},
		{mq.SubjectRetrievalIndexRequest, sampleRetrievalIndexRequestPayload()},
		{mq.SubjectRetrievalIndexResult, sampleRetrievalIndexResultPayload()},
		{mq.SubjectRetrievalSearchRequest, sampleRetrievalSearchRequestPayload()},
		{mq.SubjectRetrievalSearchResult, sampleRetrievalSearchResultPayload()},
		{mq.SubjectSubAgentSearchRequest, sampleSubAgentSearchRequestPayload()},
		{mq.SubjectSubAgentSearchResult, sampleSubAgentSearchResultPayload()},
		{mq.SubjectGraphBuildRequest, sampleGraphBuildRequestPayload()},
		{mq.SubjectGraphBuildResult, sampleGraphBuildResultPayload()},
		{mq.SubjectGraphSearchRequest, sampleGraphSearchRequestPayload()},
		{mq.SubjectGraphSearchResult, sampleGraphSearchResultPayload()},
		{mq.SubjectA2ATaskCreated, sampleA2ATaskCreatedPayload()},
		{mq.SubjectA2ATaskComplete, sampleA2ATaskCompletePayload()},
	}
}

func TestContract_GenerateFixtures(t *testing.T) {
	dir := filepath.Join("testdata", "contracts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create contracts dir: %v", err)
	}

	for _, fx := range allFixtures() {
		data, err := json.MarshalIndent(fx.Payload, "", "  ")
		if err != nil {
			t.Fatalf("marshal %s: %v", fx.Subject, err)
		}

		// Replace dots with underscores for filename safety.
		filename := subjectToFilename(fx.Subject) + ".json"
		path := filepath.Join(dir, filename)

		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}

		t.Logf("wrote %s (%d bytes)", path, len(data))
	}
}

func TestContract_RoundTrip(t *testing.T) {
	for _, fx := range allFixtures() {
		t.Run(fx.Subject, func(t *testing.T) {
			// Marshal to JSON.
			data, err := json.Marshal(fx.Payload)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			// Unmarshal into a generic map to verify key fields exist.
			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("unmarshal to map: %v", err)
			}

			// Every payload must produce at least one key.
			if len(m) == 0 {
				t.Fatal("round-trip produced empty map")
			}

			// Verify subject-specific key fields.
			verifyKeyFields(t, fx.Subject, m)
		})
	}
}

// verifyKeyFields checks that expected key fields exist in the deserialized map.
func verifyKeyFields(t *testing.T, subject string, m map[string]any) {
	t.Helper()

	// Common field expectations per subject.
	expectedKeys := map[string][]string{
		mq.SubjectConversationRunStart:    {"run_id", "conversation_id", "project_id", "messages", "model", "agentic"},
		mq.SubjectConversationRunComplete: {"run_id", "conversation_id", "assistant_content", "status", "cost_usd", "model"},
		mq.SubjectBenchmarkRunRequest:     {"run_id", "dataset_path", "model"},
		mq.SubjectBenchmarkRunResult:      {"run_id", "status", "results", "summary"},
		mq.SubjectBenchmarkTaskStarted:    {"run_id", "task_id", "task_name", "index", "total"},
		mq.SubjectBenchmarkTaskProgress:   {"run_id", "task_id", "completed_tasks", "total_tasks"},
		mq.SubjectEvalGemmasRequest:       {"plan_id", "messages"},
		mq.SubjectEvalGemmasResult:        {"plan_id", "information_diversity_score", "unnecessary_path_ratio"},
		mq.SubjectRepoMapRequest:          {"project_id", "workspace_path", "token_budget"},
		mq.SubjectRepoMapResult:           {"project_id", "map_text", "token_count", "file_count"},
		mq.SubjectRetrievalIndexRequest:   {"project_id", "workspace_path", "embedding_model"},
		mq.SubjectRetrievalIndexResult:    {"project_id", "status", "file_count", "chunk_count"},
		mq.SubjectRetrievalSearchRequest:  {"project_id", "query", "request_id", "top_k"},
		mq.SubjectRetrievalSearchResult:   {"project_id", "query", "request_id", "results"},
		mq.SubjectSubAgentSearchRequest:   {"project_id", "query", "request_id", "model"},
		mq.SubjectSubAgentSearchResult:    {"project_id", "query", "request_id", "results", "expanded_queries"},
		mq.SubjectGraphBuildRequest:       {"project_id", "workspace_path"},
		mq.SubjectGraphBuildResult:        {"project_id", "status", "node_count", "edge_count"},
		mq.SubjectGraphSearchRequest:      {"project_id", "request_id", "seed_symbols", "max_hops"},
		mq.SubjectGraphSearchResult:       {"project_id", "request_id", "results"},
		mq.SubjectA2ATaskCreated:          {"task_id", "tenant_id", "skill_id", "prompt"},
		mq.SubjectA2ATaskComplete:         {"task_id", "state"},
	}

	keys, ok := expectedKeys[subject]
	if !ok {
		t.Fatalf("no expected keys defined for subject %s", subject)
	}

	for _, key := range keys {
		if _, exists := m[key]; !exists {
			t.Errorf("missing expected key %q in %s payload", key, subject)
		}
	}
}

// subjectToFilename converts a NATS subject like "conversation.run.start" to "conversation_run_start".
func subjectToFilename(subject string) string {
	out := make([]byte, len(subject))
	for i := range subject {
		if subject[i] == '.' {
			out[i] = '_'
		} else {
			out[i] = subject[i]
		}
	}
	return string(out)
}
