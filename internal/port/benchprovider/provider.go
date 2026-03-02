// Package benchprovider defines the port interface for benchmark suite providers.
// Each provider knows how to load tasks and evaluate results for a specific
// benchmark (e.g. HumanEval, SWE-bench, SPARC-bench).
//
// Providers self-register via init() using the Register() function,
// following the same pattern as gitprovider.
package benchprovider

import (
	"context"
	"fmt"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

// TaskSpec describes a single benchmark task as provided by the benchmark suite.
type TaskSpec struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Input          string            `json:"input"`
	ExpectedOutput string            `json:"expected_output,omitempty"`
	ExpectedTools  []ToolCall        `json:"expected_tools,omitempty"`
	Context        []string          `json:"context,omitempty"`
	Difficulty     string            `json:"difficulty,omitempty"`
	InitialFiles   map[string]string `json:"initial_files,omitempty"`
	TestCommand    string            `json:"test_command,omitempty"`
	RepoURL        string            `json:"repo_url,omitempty"`
	RepoCommit     string            `json:"repo_commit,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// ToolCall represents an expected tool invocation.
type ToolCall struct {
	Name string `json:"name"`
	Args string `json:"args,omitempty"`
}

// ExecutionResult captures the output of running a benchmark task.
type ExecutionResult struct {
	ActualOutput string            `json:"actual_output"`
	ToolCalls    []ToolCall        `json:"tool_calls,omitempty"`
	FilesChanged []string          `json:"files_changed,omitempty"`
	TestOutput   string            `json:"test_output,omitempty"`
	ExitCode     int               `json:"exit_code"`
	CostUSD      float64           `json:"cost_usd"`
	TokensIn     int               `json:"tokens_in"`
	TokensOut    int               `json:"tokens_out"`
	DurationMs   int64             `json:"duration_ms"`
	StepCount    int               `json:"step_count"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// EvalDimension is a single named score from an evaluator.
type EvalDimension struct {
	Name    string            `json:"name"`
	Score   float64           `json:"score"`
	Details map[string]string `json:"details,omitempty"`
	CostUSD float64           `json:"cost_usd"`
}

// EvalScore aggregates all evaluation dimensions for a task.
type EvalScore struct {
	Dimensions        []EvalDimension `json:"dimensions"`
	TotalCostUSD      float64         `json:"total_cost_usd"`
	CostPerScorePoint float64         `json:"cost_per_score_point"`
	TokenEfficiency   float64         `json:"token_efficiency"`
}

// AverageScore computes the mean score across all dimensions.
func (s EvalScore) AverageScore() float64 {
	if len(s.Dimensions) == 0 {
		return 0
	}
	var sum float64
	for _, d := range s.Dimensions {
		sum += d.Score
	}
	return sum / float64(len(s.Dimensions))
}

// Capabilities declares which evaluation methods a provider supports.
type Capabilities struct {
	FunctionalTests bool `json:"functional_tests"`
	LLMJudge        bool `json:"llm_judge"`
	SWEBenchStyle   bool `json:"swe_bench_style"`
	SPARCStyle      bool `json:"sparc_style"`
}

// Provider is the interface that benchmark suite adapters must implement.
type Provider interface {
	// Name returns the unique identifier for this provider (e.g. "humaneval", "swe-bench").
	Name() string

	// Type returns the benchmark type this provider targets.
	Type() benchmark.BenchmarkType

	// Capabilities returns which evaluation methods this provider supports.
	Capabilities() Capabilities

	// ListTasks returns all tasks available in this benchmark suite.
	ListTasks(ctx context.Context) ([]TaskSpec, error)

	// TaskCount returns the number of tasks without loading all task data.
	TaskCount(ctx context.Context) (int, error)
}

// Factory is a constructor function that creates a new Provider instance.
type Factory func(config map[string]string) (Provider, error)

var (
	mu        sync.RWMutex
	factories = make(map[string]Factory)
)

// Register makes a benchmark provider factory available by name.
// It is typically called from an init() function in the adapter package.
func Register(name string, factory Factory) {
	mu.Lock()
	defer mu.Unlock()

	if _, exists := factories[name]; exists {
		panic(fmt.Sprintf("benchprovider: duplicate registration for %q", name))
	}
	factories[name] = factory
}

// New creates a new Provider by name using the registered factory.
func New(name string, config map[string]string) (Provider, error) {
	mu.RLock()
	factory, ok := factories[name]
	mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("benchprovider: unknown provider %q", name)
	}
	return factory(config)
}

// Available returns the names of all registered providers.
func Available() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	return names
}
