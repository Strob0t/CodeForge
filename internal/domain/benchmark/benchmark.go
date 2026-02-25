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
}

// CreateRunRequest is the payload for creating a new benchmark run.
type CreateRunRequest struct {
	Dataset string   `json:"dataset"`
	Model   string   `json:"model"`
	Metrics []string `json:"metrics"`
}

// Validate checks required fields on a CreateRunRequest.
func (r *CreateRunRequest) Validate() error {
	if r.Dataset == "" {
		return fmt.Errorf("dataset is required")
	}
	if r.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(r.Metrics) == 0 {
		return fmt.Errorf("at least one metric is required")
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

// DatasetInfo describes an available benchmark dataset.
type DatasetInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	TaskCount   int    `json:"task_count"`
	Path        string `json:"path"`
}
