package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

// CreateBenchmarkSuite inserts a new benchmark suite.
func (s *Store) CreateBenchmarkSuite(ctx context.Context, suite *benchmark.Suite) error {
	cfg := suite.Config
	if cfg == nil {
		cfg = json.RawMessage(`{}`)
	}
	const q = `INSERT INTO benchmark_suites
		(id, tenant_id, name, description, type, provider_name, task_count, config, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := s.pool.Exec(ctx, q,
		suite.ID, tenantFromCtx(ctx), suite.Name, suite.Description, string(suite.Type),
		suite.ProviderName, suite.TaskCount, cfg, suite.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create benchmark suite: %w", err)
	}
	return nil
}

// GetBenchmarkSuite retrieves a benchmark suite by ID.
func (s *Store) GetBenchmarkSuite(ctx context.Context, id string) (*benchmark.Suite, error) {
	const q = `SELECT id, tenant_id, name, description, type, provider_name, task_count, config, created_at
		FROM benchmark_suites WHERE id = $1 AND tenant_id = $2`
	var suite benchmark.Suite
	err := s.pool.QueryRow(ctx, q, id, tenantFromCtx(ctx)).Scan(
		&suite.ID, &suite.TenantID, &suite.Name, &suite.Description, &suite.Type,
		&suite.ProviderName, &suite.TaskCount, &suite.Config, &suite.CreatedAt,
	)
	if err != nil {
		return nil, notFoundWrap(err, "get benchmark suite %s", id)
	}
	return &suite, nil
}

// ListBenchmarkSuites returns all registered benchmark suites.
func (s *Store) ListBenchmarkSuites(ctx context.Context) ([]benchmark.Suite, error) {
	const q = `SELECT id, tenant_id, name, description, type, provider_name, task_count, config, created_at
		FROM benchmark_suites WHERE tenant_id = $1 ORDER BY created_at DESC
		LIMIT $2`
	rows, err := s.pool.Query(ctx, q, tenantFromCtx(ctx), DefaultListLimit)
	if err != nil {
		return nil, fmt.Errorf("list benchmark suites: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (benchmark.Suite, error) {
		var suite benchmark.Suite
		err := r.Scan(
			&suite.ID, &suite.TenantID, &suite.Name, &suite.Description, &suite.Type,
			&suite.ProviderName, &suite.TaskCount, &suite.Config, &suite.CreatedAt,
		)
		return suite, err
	})
}

// UpdateBenchmarkSuite updates an existing benchmark suite.
func (s *Store) UpdateBenchmarkSuite(ctx context.Context, suite *benchmark.Suite) error {
	cfg := suite.Config
	if cfg == nil {
		cfg = json.RawMessage(`{}`)
	}
	const q = `UPDATE benchmark_suites
		SET name=$2, description=$3, type=$4, provider_name=$5, config=$6
		WHERE id=$1 AND tenant_id=$7`
	tag, err := s.pool.Exec(ctx, q,
		suite.ID, suite.Name, suite.Description, string(suite.Type),
		suite.ProviderName, cfg, tenantFromCtx(ctx),
	)
	return execExpectOne(tag, err, "update benchmark suite %s", suite.ID)
}

// DeleteBenchmarkSuite deletes a benchmark suite by ID.
func (s *Store) DeleteBenchmarkSuite(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM benchmark_suites WHERE id=$1 AND tenant_id=$2`, id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete benchmark suite %s", id)
}
