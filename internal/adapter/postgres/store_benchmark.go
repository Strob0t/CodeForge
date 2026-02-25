package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

// CreateBenchmarkRun inserts a new benchmark run.
func (s *Store) CreateBenchmarkRun(ctx context.Context, r *benchmark.Run) error {
	metricsArr := pgTextArray(r.Metrics)
	scores := r.SummaryScores
	if scores == nil {
		scores = json.RawMessage(`{}`)
	}
	const q = `INSERT INTO benchmark_runs
		(id, dataset, model, metrics, status, summary_scores, total_cost, total_tokens, total_duration_ms, created_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	_, err := s.pool.Exec(ctx, q,
		r.ID, r.Dataset, r.Model, metricsArr, string(r.Status),
		scores, r.TotalCost, r.TotalTokens, r.TotalDurationMs,
		r.CreatedAt, r.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("create benchmark run: %w", err)
	}
	return nil
}

// GetBenchmarkRun retrieves a benchmark run by ID.
func (s *Store) GetBenchmarkRun(ctx context.Context, id string) (*benchmark.Run, error) {
	const q = `SELECT id, dataset, model, metrics, status, summary_scores,
		total_cost, total_tokens, total_duration_ms, created_at, completed_at
		FROM benchmark_runs WHERE id = $1`
	r, err := scanBenchmarkRun(s.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get benchmark run %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get benchmark run %s: %w", id, err)
	}
	return &r, nil
}

// ListBenchmarkRuns returns all benchmark runs ordered by creation time.
func (s *Store) ListBenchmarkRuns(ctx context.Context) ([]benchmark.Run, error) {
	const q = `SELECT id, dataset, model, metrics, status, summary_scores,
		total_cost, total_tokens, total_duration_ms, created_at, completed_at
		FROM benchmark_runs ORDER BY created_at DESC`
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
	if err != nil {
		return fmt.Errorf("update benchmark run %s: %w", r.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update benchmark run %s: %w", r.ID, domain.ErrNotFound)
	}
	return nil
}

// DeleteBenchmarkRun deletes a benchmark run and its results (ON DELETE CASCADE).
func (s *Store) DeleteBenchmarkRun(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM benchmark_runs WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete benchmark run %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete benchmark run %s: %w", id, domain.ErrNotFound)
	}
	return nil
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
	const q = `INSERT INTO benchmark_results
		(id, run_id, task_id, task_name, scores, actual_output, expected_output,
		 tool_calls, cost_usd, tokens_in, tokens_out, duration_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	_, err := s.pool.Exec(ctx, q,
		res.ID, res.RunID, res.TaskID, res.TaskName,
		scores, res.ActualOutput, res.ExpectedOutput,
		toolCalls, res.CostUSD, res.TokensIn, res.TokensOut, res.DurationMs,
	)
	if err != nil {
		return fmt.Errorf("create benchmark result: %w", err)
	}
	return nil
}

// ListBenchmarkResults returns all results for a given benchmark run.
func (s *Store) ListBenchmarkResults(ctx context.Context, runID string) ([]benchmark.Result, error) {
	const q = `SELECT id, run_id, task_id, task_name, scores, actual_output, expected_output,
		tool_calls, cost_usd, tokens_in, tokens_out, duration_ms
		FROM benchmark_results WHERE run_id = $1 ORDER BY task_id`
	rows, err := s.pool.Query(ctx, q, runID)
	if err != nil {
		return nil, fmt.Errorf("list benchmark results: %w", err)
	}
	defer rows.Close()

	var result []benchmark.Result
	for rows.Next() {
		var res benchmark.Result
		if err := rows.Scan(
			&res.ID, &res.RunID, &res.TaskID, &res.TaskName,
			&res.Scores, &res.ActualOutput, &res.ExpectedOutput,
			&res.ToolCalls, &res.CostUSD, &res.TokensIn, &res.TokensOut, &res.DurationMs,
		); err != nil {
			return nil, fmt.Errorf("scan benchmark result: %w", err)
		}
		result = append(result, res)
	}
	return result, rows.Err()
}

// scanBenchmarkRun scans a single benchmark run row.
func scanBenchmarkRun(row scannable) (benchmark.Run, error) {
	var r benchmark.Run
	var metrics []string
	err := row.Scan(
		&r.ID, &r.Dataset, &r.Model, &metrics, &r.Status,
		&r.SummaryScores, &r.TotalCost, &r.TotalTokens, &r.TotalDurationMs,
		&r.CreatedAt, &r.CompletedAt,
	)
	if err != nil {
		return r, err
	}
	r.Metrics = metrics
	return r, nil
}

// pgTextArray converts a string slice to a pgx-compatible text array.
func pgTextArray(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
