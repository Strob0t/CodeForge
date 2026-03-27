package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/dashboard"
)

// DashboardStats returns the 7 KPI values with trend deltas.
func (s *Store) DashboardStats(ctx context.Context) (*dashboard.DashboardStats, error) {
	tid := tenantFromCtx(ctx)
	var ds dashboard.DashboardStats

	// Single-pass conditional aggregation across all time windows
	var costYesterday float64
	var tokensYesterday int64
	var completedCur, failedCur, timeoutCur, totalCur int
	var completedPrev, failedPrev, timeoutPrev, totalPrev int
	var avgCostCur, avgCostPrev float64
	var failedRecent, totalRecent int

	err := s.pool.QueryRow(ctx, `
		SELECT
			-- cost today/yesterday
			COALESCE(SUM(CASE WHEN created_at::date = CURRENT_DATE THEN cost_usd ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at::date = CURRENT_DATE - 1 THEN cost_usd ELSE 0 END), 0),
			-- tokens today/yesterday
			COALESCE(SUM(CASE WHEN created_at::date = CURRENT_DATE THEN tokens_in + tokens_out ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at::date = CURRENT_DATE - 1 THEN tokens_in + tokens_out ELSE 0 END), 0),
			-- success rate current 7d
			COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE - 7 AND status = 'completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE - 7 AND status = 'failed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE - 7 AND status = 'timeout' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE - 7 THEN 1 ELSE 0 END), 0),
			-- success rate previous 7d
			COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE - 14 AND created_at < CURRENT_DATE - 7 AND status = 'completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE - 14 AND created_at < CURRENT_DATE - 7 AND status = 'failed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE - 14 AND created_at < CURRENT_DATE - 7 AND status = 'timeout' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE - 14 AND created_at < CURRENT_DATE - 7 THEN 1 ELSE 0 END), 0),
			-- avg cost current/previous 7d
			COALESCE(AVG(CASE WHEN created_at >= CURRENT_DATE - 7 THEN cost_usd END), 0),
			COALESCE(AVG(CASE WHEN created_at >= CURRENT_DATE - 14 AND created_at < CURRENT_DATE - 7 THEN cost_usd END), 0),
			-- error rate 24h / previous 24h
			COALESCE(SUM(CASE WHEN created_at >= NOW() - INTERVAL '24 hours' AND status = 'failed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at >= NOW() - INTERVAL '24 hours' THEN 1 ELSE 0 END), 0)
		FROM runs WHERE tenant_id = $1 AND created_at >= CURRENT_DATE - 14
	`, tid).Scan(
		&ds.CostTodayUSD, &costYesterday,
		&ds.TokenUsageToday, &tokensYesterday,
		&completedCur, &failedCur, &timeoutCur, &totalCur,
		&completedPrev, &failedPrev, &timeoutPrev, &totalPrev,
		&avgCostCur, &avgCostPrev,
		&failedRecent, &totalRecent,
	)
	if err != nil {
		return nil, fmt.Errorf("dashboard stats: %w", err)
	}

	// Compute derived values
	ds.CostTodayDelta = deltaPct(ds.CostTodayUSD, costYesterday)
	ds.TokenUsageDelta = deltaPct(float64(ds.TokenUsageToday), float64(tokensYesterday))
	ds.AvgCostPerRun = avgCostCur
	ds.AvgCostDelta = deltaPct(avgCostCur, avgCostPrev)

	denomCur := completedCur + failedCur + timeoutCur
	if denomCur > 0 {
		ds.SuccessRate7d = float64(completedCur) / float64(denomCur) * 100
	}
	denomPrev := completedPrev + failedPrev + timeoutPrev
	var prevRate float64
	if denomPrev > 0 {
		prevRate = float64(completedPrev) / float64(denomPrev) * 100
	}
	ds.SuccessRateDelta = ds.SuccessRate7d - prevRate

	if totalRecent > 0 {
		ds.ErrorRate24h = float64(failedRecent) / float64(totalRecent) * 100
	}

	// Active runs + agents (separate queries for real-time counts).
	// Non-fatal: log errors but return zero-valued counts rather than failing the entire dashboard.
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM runs WHERE tenant_id = $1 AND status = 'running'`, tid,
	).Scan(&ds.ActiveRuns); err != nil {
		slog.Warn("dashboard: failed to count active runs", "tenant_id", tid, "error", err)
	}

	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM agents WHERE tenant_id = $1 AND status = 'running'`, tid,
	).Scan(&ds.ActiveAgents); err != nil {
		slog.Warn("dashboard: failed to count active agents", "tenant_id", tid, "error", err)
	}

	return &ds, nil
}

