package messagequeue

import "encoding/json"

// --- GEMMAS Evaluation payloads (Phase 20G) ---

// GemmasAgentMessagePayload represents a single agent message for GEMMAS evaluation.
type GemmasAgentMessagePayload struct {
	AgentID       string `json:"agent_id"`
	Content       string `json:"content"`
	Round         int    `json:"round"`
	ParentAgentID string `json:"parent_agent_id,omitempty"`
}

// GemmasEvalRequestPayload is published to request GEMMAS metric computation.
type GemmasEvalRequestPayload struct {
	PlanID   string                      `json:"plan_id"`
	Messages []GemmasAgentMessagePayload `json:"messages"`
}

// GemmasEvalResultPayload is published with GEMMAS metric results.
type GemmasEvalResultPayload struct {
	PlanID                    string  `json:"plan_id"`
	InformationDiversityScore float64 `json:"information_diversity_score"`
	UnnecessaryPathRatio      float64 `json:"unnecessary_path_ratio"`
	Error                     string  `json:"error,omitempty"`
}

// --- Benchmark run payloads (Phase 26/28) ---

// BenchmarkRunRequestPayload is published to trigger benchmark execution in Python.
type BenchmarkRunRequestPayload struct {
	RunID              string          `json:"run_id"`
	TenantID           string          `json:"tenant_id,omitempty"`
	DatasetPath        string          `json:"dataset_path"`
	Model              string          `json:"model"`
	Metrics            []string        `json:"metrics,omitempty"`
	BenchmarkType      string          `json:"benchmark_type,omitempty"`
	SuiteID            string          `json:"suite_id,omitempty"`
	ExecMode           string          `json:"exec_mode,omitempty"`
	Evaluators         []string        `json:"evaluators,omitempty"`
	HybridVerification bool            `json:"hybrid_verification,omitempty"`
	RolloutCount       int             `json:"rollout_count,omitempty"`
	RolloutStrategy    string          `json:"rollout_strategy,omitempty"`
	ProviderName       string          `json:"provider_name,omitempty"`
	ProviderConfig     json.RawMessage `json:"provider_config,omitempty"`
}

// BenchmarkSummary holds aggregate statistics computed by the Python worker.
type BenchmarkSummary struct {
	TaskCount      int     `json:"task_count"`
	AvgScore       float64 `json:"avg_score"`
	TotalCostUSD   float64 `json:"total_cost_usd"`
	TotalTokensIn  int64   `json:"total_tokens_in"`
	TotalTokensOut int64   `json:"total_tokens_out"`
	ElapsedMs      int64   `json:"elapsed_ms"`
}

// BenchmarkRunResultPayload is published by Python when benchmark execution completes.
type BenchmarkRunResultPayload struct {
	RunID           string                `json:"run_id"`
	TenantID        string                `json:"tenant_id,omitempty"`
	Status          string                `json:"status"`
	Results         []BenchmarkTaskResult `json:"results"`
	Summary         BenchmarkSummary      `json:"summary"`
	TotalCost       float64               `json:"total_cost"`
	TotalTokens     int64                 `json:"total_tokens"`
	TotalDurationMs int64                 `json:"total_duration_ms"`
	Error           string                `json:"error,omitempty"`
}

// BenchmarkTaskResult represents a single task's evaluation outcome.
type BenchmarkTaskResult struct {
	TaskID               string                        `json:"task_id"`
	TaskName             string                        `json:"task_name"`
	Scores               map[string]float64            `json:"scores"`
	ActualOutput         string                        `json:"actual_output"`
	ExpectedOutput       string                        `json:"expected_output"`
	ToolCalls            []map[string]string           `json:"tool_calls"`
	CostUSD              float64                       `json:"cost_usd"`
	TokensIn             int64                         `json:"tokens_in"`
	TokensOut            int64                         `json:"tokens_out"`
	DurationMs           int64                         `json:"duration_ms"`
	EvaluatorScores      map[string]map[string]float64 `json:"evaluator_scores,omitempty"`
	FilesChanged         []string                      `json:"files_changed,omitempty"`
	FunctionalTestOutput string                        `json:"functional_test_output,omitempty"`
	RolloutID            int                           `json:"rollout_id"`
	RolloutCount         int                           `json:"rollout_count"`
	IsBestRollout        bool                          `json:"is_best_rollout"`
	DiversityScore       float64                       `json:"diversity_score"`
	SelectedModel        string                        `json:"selected_model,omitempty"`
	RoutingReason        string                        `json:"routing_reason,omitempty"`
	FallbackChain        string                        `json:"fallback_chain,omitempty"`
	FallbackCount        int                           `json:"fallback_count,omitempty"`
	ProviderErrors       string                        `json:"provider_errors,omitempty"`
}

// BenchmarkTaskStartedPayload is published by Python when a benchmark task begins.
type BenchmarkTaskStartedPayload struct {
	RunID    string `json:"run_id"`
	TaskID   string `json:"task_id"`
	TaskName string `json:"task_name"`
	Index    int    `json:"index"`
	Total    int    `json:"total"`
}

// BenchmarkTaskProgressPayload is published by Python when a benchmark task completes.
type BenchmarkTaskProgressPayload struct {
	RunID          string  `json:"run_id"`
	TaskID         string  `json:"task_id"`
	TaskName       string  `json:"task_name"`
	Score          float64 `json:"score"`
	CostUSD        float64 `json:"cost_usd"`
	CompletedTasks int     `json:"completed_tasks"`
	TotalTasks     int     `json:"total_tasks"`
	AvgScore       float64 `json:"avg_score"`
	TotalCostUSD   float64 `json:"total_cost_usd"`
}
