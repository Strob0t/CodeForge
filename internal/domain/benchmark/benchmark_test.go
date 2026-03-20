package benchmark_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

func TestBenchmarkType_Valid(t *testing.T) {
	tests := []struct {
		typ  benchmark.BenchmarkType
		want bool
	}{
		{benchmark.TypeSimple, true},
		{benchmark.TypeToolUse, true},
		{benchmark.TypeAgent, true},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.typ.IsValid(); got != tt.want {
			t.Errorf("BenchmarkType(%q).IsValid() = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestExecMode_Valid(t *testing.T) {
	tests := []struct {
		mode benchmark.ExecMode
		want bool
	}{
		{benchmark.ExecModeMount, true},
		{benchmark.ExecModeSandbox, true},
		{benchmark.ExecModeHybrid, true},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.mode.IsValid(); got != tt.want {
			t.Errorf("ExecMode(%q).IsValid() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestCreateSuiteRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     benchmark.CreateSuiteRequest
		wantErr bool
	}{
		{
			name: "valid",
			req: benchmark.CreateSuiteRequest{
				Name:         "HumanEval",
				Type:         benchmark.TypeSimple,
				ProviderName: "humaneval",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			req: benchmark.CreateSuiteRequest{
				Type:         benchmark.TypeSimple,
				ProviderName: "humaneval",
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			req: benchmark.CreateSuiteRequest{
				Name:         "Test",
				Type:         "invalid",
				ProviderName: "test",
			},
			wantErr: true,
		},
		{
			name: "missing provider",
			req: benchmark.CreateSuiteRequest{
				Name: "Test",
				Type: benchmark.TypeAgent,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateRunRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     benchmark.CreateRunRequest
		wantErr bool
	}{
		{
			name: "valid with dataset",
			req: benchmark.CreateRunRequest{
				Dataset: "basic-coding",
				Model:   "gpt-4",
				Metrics: []string{"correctness"},
			},
			wantErr: false,
		},
		{
			name: "valid with suite_id",
			req: benchmark.CreateRunRequest{
				Dataset: "basic-coding",
				SuiteID: "suite-123",
				Model:   "claude-3",
				Metrics: []string{"correctness"},
			},
			wantErr: false,
		},
		{
			name: "missing dataset and suite_id",
			req: benchmark.CreateRunRequest{
				Model:   "gpt-4",
				Metrics: []string{"correctness"},
			},
			wantErr: true,
		},
		{
			name: "missing model",
			req: benchmark.CreateRunRequest{
				Dataset: "basic-coding",
				Metrics: []string{"correctness"},
			},
			wantErr: true,
		},
		{
			name: "no metrics",
			req: benchmark.CreateRunRequest{
				Dataset: "basic-coding",
				Model:   "gpt-4",
			},
			wantErr: true,
		},
		{
			name: "invalid benchmark type",
			req: benchmark.CreateRunRequest{
				Dataset:       "basic-coding",
				Model:         "gpt-4",
				Metrics:       []string{"correctness"},
				BenchmarkType: "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid exec mode",
			req: benchmark.CreateRunRequest{
				Dataset:  "basic-coding",
				Model:    "gpt-4",
				Metrics:  []string{"correctness"},
				ExecMode: "invalid",
			},
			wantErr: true,
		},
		{
			name: "valid with all phase 26 fields",
			req: benchmark.CreateRunRequest{
				Dataset:       "basic-coding",
				SuiteID:       "suite-1",
				Model:         "claude-3",
				Metrics:       []string{"correctness", "trajectory_verifier"},
				BenchmarkType: benchmark.TypeAgent,
				ExecMode:      benchmark.ExecModeSandbox,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSuite_JSONRoundTrip(t *testing.T) {
	s := benchmark.Suite{
		ID:           "suite-1",
		Name:         "HumanEval",
		Description:  "164 Python function tasks",
		Type:         benchmark.TypeSimple,
		ProviderName: "humaneval",
		TaskCount:    164,
		Config:       json.RawMessage(`{"language":"python"}`),
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}

	var got benchmark.Suite
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	if got.ID != s.ID || got.Name != s.Name || got.Type != s.Type ||
		got.ProviderName != s.ProviderName || got.TaskCount != s.TaskCount {
		t.Errorf("JSON roundtrip mismatch: got %+v", got)
	}
}

func TestRun_Phase26FieldsBackwardCompatible(t *testing.T) {
	// A run without Phase 26 fields should still serialize/deserialize correctly.
	r := benchmark.Run{
		ID:      "run-1",
		Dataset: "basic-coding",
		Model:   "gpt-4",
		Metrics: []string{"correctness"},
		Status:  benchmark.StatusRunning,
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}

	var got benchmark.Run
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	if got.SuiteID != "" || got.BenchmarkType != "" || got.ExecMode != "" {
		t.Errorf("Phase 26 fields should be empty for legacy runs: suite=%q type=%q exec=%q",
			got.SuiteID, got.BenchmarkType, got.ExecMode)
	}
}

func TestResult_Phase26Fields(t *testing.T) {
	r := benchmark.Result{
		ID:                   "res-1",
		RunID:                "run-1",
		TaskID:               "task-1",
		TaskName:             "FizzBuzz",
		EvaluatorScores:      json.RawMessage(`{"llm_judge":{"correctness":0.95},"functional":{"pass_rate":1.0}}`),
		FilesChanged:         []string{"main.py", "test_main.py"},
		FunctionalTestOutput: "2 passed, 0 failed",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}

	var got benchmark.Result
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	if len(got.FilesChanged) != 2 {
		t.Errorf("expected 2 files changed, got %d", len(got.FilesChanged))
	}
	if got.FunctionalTestOutput != "2 passed, 0 failed" {
		t.Errorf("unexpected functional test output: %s", got.FunctionalTestOutput)
	}
}

func TestCreateRunRequest_Validate_ReturnsErrValidation(t *testing.T) {
	tests := []struct {
		name string
		req  benchmark.CreateRunRequest
		msg  string
	}{
		{
			name: "missing dataset and suite_id",
			req:  benchmark.CreateRunRequest{Model: "gpt-4", Metrics: []string{"llm_judge"}},
			msg:  "dataset or suite_id is required",
		},
		{
			name: "missing model",
			req:  benchmark.CreateRunRequest{Dataset: "foo", Metrics: []string{"llm_judge"}},
			msg:  "model is required",
		},
		{
			name: "missing metrics",
			req:  benchmark.CreateRunRequest{Dataset: "foo", Model: "gpt-4"},
			msg:  "at least one metric is required",
		},
		{
			name: "invalid benchmark type",
			req:  benchmark.CreateRunRequest{Dataset: "foo", Model: "gpt-4", Metrics: []string{"llm_judge"}, BenchmarkType: "invalid"},
			msg:  "invalid benchmark type",
		},
		{
			name: "invalid exec mode",
			req:  benchmark.CreateRunRequest{Dataset: "foo", Model: "gpt-4", Metrics: []string{"llm_judge"}, ExecMode: "invalid"},
			msg:  "invalid exec mode",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, domain.ErrValidation) {
				t.Errorf("expected ErrValidation, got: %v", err)
			}
			if !strings.Contains(err.Error(), tt.msg) {
				t.Errorf("expected message containing %q, got: %v", tt.msg, err)
			}
		})
	}
}

func TestCreateRunRequest_Validate_UnknownMetric(t *testing.T) {
	tests := []struct {
		name    string
		metrics []string
		wantErr bool
	}{
		{"valid single", []string{"llm_judge"}, false},
		{"valid multiple", []string{"llm_judge", "functional_test", "sparc"}, false},
		{"valid all", []string{"llm_judge", "functional_test", "sparc", "trajectory_verifier"}, false},
		{"valid sub-metric", []string{"correctness", "faithfulness"}, false},
		{"unknown metric", []string{"nonexistent_evaluator"}, true},
		{"one valid one invalid", []string{"llm_judge", "invalid"}, true},
		{"empty string metric", []string{""}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := benchmark.CreateRunRequest{
				Dataset: "basic-coding",
				Model:   "gpt-4",
				Metrics: tt.metrics,
			}
			err := req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, domain.ErrValidation) {
				t.Errorf("expected ErrValidation, got: %v", err)
			}
		})
	}
}

func TestProviderDefaultType(t *testing.T) {
	tests := []struct {
		provider string
		want     benchmark.BenchmarkType
	}{
		// Simple providers.
		{"codeforge_simple", benchmark.TypeSimple},
		{"humaneval", benchmark.TypeSimple},
		{"mbpp", benchmark.TypeSimple},
		{"bigcodebench", benchmark.TypeSimple},
		{"cruxeval", benchmark.TypeSimple},
		{"livecodebench", benchmark.TypeSimple},
		{"dpai_arena", benchmark.TypeSimple},
		// Agent providers.
		{"codeforge_agent", benchmark.TypeAgent},
		{"swebench", benchmark.TypeAgent},
		{"sparcbench", benchmark.TypeAgent},
		{"aider_polyglot", benchmark.TypeAgent},
		{"terminal_bench", benchmark.TypeAgent},
		// Tool-use providers.
		{"codeforge_tool_use", benchmark.TypeToolUse},
		// Unknown providers return empty string.
		{"unknown_provider", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := benchmark.ProviderDefaultType(tt.provider)
			if got != tt.want {
				t.Errorf("ProviderDefaultType(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

func TestMultiCompareRequest_Validate(t *testing.T) {
	req := benchmark.MultiCompareRequest{
		RunIDs: []string{"run-1", "run-2", "run-3"},
	}
	if len(req.RunIDs) != 3 {
		t.Errorf("expected 3 run IDs, got %d", len(req.RunIDs))
	}
}