// ProjectHealth computes the health score + stats for a single project.
func (s *Store) ProjectHealth(ctx context.Context, projectID string) (*dashboard.ProjectHealth, error) {
	tid := tenantFromCtx(ctx)
	ph := &dashboard.ProjectHealth{}

	// Run stats for health factors
	var completed7d, failed7d, timeout7d, total7d int
	var failed24h, total24h int
	var tasksCompleted, tasksTotal int
	var activeAgents, runningTasks int
	var costCur, costPrev float64
	var lastActivityAt *time.Time
	var lastActivity string

	err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE - 7 AND status = 'completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE - 7 AND status = 'failed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE - 7 AND status = 'timeout' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE - 7 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at >= NOW() - INTERVAL '24 hours' AND status = 'failed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at >= NOW() - INTERVAL '24 hours' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE - 7 THEN cost_usd ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE - 14 AND created_at < CURRENT_DATE - 7 THEN cost_usd ELSE 0 END), 0)
		FROM runs WHERE project_id = $1 AND tenant_id = $2 AND created_at >= CURRENT_DATE - 14
	`, projectID, tid).Scan(
		&completed7d, &failed7d, &timeout7d, &total7d,
		&failed24h, &total24h,
		&costCur, &costPrev,
	)
	if err != nil {
		return nil, fmt.Errorf("project health runs: %w", err)
	}

	// Tasks — non-fatal, log and continue with zero values.
	if err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END), 0),
			COUNT(*)
		FROM tasks WHERE project_id = $1 AND tenant_id = $2
	`, projectID, tid).Scan(&tasksCompleted, &tasksTotal); err != nil {
		slog.Warn("project health: failed to count tasks", "project_id", projectID, "error", err)
	}

	// Active agents + running tasks — non-fatal.
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM agents WHERE project_id = $1 AND tenant_id = $2 AND status = 'running'`,
		projectID, tid,
	).Scan(&activeAgents); err != nil {
		slog.Warn("project health: failed to count active agents", "project_id", projectID, "error", err)
	}

	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tasks WHERE project_id = $1 AND tenant_id = $2 AND status = 'running'`,
		projectID, tid,
	).Scan(&runningTasks); err != nil {
		slog.Warn("project health: failed to count running tasks", "project_id", projectID, "error", err)
	}

	// Last activity (most recent run) — non-fatal.
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(status, ''), completed_at
		FROM runs WHERE project_id = $1 AND tenant_id = $2
		ORDER BY created_at DESC LIMIT 1
	`, projectID, tid).Scan(&lastActivity, &lastActivityAt); err != nil {
		slog.Warn("project health: failed to get last activity", "project_id", projectID, "error", err)
	}

	// Sparkline: daily costs last 7 days
	sparkRows, err := s.pool.Query(ctx, `
		SELECT COALESCE(SUM(cost_usd), 0)
		FROM runs
		WHERE project_id = $1 AND tenant_id = $2
			AND created_at >= CURRENT_DATE - 7
		GROUP BY created_at::date
		ORDER BY created_at::date
	`, projectID, tid)
	if err != nil {
		return nil, fmt.Errorf("project health sparkline: %w", err)
	}
	defer sparkRows.Close()
	for sparkRows.Next() {
		var v float64
		if err := sparkRows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan sparkline: %w", err)
		}
		ph.Sparkline = append(ph.Sparkline, v)
	}
	if ph.Sparkline == nil {
		ph.Sparkline = []float64{}
	}

	// Compute health factors
	var successRate float64
	denom := completed7d + failed7d + timeout7d
	if denom > 0 {
		successRate = float64(completed7d) / float64(denom) * 100
	}

	var errorRateInv float64 = 100
	if total24h > 0 {
		errorRateInv = (1 - float64(failed24h)/float64(total24h)) * 100
	}

	var actFreshness float64
	if lastActivityAt != nil {
		hours := time.Since(*lastActivityAt).Hours()
		switch {
		case hours < 1:
			actFreshness = 100
		case hours < 6:
			actFreshness = 80
		case hours < 24:
			actFreshness = 50
		case hours < 72:
			actFreshness = 20
		}
	}

	var taskVelocity float64
	if tasksTotal > 0 {
		taskVelocity = float64(tasksCompleted) / float64(tasksTotal) * 100
	}

	var costStab float64 = 100
	if costPrev > 0 {
		deltaAbs := math.Abs(costCur-costPrev) / costPrev * 100
		costStab = math.Max(0, 100-deltaAbs)
	}

	ph.Factors = dashboard.HealthFactors{
		SuccessRate:       successRate,
		ErrorRateInv:      errorRateInv,
		ActivityFreshness: actFreshness,
		TaskVelocity:      taskVelocity,
		CostStability:     costStab,
	}
	ph.Score = ph.Factors.Score()
	ph.Level = dashboard.HealthLevel(ph.Score)

	ph.Stats = dashboard.ProjectHealthStats{
		SuccessRatePct: successRate,
		TotalRuns7d:    total7d,
		TotalCostUSD:   costCur,
		CostDeltaPct:   deltaPct(costCur, costPrev),
		ActiveAgents:   activeAgents,
		RunningTasks:   runningTasks,
		TasksCompleted: tasksCompleted,
		TasksTotal:     tasksTotal,
		LastActivity:   lastActivity,
	}
	if lastActivityAt != nil {
		ph.Stats.LastActivityAt = lastActivityAt.Format(time.RFC3339)
	}

	return ph, nil
}

// DashboardRunOutcomes returns run status counts for the donut chart.
func (s *Store) DashboardRunOutcomes(ctx context.Context, days int) ([]dashboard.RunOutcome, error) {
	if days <= 0 {
		days = 7
	}
	tid := tenantFromCtx(ctx)
	rows, err := s.pool.Query(ctx, `
		SELECT status, COUNT(*)
		FROM runs WHERE tenant_id = $1 AND created_at >= NOW() - ($2 || ' days')::interval
		GROUP BY status ORDER BY COUNT(*) DESC
	`, tid, fmt.Sprintf("%d", days))
	if err != nil {
		return nil, fmt.Errorf("dashboard run outcomes: %w", err)
	}
	defer rows.Close()
	var result []dashboard.RunOutcome
	for rows.Next() {
		var ro dashboard.RunOutcome
		if err := rows.Scan(&ro.Status, &ro.Count); err != nil {
			return nil, fmt.Errorf("scan run outcome: %w", err)
		}
		result = append(result, ro)
	}
	if result == nil {
		result = []dashboard.RunOutcome{}
	}
	return result, rows.Err()
}

// DashboardAgentPerformance returns agent success rates for the bar chart.
// Computed from runs table via JOIN (agents table has no success_rate/total_runs columns).
func (s *Store) DashboardAgentPerformance(ctx context.Context) ([]dashboard.AgentPerf, error) {
	tid := tenantFromCtx(ctx)
	rows, err := s.pool.Query(ctx, `
		SELECT a.name,
			CASE WHEN COUNT(r.id) > 0
				THEN (SUM(CASE WHEN r.status = 'completed' THEN 1 ELSE 0 END)::float / COUNT(r.id)) * 100
				ELSE 0
			END AS success_rate,
			COUNT(r.id)::int AS total_runs
		FROM agents a
		JOIN runs r ON r.agent_id = a.id AND r.tenant_id = $1
		WHERE a.tenant_id = $1
		GROUP BY a.id, a.name
		HAVING COUNT(r.id) > 0
		ORDER BY success_rate DESC
		LIMIT 20
	`, tid)
	if err != nil {
		return nil, fmt.Errorf("dashboard agent perf: %w", err)
	}
	defer rows.Close()
	var result []dashboard.AgentPerf
	for rows.Next() {
		var ap dashboard.AgentPerf
		if err := rows.Scan(&ap.AgentName, &ap.SuccessRate, &ap.TotalRuns); err != nil {
			return nil, fmt.Errorf("scan agent perf: %w", err)
		}
		result = append(result, ap)
	}
	if result == nil {
		result = []dashboard.AgentPerf{}
	}
	return result, rows.Err()
}

// DashboardModelUsage returns cost per model for the pie chart.
func (s *Store) DashboardModelUsage(ctx context.Context) ([]dashboard.ModelUsage, error) {
	tid := tenantFromCtx(ctx)
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(model, 'unknown'), SUM(cost_usd)
		FROM runs WHERE tenant_id = $1 AND created_at >= CURRENT_DATE - 30
		GROUP BY model ORDER BY SUM(cost_usd) DESC
		LIMIT 10
	`, tid)
	if err != nil {
		return nil, fmt.Errorf("dashboard model usage: %w", err)
	}
	defer rows.Close()
	var result []dashboard.ModelUsage
	for rows.Next() {
		var mu dashboard.ModelUsage
		if err := rows.Scan(&mu.Model, &mu.CostUSD); err != nil {
			return nil, fmt.Errorf("scan model usage: %w", err)
		}
		result = append(result, mu)
	}
	if result == nil {
		result = []dashboard.ModelUsage{}
	}
	return result, rows.Err()
}

