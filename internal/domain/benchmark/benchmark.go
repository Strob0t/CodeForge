// Package benchmark defines domain types for the benchmark evaluation system.
// Benchmarks measure agent quality by running evaluation tasks with configurable
// metrics (DeepEval, AgentNeo, GEMMAS) and recording per-task scores.
package benchmark

import (
	"encoding/json"
	"fmt"
	"time"
)

// RunStatus represents the lifecycle state of a benchmark run.
type RunStatus string

const (
	StatusRunning   RunStatus = "running"
	StatusCompleted RunStatus = "completed"
	StatusFailed    RunStatus = "failed"
)

// BenchmarkType distinguishes the three benchmark evaluation modes.
type BenchmarkType string

const (
	TypeSimple  BenchmarkType = "simple"
	TypeToolUse BenchmarkType = "tool_use"
	TypeAgent   BenchmarkType = "agent"
)

// IsValid returns true if the benchmark type is one of the known values.
func (t BenchmarkType) IsValid() bool {
	switch t {
	case TypeSimple, TypeToolUse, TypeAgent:
		return true
	}
	return false
}

// ExecMode defines how an agent benchmark task is executed.
type ExecMode string

const (
	ExecModeMount   ExecMode = "mount"
	ExecModeSandbox ExecMode = "sandbox"
	ExecModeHybrid  ExecMode = "hybrid"
)

// IsValid returns true if the exec mode is one of the known values.
func (m ExecMode) IsValid() bool {
	switch m {
	case ExecModeMount, ExecModeSandbox, ExecModeHybrid:
		return true
	}
	return false
}

