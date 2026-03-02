package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

const benchmarkRunColumns = `id, dataset, model, metrics, status, summary_scores,
		total_cost, total_tokens, total_duration_ms, created_at, completed_at,
		suite_id, benchmark_type, exec_mode, config`

const benchmarkResultColumns = `id, run_id, task_id, task_name, scores, actual_output, expected_output,
		tool_calls, cost_usd, tokens_in, tokens_out, duration_ms,
		evaluator_scores, files_changed, functional_test_output`

// CreateBenchmarkRun inserts a new benchmark run.
func (s *Store) CreateBenchmarkRun(ctx context.Context, r *benchmark.Run) error {
	metricsArr := pgTextArray(r.Metrics)
	scores := r.SummaryScores
	if scores == nil {
		scores = json.RawMessage(`{}`)
	}
	cfg := r.Config
	if cfg == nil {
		cfg = json.RawMessage(`{}`)
	}
	const q = `INSERT INTO benchmark_runs
		(id, dataset, model, metrics, status, summary_scores, total_cost, total_tokens,
		 total_duration_ms, created_at, completed_at, suite_id, benchmark_type, exec_mode, config)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`
	_, err := s.pool.Exec(ctx, q,
		r.ID, r.Dataset, r.Model, metricsArr, string(r.Status),
		scores, r.TotalCost, r.TotalTokens, r.TotalDurationMs,
		r.CreatedAt, r.CompletedAt,
		nilIfEmpty(r.SuiteID), nilIfEmpty(string(r.BenchmarkType)),
		nilIfEmpty(string(r.ExecMode)), cfg,
	)
	if err != nil {
		return fmt.Errorf("create benchmark run: %w", err)
	}
	return nil
}

// GetBenchmarkRun retrieves a benchmark run by ID.
func (s *Store) GetBenchmarkRun(ctx context.Context, id string) (*benchmark.Run, error) {
	q := `SELECT ` + benchmarkRunColumns + ` FROM benchmark_runs WHERE id = $1`
	r, err := scanBenchmarkRun(s.pool.QueryRow(ctx, q, id))
	if err != nil {
		return nil, notFoundWrap(err, "get benchmark run %s", id)
	}
	return &r, nil
}

// ListBenchmarkRuns returns all benchmark runs ordered by creation time.
func (s *Store) ListBenchmarkRuns(ctx context.Context) ([]benchmark.Run, error) {
	q := `SELECT ` + benchmarkRunColumns + ` FROM benchmark_runs ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list benchmark runs: %w", err)
	}
	defer rows.Close()

	var result []benchmark.Run
	for rows.Next() {
		r, err := scanBenchmarkRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ListBenchmarkRunsFiltered returns runs matching the given filter.
func (s *Store) ListBenchmarkRunsFiltered(ctx context.Context, filter benchmark.RunFilter) ([]benchmark.Run, error) {
	var conditions []string
	var args []interface{}
	idx := 1

	if filter.SuiteID != "" {
		conditions = append(conditions, fmt.Sprintf("suite_id = $%d", idx))
		args = append(args, filter.SuiteID)
		idx++
	}
	if filter.BenchmarkType != "" {
		conditions = append(conditions, fmt.Sprintf("benchmark_type = $%d", idx))
		args = append(args, string(filter.BenchmarkType))
		idx++
	}
	if filter.Model != "" {
		conditions = append(conditions, fmt.Sprintf("model = $%d", idx))
		args = append(args, filter.Model)
		idx++
	}

	q := `SELECT ` + benchmarkRunColumns + ` FROM benchmark_runs`
	if len(conditions) > 0 {
		q += " WHERE " + strings.Join(conditions, " AND ")
	}
	q += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list benchmark runs filtered: %w", err)
	}
	defer rows.Close()

	var result []benchmark.Run
	for rows.Next() {
		r, err := scanBenchmarkRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// UpdateBenchmarkRun updates a benchmark run (status, scores, totals, completed_at).
func (s *Store) UpdateBenchmarkRun(ctx context.Context, r *benchmark.Run) error {
	scores := r.SummaryScores
	if scores == nil {
		scores = json.RawMessage(`{}`)
	}
	const q = `UPDATE benchmark_runs
		SET status=$2, summary_scores=$3, total_cost=$4, total_tokens=$5,
		    total_duration_ms=$6, completed_at=$7
		WHERE id=$1`
	tag, err := s.pool.Exec(ctx, q,
		r.ID, string(r.Status), scores, r.TotalCost,
		r.TotalTokens, r.TotalDurationMs, r.CompletedAt,
	)
	return execExpectOne(tag, err, "update benchmark run %s", r.ID)
}

// DeleteBenchmarkRun deletes a benchmark run and its results (ON DELETE CASCADE).
func (s *Store) DeleteBenchmarkRun(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM benchmark_runs WHERE id=$1`, id)
	return execExpectOne(tag, err, "delete benchmark run %s", id)
}

