package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

// BenchmarkStore defines database operations for benchmark suites, runs, and results.
type BenchmarkStore interface {
	// Benchmark Suites (Phase 26)
	CreateBenchmarkSuite(ctx context.Context, suite *benchmark.Suite) error
	GetBenchmarkSuite(ctx context.Context, id string) (*benchmark.Suite, error)
	ListBenchmarkSuites(ctx context.Context) ([]benchmark.Suite, error)
	UpdateBenchmarkSuite(ctx context.Context, suite *benchmark.Suite) error
	DeleteBenchmarkSuite(ctx context.Context, id string) error

	// Benchmark Runs
	CreateBenchmarkRun(ctx context.Context, r *benchmark.Run) error
	GetBenchmarkRun(ctx context.Context, id string) (*benchmark.Run, error)
	ListBenchmarkRuns(ctx context.Context) ([]benchmark.Run, error)
	UpdateBenchmarkRun(ctx context.Context, r *benchmark.Run) error
	DeleteBenchmarkRun(ctx context.Context, id string) error
	ListBenchmarkRunsFiltered(ctx context.Context, filter *benchmark.RunFilter) ([]benchmark.Run, error)

	// Benchmark Results
	CreateBenchmarkResult(ctx context.Context, res *benchmark.Result) error
	ListBenchmarkResults(ctx context.Context, runID string) ([]benchmark.Result, error)
}