// Suite represents a registered benchmark suite (e.g. HumanEval, SWE-bench).
type Suite struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Type         BenchmarkType   `json:"type"`
	ProviderName string          `json:"provider_name"`
	TaskCount    int             `json:"task_count"`
	Config       json.RawMessage `json:"config,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// CreateSuiteRequest is the payload for registering a new benchmark suite.
type CreateSuiteRequest struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Type         BenchmarkType   `json:"type"`
	ProviderName string          `json:"provider_name"`
	Config       json.RawMessage `json:"config,omitempty"`
}

// Validate checks required fields on a CreateSuiteRequest.
func (r *CreateSuiteRequest) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("name is required")
	}
	if !r.Type.IsValid() {
		return fmt.Errorf("invalid benchmark type: %q", r.Type)
	}
	if r.ProviderName == "" {
		return fmt.Errorf("provider_name is required")
	}
	return nil
}

// Run is a single benchmark execution against a dataset with a specific model.
type Run struct {
	ID              string          `json:"id"`
	Dataset         string          `json:"dataset"`
	Model           string          `json:"model"`
	Metrics         []string        `json:"metrics"`
	Status          RunStatus       `json:"status"`
	SummaryScores   json.RawMessage `json:"summary_scores"`
	TotalCost       float64         `json:"total_cost"`
	TotalTokens     int             `json:"total_tokens"`
	TotalDurationMs int64           `json:"total_duration_ms"`
	CreatedAt       time.Time       `json:"created_at"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`

	// Phase 26 fields (nullable for backward compatibility).
	SuiteID       string          `json:"suite_id,omitempty"`
	BenchmarkType BenchmarkType   `json:"benchmark_type,omitempty"`
	ExecMode      ExecMode        `json:"exec_mode,omitempty"`
	Config        json.RawMessage `json:"config,omitempty"`
}

// Result stores evaluation output for a single task within a benchmark run.
type Result struct {
	ID             string          `json:"id"`
	RunID          string          `json:"run_id"`
	TaskID         string          `json:"task_id"`
	TaskName       string          `json:"task_name"`
	Scores         json.RawMessage `json:"scores"`
	ActualOutput   string          `json:"actual_output"`
	ExpectedOutput string          `json:"expected_output"`
	ToolCalls      json.RawMessage `json:"tool_calls"`
	CostUSD        float64         `json:"cost_usd"`
	TokensIn       int             `json:"tokens_in"`
	TokensOut      int             `json:"tokens_out"`
	DurationMs     int64           `json:"duration_ms"`

	// Phase 26 fields.
	EvaluatorScores      json.RawMessage `json:"evaluator_scores,omitempty"`
	FilesChanged         []string        `json:"files_changed,omitempty"`
	FunctionalTestOutput string          `json:"functional_test_output,omitempty"`
}

// CreateRunRequest is the payload for creating a new benchmark run.
type CreateRunRequest struct {
	Dataset       string        `json:"dataset"`
	SuiteID       string        `json:"suite_id,omitempty"`
	Model         string        `json:"model"`
	Metrics       []string      `json:"metrics"`
	BenchmarkType BenchmarkType `json:"benchmark_type,omitempty"`
	ExecMode      ExecMode      `json:"exec_mode,omitempty"`
}

// Validate checks required fields on a CreateRunRequest.
func (r *CreateRunRequest) Validate() error {
	if r.Dataset == "" && r.SuiteID == "" {
		return fmt.Errorf("dataset or suite_id is required")
	}
	if r.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(r.Metrics) == 0 {
		return fmt.Errorf("at least one metric is required")
	}
	if r.BenchmarkType != "" && !r.BenchmarkType.IsValid() {
		return fmt.Errorf("invalid benchmark type: %q", r.BenchmarkType)
	}
	if r.ExecMode != "" && !r.ExecMode.IsValid() {
		return fmt.Errorf("invalid exec mode: %q", r.ExecMode)
	}
	return nil
}

// CompareRequest specifies two runs to compare side-by-side.
type CompareRequest struct {
	RunIDA string `json:"run_id_a"`
	RunIDB string `json:"run_id_b"`
}

// CompareResult holds the side-by-side comparison output.
type CompareResult struct {
	RunA    *Run     `json:"run_a"`
	RunB    *Run     `json:"run_b"`
	ResultA []Result `json:"results_a"`
	ResultB []Result `json:"results_b"`
}

// MultiCompareRequest specifies N runs for multi-way comparison.
type MultiCompareRequest struct {
	RunIDs []string `json:"run_ids"`
}

// MultiCompareEntry holds one run and its results for multi-comparison.
type MultiCompareEntry struct {
	Run     *Run     `json:"run"`
	Results []Result `json:"results"`
}

// RunFilter constrains which benchmark runs to return.
type RunFilter struct {
	SuiteID       string        `json:"suite_id,omitempty"`
	BenchmarkType BenchmarkType `json:"benchmark_type,omitempty"`
	Model         string        `json:"model,omitempty"`
}

// DatasetInfo describes an available benchmark dataset.
type DatasetInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	TaskCount   int    `json:"task_count"`
	Path        string `json:"path"`
}

// --- Phase 26G: Cost Analysis, Leaderboard, Benchmark Progress ---

// CostBreakdown holds cost details for a single task in a benchmark run.
type CostBreakdown struct {
	TaskID    string  `json:"task_id"`
	TaskName  string  `json:"task_name"`
	CostUSD   float64 `json:"cost_usd"`
	TokensIn  int     `json:"tokens_in"`
	TokensOut int     `json:"tokens_out"`
	Score     float64 `json:"score"`
}

// CostAnalysis aggregates cost metrics for a benchmark run or suite.
type CostAnalysis struct {
	RunID             string          `json:"run_id"`
	Model             string          `json:"model"`
	SuiteID           string          `json:"suite_id,omitempty"`
	TotalCostUSD      float64         `json:"total_cost_usd"`
	TotalTokensIn     int             `json:"total_tokens_in"`
	TotalTokensOut    int             `json:"total_tokens_out"`
	AvgScore          float64         `json:"avg_score"`
	CostPerScorePoint float64         `json:"cost_per_score_point"`
	TokenEfficiency   float64         `json:"token_efficiency"`
	TaskBreakdown     []CostBreakdown `json:"task_breakdown"`
}

// LeaderboardEntry represents one model's performance on a specific suite.
type LeaderboardEntry struct {
	Model             string  `json:"model"`
	RunID             string  `json:"run_id"`
	SuiteID           string  `json:"suite_id,omitempty"`
	AvgScore          float64 `json:"avg_score"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	TotalTokensIn     int     `json:"total_tokens_in"`
	TotalTokensOut    int     `json:"total_tokens_out"`
	TaskCount         int     `json:"task_count"`
	CostPerScorePoint float64 `json:"cost_per_score_point"`
	TokenEfficiency   float64 `json:"token_efficiency"`
	DurationMs        int64   `json:"duration_ms"`
}
