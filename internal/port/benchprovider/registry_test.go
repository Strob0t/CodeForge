package benchprovider_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/port/benchprovider"
)

type testProvider struct {
	providerName string
}

func (p *testProvider) Name() string                  { return p.providerName }
func (p *testProvider) Type() benchmark.BenchmarkType { return benchmark.TypeSimple }
func (p *testProvider) Capabilities() benchprovider.Capabilities {
	return benchprovider.Capabilities{LLMJudge: true, FunctionalTests: true}
}
func (p *testProvider) ListTasks(_ context.Context) ([]benchprovider.TaskSpec, error) {
	return []benchprovider.TaskSpec{
		{ID: "task-1", Name: "FizzBuzz", Input: "Write fizzbuzz"},
	}, nil
}
func (p *testProvider) TaskCount(_ context.Context) (int, error) { return 1, nil }

func TestBenchProvider_RegisterAndNew(t *testing.T) {
	benchprovider.Register("test-bench", func(_ map[string]string) (benchprovider.Provider, error) {
		return &testProvider{providerName: "test-bench"}, nil
	})

	p, err := benchprovider.New("test-bench", nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "test-bench" {
		t.Fatalf("expected test-bench, got %s", p.Name())
	}
	if p.Type() != benchmark.TypeSimple {
		t.Fatalf("expected simple type, got %s", p.Type())
	}
}

func TestBenchProvider_UnknownProvider(t *testing.T) {
	_, err := benchprovider.New("nonexistent-bench", nil)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestBenchProvider_Available(t *testing.T) {
	names := benchprovider.Available()
	found := false
	for _, n := range names {
		if n == "test-bench" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected test-bench in available providers")
	}
}

func TestBenchProvider_DuplicateRegistrationPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	benchprovider.Register("test-bench", func(_ map[string]string) (benchprovider.Provider, error) {
		return &testProvider{providerName: "test-bench"}, nil
	})
}

func TestBenchProvider_Capabilities(t *testing.T) {
	p, err := benchprovider.New("test-bench", nil)
	if err != nil {
		t.Fatal(err)
	}
	caps := p.Capabilities()
	if !caps.LLMJudge {
		t.Error("expected LLMJudge capability")
	}
	if !caps.FunctionalTests {
		t.Error("expected FunctionalTests capability")
	}
	if caps.SWEBenchStyle {
		t.Error("did not expect SWEBenchStyle capability")
	}
}

func TestBenchProvider_ListTasks(t *testing.T) {
	p, err := benchprovider.New("test-bench", nil)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := p.ListTasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].ID != "task-1" {
		t.Errorf("expected task-1, got %s", tasks[0].ID)
	}
}

func TestEvalScore_AverageScore(t *testing.T) {
	tests := []struct {
		name string
		dims []benchprovider.EvalDimension
		want float64
	}{
		{
			name: "empty",
			dims: nil,
			want: 0,
		},
		{
			name: "single",
			dims: []benchprovider.EvalDimension{{Name: "correctness", Score: 0.8}},
			want: 0.8,
		},
		{
			name: "multiple",
			dims: []benchprovider.EvalDimension{
				{Name: "correctness", Score: 0.9},
				{Name: "code_quality", Score: 0.7},
			},
			want: 0.8,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := benchprovider.EvalScore{Dimensions: tt.dims}
			got := s.AverageScore()
			if got != tt.want {
				t.Errorf("AverageScore() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaskSpec_Fields(t *testing.T) {
	task := benchprovider.TaskSpec{
		ID:           "swe-001",
		Name:         "Fix login bug",
		Input:        "The login form crashes when...",
		InitialFiles: map[string]string{"main.py": "print('hello')"},
		TestCommand:  "pytest tests/",
		RepoURL:      "https://github.com/example/repo",
		RepoCommit:   "abc123",
		Difficulty:   "hard",
	}

	if task.InitialFiles["main.py"] != "print('hello')" {
		t.Error("unexpected initial files")
	}
	if task.TestCommand != "pytest tests/" {
		t.Error("unexpected test command")
	}
}

func TestExecutionResult_Fields(t *testing.T) {
	result := benchprovider.ExecutionResult{
		ActualOutput: "def fizzbuzz(n): ...",
		FilesChanged: []string{"main.py"},
		TestOutput:   "1 passed",
		ExitCode:     0,
		CostUSD:      0.05,
		TokensIn:     100,
		TokensOut:    50,
		DurationMs:   1500,
		StepCount:    3,
	}

	if result.ExitCode != 0 {
		t.Error("expected exit code 0")
	}
	if result.StepCount != 3 {
		t.Error("expected 3 steps")
	}
}