// CreateBenchmarkResult inserts a single benchmark result.
func (s *Store) CreateBenchmarkResult(ctx context.Context, res *benchmark.Result) error {
	scores := res.Scores
	if scores == nil {
		scores = json.RawMessage(`{}`)
	}
	toolCalls := res.ToolCalls
	if toolCalls == nil {
		toolCalls = json.RawMessage(`[]`)
	}
	evalScores := res.EvaluatorScores
	if evalScores == nil {
		evalScores = json.RawMessage(`{}`)
	}
	filesChanged := pgTextArray(res.FilesChanged)
	const q = `INSERT INTO benchmark_results
		(id, run_id, task_id, task_name, scores, actual_output, expected_output,
		 tool_calls, cost_usd, tokens_in, tokens_out, duration_ms,
		 evaluator_scores, files_changed, functional_test_output)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`
	_, err := s.pool.Exec(ctx, q,
		res.ID, res.RunID, res.TaskID, res.TaskName,
		scores, res.ActualOutput, res.ExpectedOutput,
		toolCalls, res.CostUSD, res.TokensIn, res.TokensOut, res.DurationMs,
		evalScores, filesChanged, res.FunctionalTestOutput,
	)
	if err != nil {
		return fmt.Errorf("create benchmark result: %w", err)
	}
	return nil
}

// ListBenchmarkResults returns all results for a given benchmark run.
func (s *Store) ListBenchmarkResults(ctx context.Context, runID string) ([]benchmark.Result, error) {
	q := `SELECT ` + benchmarkResultColumns + ` FROM benchmark_results WHERE run_id = $1 ORDER BY task_id`
	rows, err := s.pool.Query(ctx, q, runID)
	if err != nil {
		return nil, fmt.Errorf("list benchmark results: %w", err)
	}
	defer rows.Close()

	var result []benchmark.Result
	for rows.Next() {
		res, err := scanBenchmarkResult(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, res)
	}
	return result, rows.Err()
}

// scanBenchmarkRun scans a single benchmark run row.
func scanBenchmarkRun(row scannable) (benchmark.Run, error) {
	var r benchmark.Run
	var metrics []string
	var suiteID, bmType, execMode *string
	err := row.Scan(
		&r.ID, &r.Dataset, &r.Model, &metrics, &r.Status,
		&r.SummaryScores, &r.TotalCost, &r.TotalTokens, &r.TotalDurationMs,
		&r.CreatedAt, &r.CompletedAt,
		&suiteID, &bmType, &execMode, &r.Config,
	)
	if err != nil {
		return r, err
	}
	r.Metrics = metrics
	if suiteID != nil {
		r.SuiteID = *suiteID
	}
	if bmType != nil {
		r.BenchmarkType = benchmark.BenchmarkType(*bmType)
	}
	if execMode != nil {
		r.ExecMode = benchmark.ExecMode(*execMode)
	}
	return r, nil
}

// scanBenchmarkResult scans a single benchmark result row.
func scanBenchmarkResult(row scannable) (benchmark.Result, error) {
	var res benchmark.Result
	var filesChanged []string
	err := row.Scan(
		&res.ID, &res.RunID, &res.TaskID, &res.TaskName,
		&res.Scores, &res.ActualOutput, &res.ExpectedOutput,
		&res.ToolCalls, &res.CostUSD, &res.TokensIn, &res.TokensOut, &res.DurationMs,
		&res.EvaluatorScores, &filesChanged, &res.FunctionalTestOutput,
	)
	if err != nil {
		return res, err
	}
	res.FilesChanged = filesChanged
	return res, nil
}

// nilIfEmpty returns nil for empty strings, or the string pointer.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