// DashboardCostByProject returns per-project costs for the horizontal bar chart.
func (s *Store) DashboardCostByProject(ctx context.Context) ([]dashboard.ProjectCost, error) {
	tid := tenantFromCtx(ctx)
	rows, err := s.pool.Query(ctx, `
		SELECT r.project_id, COALESCE(p.name, ''), SUM(r.cost_usd)
		FROM runs r LEFT JOIN projects p ON r.project_id = p.id
		WHERE r.tenant_id = $1
		GROUP BY r.project_id, p.name
		ORDER BY SUM(r.cost_usd) DESC
		LIMIT 20
	`, tid)
	if err != nil {
		return nil, fmt.Errorf("dashboard cost by project: %w", err)
	}
	defer rows.Close()
	var result []dashboard.ProjectCost
	for rows.Next() {
		var pc dashboard.ProjectCost
		if err := rows.Scan(&pc.ProjectID, &pc.ProjectName, &pc.CostUSD); err != nil {
			return nil, fmt.Errorf("scan cost by project: %w", err)
		}
		result = append(result, pc)
	}
	if result == nil {
		result = []dashboard.ProjectCost{}
	}
	return result, rows.Err()
}

// DashboardCostTrend returns daily cost aggregated across ALL projects (global).
func (s *Store) DashboardCostTrend(ctx context.Context, days int) ([]cost.DailyCost, error) {
	if days <= 0 {
		days = 30
	}
	tid := tenantFromCtx(ctx)
	rows, err := s.pool.Query(ctx,
		`SELECT TO_CHAR(created_at::date, 'YYYY-MM-DD'), SUM(cost_usd), SUM(tokens_in), SUM(tokens_out), COUNT(*)
		 FROM runs
		 WHERE tenant_id = $1 AND created_at >= NOW() - ($2 || ' days')::interval
		 GROUP BY created_at::date
		 ORDER BY created_at::date`, tid, fmt.Sprintf("%d", days))
	if err != nil {
		return nil, fmt.Errorf("dashboard cost trend: %w", err)
	}
	defer rows.Close()
	var result []cost.DailyCost
	for rows.Next() {
		var dc cost.DailyCost
		if err := rows.Scan(&dc.Date, &dc.CostUSD, &dc.TokensIn, &dc.TokensOut, &dc.RunCount); err != nil {
			return nil, fmt.Errorf("scan cost trend: %w", err)
		}
		result = append(result, dc)
	}
	if result == nil {
		result = []cost.DailyCost{}
	}
	return result, rows.Err()
}

// deltaPct computes percentage change from prev to cur. Returns 0 if prev is 0.
func deltaPct(cur, prev float64) float64 {
	if prev == 0 {
		if cur > 0 {
			return 100
		}
		return 0
	}
	return (cur - prev) / prev * 100
}
