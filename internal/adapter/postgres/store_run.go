package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
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

// runColumns is the column list scanRun reads.
const runColumns = `id, tenant_id, task_id, agent_id, project_id, COALESCE(team_id::text, ''), mode_id, policy_profile, exec_mode, deliver_mode, status,
		        step_count, cost_usd, tokens_in, tokens_out, model, artifact_type, artifact_valid, artifact_errors,
		        output, error, version, started_at, completed_at, created_at, updated_at`

func (s *Store) GetRun(ctx context.Context, id string) (*run.Run, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+runColumns+` FROM runs WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	r, err := scanRun(row)
	if err != nil {
		return nil, notFoundWrap(err, "get run %s", id)
	}
	return &r, nil
}

const runExistsSQL = `SELECT EXISTS (SELECT 1 FROM runs WHERE id = $1 AND tenant_id = $2)`

// sourceStatuses parameterizes the transition predicate of a status write:
// the statuses a run may be moved to status from (run.SourceStatuses).
func sourceStatuses(status run.Status) []string {
	return statusStrings(run.SourceStatuses(status))
}

// UpdateRunStatus sets a run's status and counters. It returns
// domain.ErrConflict when the run's current status does not lead to status:
// running is written only while the run is pending or running.
func (s *Store) UpdateRunStatus(ctx context.Context, id string, status run.Status, stepCount int, costUSD float64, tokensIn, tokensOut int64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET status = $2, step_count = $3, cost_usd = $4, tokens_in = $5, tokens_out = $6, updated_at = now()
		 WHERE id = $1 AND tenant_id = $7 AND status = ANY($8)`,
		id, string(status), stepCount, costUSD, tokensIn, tokensOut, tenantFromCtx(ctx), sourceStatuses(status))
	return s.guardedUpdateResult(ctx, tag, err, runExistsSQL, "update run status", id)
}

// completionUsageSQL sets a run's counters to the reported ones ($5 cost, $6
// steps, $7/$8 tokens) without lowering them: usage counters never go down,
// so a stale or partial report cannot undo usage recorded in between.
const completionUsageSQL = `cost_usd = GREATEST(cost_usd, $5), step_count = GREATEST(step_count, $6),
		 tokens_in = GREATEST(tokens_in, $7), tokens_out = GREATEST(tokens_out, $8)`

// EnterQualityGate moves a running run to quality_gate with the outcome the
// worker reported (output, model, usage), which the run keeps when the gate
// result ends it. Any other source status is refused with domain.ErrConflict.
func (s *Store) EnterQualityGate(ctx context.Context, req *run.CompletionRequest) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET status = $2, output = $3, error = $4, `+completionUsageSQL+`, model = $9, updated_at = now()
		 WHERE id = $1 AND tenant_id = $10 AND status = ANY($11)`,
		req.ID, string(run.StatusQualityGate), req.Output, req.Error, req.CostUSD, req.StepCount, req.TokensIn, req.TokensOut, req.Model,
		tenantFromCtx(ctx), sourceStatuses(run.StatusQualityGate))
	return s.guardedUpdateResult(ctx, tag, err, runExistsSQL, "enter quality gate", req.ID)
}

// CompleteRun ends an active run. It returns domain.ErrConflict when the run
// already ended, so a run is completed exactly once, and domain.ErrValidation
// for a status that does not end a run. The usage counters never go down.
func (s *Store) CompleteRun(ctx context.Context, req *run.CompletionRequest) error {
	if !req.Status.IsTerminal() {
		return fmt.Errorf("complete run %s with status %q: %w", req.ID, req.Status, domain.ErrValidation)
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET status = $2, output = $3, error = $4, `+completionUsageSQL+`, model = $9,
		 completed_at = now(), updated_at = now()
		 WHERE id = $1 AND tenant_id = $10 AND status = ANY($11)`,
		req.ID, string(req.Status), req.Output, req.Error, req.CostUSD, req.StepCount, req.TokensIn, req.TokensOut, req.Model,
		tenantFromCtx(ctx), sourceStatuses(req.Status))
	return s.guardedUpdateResult(ctx, tag, err, runExistsSQL, "complete run", req.ID)
}

// CountRunStep counts one tool call of a running run without touching its
// usage counters. A run that is not running is refused with
// domain.ErrConflict.
func (s *Store) CountRunStep(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET step_count = step_count + 1, updated_at = now()
		 WHERE id = $1 AND tenant_id = $2 AND status = $3`,
		id, tenantFromCtx(ctx), string(run.StatusRunning))
	return s.guardedUpdateResult(ctx, tag, err, runExistsSQL, "count run step", id)
}

// AddRunUsage adds a tool call's usage to a running run's counters atomically
// and returns the run as stored afterwards; the status is never touched. A
// run that is no longer running is refused with domain.ErrConflict: the
// worker's totals (stored when the run entered its gate or ended, or raised
// with RaiseRunUsage) include the call, and adding it would count it twice.
func (s *Store) AddRunUsage(ctx context.Context, id string, usage *run.Usage) (*run.Run, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE runs SET step_count = step_count + $2, cost_usd = cost_usd + $3,
		 tokens_in = tokens_in + $4, tokens_out = tokens_out + $5, updated_at = now()
		 WHERE id = $1 AND tenant_id = $6 AND status = $7
		 RETURNING `+runColumns,
		id, usage.Steps, usage.CostUSD, usage.TokensIn, usage.TokensOut, tenantFromCtx(ctx), string(run.StatusRunning))
	r, err := scanRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, s.refusedUpdate(ctx, runExistsSQL, "add run usage", id)
	}
	if err != nil {
		return nil, fmt.Errorf("add run usage %s: %w", id, err)
	}
	return &r, nil
}

// RaiseRunUsage raises a run's counters to the totals the worker reported,
// whatever the run's status; a counter never goes down. The status is never
// touched.
func (s *Store) RaiseRunUsage(ctx context.Context, id string, totals *run.Usage) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET step_count = GREATEST(step_count, $2), cost_usd = GREATEST(cost_usd, $3),
		 tokens_in = GREATEST(tokens_in, $4), tokens_out = GREATEST(tokens_out, $5), updated_at = now()
		 WHERE id = $1 AND tenant_id = $6`,
		id, totals.Steps, totals.CostUSD, totals.TokensIn, totals.TokensOut, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "raise run usage %s", id)
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
		`SELECT `+runColumns+` FROM runs WHERE task_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`, taskID, tenantFromCtx(ctx))
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
