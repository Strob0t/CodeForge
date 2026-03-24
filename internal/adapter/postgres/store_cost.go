package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// --- Cost Aggregation ---

func (s *Store) CostSummaryGlobal(ctx context.Context) ([]cost.ProjectSummary, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT r.project_id, COALESCE(p.name, ''), SUM(r.cost_usd), SUM(r.tokens_in), SUM(r.tokens_out), COUNT(*)
		 FROM runs r LEFT JOIN projects p ON r.project_id = p.id
		 WHERE r.tenant_id = $1
		 GROUP BY r.project_id, p.name
		 ORDER BY SUM(r.cost_usd) DESC`, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("cost summary global: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (cost.ProjectSummary, error) {
		var ps cost.ProjectSummary
		err := r.Scan(&ps.ProjectID, &ps.ProjectName, &ps.TotalCostUSD, &ps.TotalTokensIn, &ps.TotalTokensOut, &ps.RunCount)
		return ps, err
	})
}

func (s *Store) CostSummaryByProject(ctx context.Context, projectID string) (*cost.Summary, error) {
	var cs cost.Summary
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(tokens_in), 0), COALESCE(SUM(tokens_out), 0), COUNT(*)
		 FROM runs WHERE project_id = $1 AND tenant_id = $2`, projectID, tenantFromCtx(ctx)).
		Scan(&cs.TotalCostUSD, &cs.TotalTokensIn, &cs.TotalTokensOut, &cs.RunCount)
	if err != nil {
		return nil, fmt.Errorf("cost summary by project: %w", err)
	}
	return &cs, nil
}

func (s *Store) CostByModel(ctx context.Context, projectID string) ([]cost.ModelSummary, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT COALESCE(model, ''), SUM(cost_usd), SUM(tokens_in), SUM(tokens_out), COUNT(*)
		 FROM runs WHERE project_id = $1 AND tenant_id = $2
		 GROUP BY model ORDER BY SUM(cost_usd) DESC`, projectID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("cost by model: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (cost.ModelSummary, error) {
		var ms cost.ModelSummary
		err := r.Scan(&ms.Model, &ms.TotalCostUSD, &ms.TotalTokensIn, &ms.TotalTokensOut, &ms.RunCount)
		return ms, err
	})
}

func (s *Store) CostTimeSeries(ctx context.Context, projectID string, days int) ([]cost.DailyCost, error) {
	if days <= 0 {
		days = 30
	}
	rows, err := s.pool.Query(ctx,
		`SELECT TO_CHAR(created_at::date, 'YYYY-MM-DD'), SUM(cost_usd), SUM(tokens_in), SUM(tokens_out), COUNT(*)
		 FROM runs
		 WHERE project_id = $1 AND tenant_id = $2 AND created_at >= NOW() - ($3 || ' days')::interval
		 GROUP BY created_at::date
		 ORDER BY created_at::date`, projectID, tenantFromCtx(ctx), fmt.Sprintf("%d", days))
	if err != nil {
		return nil, fmt.Errorf("cost time series: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (cost.DailyCost, error) {
		var dc cost.DailyCost
		err := r.Scan(&dc.Date, &dc.CostUSD, &dc.TokensIn, &dc.TokensOut, &dc.RunCount)
		return dc, err
	})
}

func (s *Store) RecentRunsWithCost(ctx context.Context, projectID string, limit int) ([]run.Run, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, task_id, agent_id, project_id, COALESCE(team_id::text, ''), mode_id, policy_profile,
		        exec_mode, deliver_mode, status, step_count, cost_usd, tokens_in, tokens_out, model,
		        artifact_type, artifact_valid, artifact_errors,
		        output, error, version, started_at, completed_at, created_at, updated_at
		 FROM runs WHERE project_id = $1 AND tenant_id = $2
		 ORDER BY created_at DESC LIMIT $3`, projectID, tenantFromCtx(ctx), limit)
	if err != nil {
		return nil, fmt.Errorf("recent runs with cost: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (run.Run, error) {
		return scanRun(r)
	})
}

// --- Per-Tool Cost Aggregation (Phase 12H) ---

func (s *Store) CostByTool(ctx context.Context, projectID string) ([]cost.ToolSummary, error) {
	tid := tenantFromCtx(ctx)
	rows, err := s.pool.Query(ctx,
		`SELECT tool_name, COALESCE(model, ''), SUM(cost_usd), SUM(tokens_in), SUM(tokens_out), COUNT(*)
		 FROM agent_events
		 WHERE project_id = $1 AND tenant_id = $2 AND event_type = 'run.toolcall.result' AND tool_name != ''
		 GROUP BY tool_name, model
		 ORDER BY SUM(cost_usd) DESC`, projectID, tid)
	if err != nil {
		return nil, fmt.Errorf("cost by tool: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (cost.ToolSummary, error) {
		return scanToolSummary(r)
	})
}

func (s *Store) CostByToolForRun(ctx context.Context, runID string) ([]cost.ToolSummary, error) {
	tid := tenantFromCtx(ctx)
	rows, err := s.pool.Query(ctx,
		`SELECT tool_name, COALESCE(model, ''), SUM(cost_usd), SUM(tokens_in), SUM(tokens_out), COUNT(*)
		 FROM agent_events
		 WHERE run_id = $1 AND tenant_id = $2 AND event_type = 'run.toolcall.result' AND tool_name != ''
		 GROUP BY tool_name, model
		 ORDER BY SUM(cost_usd) DESC`, runID, tid)
	if err != nil {
		return nil, fmt.Errorf("cost by tool for run: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (cost.ToolSummary, error) {
		return scanToolSummary(r)
	})
}

func scanToolSummary(row scannable) (cost.ToolSummary, error) {
	var ts cost.ToolSummary
	err := row.Scan(&ts.Tool, &ts.Model, &ts.CostUSD, &ts.TokensIn, &ts.TokensOut, &ts.CallCount)
	return ts, err
}
