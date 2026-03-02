package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

// CreateBenchmarkSuite inserts a new benchmark suite.
func (s *Store) CreateBenchmarkSuite(ctx context.Context, suite *benchmark.Suite) error {
	cfg := suite.Config
	if cfg == nil {
		cfg = json.RawMessage(`{}`)
	}
	const q = `INSERT INTO benchmark_suites
		(id, name, description, type, provider_name, task_count, config, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := s.pool.Exec(ctx, q,
		suite.ID, suite.Name, suite.Description, string(suite.Type),
		suite.ProviderName, suite.TaskCount, cfg, suite.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create benchmark suite: %w", err)
	}
	return nil
}

// GetBenchmarkSuite retrieves a benchmark suite by ID.
func (s *Store) GetBenchmarkSuite(ctx context.Context, id string) (*benchmark.Suite, error) {
	const q = `SELECT id, name, description, type, provider_name, task_count, config, created_at
		FROM benchmark_suites WHERE id = $1`
	var suite benchmark.Suite
	err := s.pool.QueryRow(ctx, q, id).Scan(
		&suite.ID, &suite.Name, &suite.Description, &suite.Type,
		&suite.ProviderName, &suite.TaskCount, &suite.Config, &suite.CreatedAt,
	)
	if err != nil {
		return nil, notFoundWrap(err, "get benchmark suite %s", id)
	}
	return &suite, nil
}

// ListBenchmarkSuites returns all registered benchmark suites.
func (s *Store) ListBenchmarkSuites(ctx context.Context) ([]benchmark.Suite, error) {
	const q = `SELECT id, name, description, type, provider_name, task_count, config, created_at
		FROM benchmark_suites ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list benchmark suites: %w", err)
	}
	defer rows.Close()

	var result []benchmark.Suite
	for rows.Next() {
		var suite benchmark.Suite
		if err := rows.Scan(
			&suite.ID, &suite.Name, &suite.Description, &suite.Type,
			&suite.ProviderName, &suite.TaskCount, &suite.Config, &suite.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan benchmark suite: %w", err)
		}
		result = append(result, suite)
	}
	return result, rows.Err()
}

// DeleteBenchmarkSuite deletes a benchmark suite by ID.
func (s *Store) DeleteBenchmarkSuite(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM benchmark_suites WHERE id=$1`, id)
	return execExpectOne(tag, err, "delete benchmark suite %s", id)
}
