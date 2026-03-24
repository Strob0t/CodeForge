package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// --- Runs ---

func (s *Store) CreateRun(ctx context.Context, r *run.Run) error {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO runs (tenant_id, task_id, agent_id, project_id, team_id, mode_id, policy_profile, exec_mode, deliver_mode, status, output)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING id, started_at, created_at, updated_at, version`,
		tenantFromCtx(ctx), r.TaskID, r.AgentID, r.ProjectID, nullIfEmpty(r.TeamID), r.ModeID, r.PolicyProfile, string(r.ExecMode), string(r.DeliverMode), string(r.Status), r.Output)

	return row.Scan(&r.ID, &r.StartedAt, &r.CreatedAt, &r.UpdatedAt, &r.Version)
}

func (s *Store) GetRun(ctx context.Context, id string) (*run.Run, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, task_id, agent_id, project_id, COALESCE(team_id::text, ''), mode_id, policy_profile, exec_mode, deliver_mode, status,
		        step_count, cost_usd, tokens_in, tokens_out, model, artifact_type, artifact_valid, artifact_errors,
		        output, error, version, started_at, completed_at, created_at, updated_at
		 FROM runs WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	r, err := scanRun(row)
	if err != nil {
		return nil, notFoundWrap(err, "get run %s", id)
	}
	return &r, nil
}

func (s *Store) UpdateRunStatus(ctx context.Context, id string, status run.Status, stepCount int, costUSD float64, tokensIn, tokensOut int64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET status = $2, step_count = $3, cost_usd = $4, tokens_in = $5, tokens_out = $6, updated_at = now()
		 WHERE id = $1 AND tenant_id = $7`,
		id, string(status), stepCount, costUSD, tokensIn, tokensOut, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "update run status %s", id)
}

func (s *Store) CompleteRun(ctx context.Context, id string, status run.Status, output, errMsg string, costUSD float64, stepCount int, tokensIn, tokensOut int64, model string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET status = $2, output = $3, error = $4, cost_usd = $5, step_count = $6,
		 tokens_in = $7, tokens_out = $8, model = $9, completed_at = now(), updated_at = now()
		 WHERE id = $1 AND tenant_id = $10`,
		id, string(status), output, errMsg, costUSD, stepCount, tokensIn, tokensOut, model, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "complete run %s", id)
}

func (s *Store) UpdateRunArtifact(ctx context.Context, id, artifactType string, valid *bool, errs []string) error {
	errJSON, err := marshalJSON(errs, "artifact errors")
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET artifact_type = $2, artifact_valid = $3, artifact_errors = $4, updated_at = now()
		 WHERE id = $1 AND tenant_id = $5`,
		id, artifactType, valid, errJSON, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "update run artifact %s", id)
}

func (s *Store) ListRunsByTask(ctx context.Context, taskID string) ([]run.Run, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, task_id, agent_id, project_id, COALESCE(team_id::text, ''), mode_id, policy_profile, exec_mode, deliver_mode, status,
		        step_count, cost_usd, tokens_in, tokens_out, model, artifact_type, artifact_valid, artifact_errors,
		        output, error, version, started_at, completed_at, created_at, updated_at
		 FROM runs WHERE task_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`, taskID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list runs by task: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (run.Run, error) {
		return scanRun(r)
	})
}

func scanRun(row scannable) (run.Run, error) {
	var r run.Run
	var artifactErrorsJSON []byte
	err := row.Scan(
		&r.ID, &r.TenantID, &r.TaskID, &r.AgentID, &r.ProjectID, &r.TeamID, &r.ModeID, &r.PolicyProfile,
		&r.ExecMode, &r.DeliverMode, &r.Status, &r.StepCount, &r.CostUSD,
		&r.TokensIn, &r.TokensOut, &r.Model,
		&r.ArtifactType, &r.ArtifactValid, &artifactErrorsJSON,
		&r.Output, &r.Error,
		&r.Version, &r.StartedAt, &r.CompletedAt, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return r, err
	}
	if err := unmarshalJSONField(artifactErrorsJSON, &r.ArtifactErrors, "artifact_errors"); err != nil {
		return r, err
	}
	return r, nil
}
