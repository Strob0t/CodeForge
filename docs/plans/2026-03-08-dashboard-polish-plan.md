# Dashboard Polish Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Transform the dashboard from a plain project list into a hybrid command center with KPI strip, enhanced project cards (health scores, sparklines), smart activity timeline, and 5 chart types.

**Architecture:** Three new Go endpoints (dashboard stats, project health, chart data) backed by SQL aggregations on existing tables. Frontend restructured with @unovis/solid for charts, new components for KPI strip, activity timeline, and health indicators. No new DB tables or migrations needed.

**Tech Stack:** Go (chi, pgx), SolidJS, @unovis/ts + @unovis/solid, Tailwind CSS v4

**Design Doc:** `docs/plans/2026-03-08-dashboard-polish-design.md`

---

## Task 1: Install @unovis/solid dependency

**Files:**
- Modify: `frontend/package.json`

**Step 1: Install the charting library**

Run:
```bash
cd /workspaces/CodeForge/frontend && npm install @unovis/ts @unovis/solid
```

**Step 2: Verify installation**

Run:
```bash
cd /workspaces/CodeForge/frontend && node -e "require('@unovis/ts'); console.log('OK')"
```
Expected: `OK`

**Step 3: Commit**

```bash
git add frontend/package.json frontend/package-lock.json
git commit -m "deps: add @unovis/ts and @unovis/solid for dashboard charts"
```

---

## Task 2: Domain types for dashboard aggregation (Go)

**Files:**
- Create: `internal/domain/dashboard/dashboard.go`
- Test: `internal/domain/dashboard/dashboard_test.go`

**Step 1: Write the failing test**

```go
// internal/domain/dashboard/dashboard_test.go
package dashboard_test

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/dashboard"
)

func TestHealthScore_Clamp(t *testing.T) {
	tests := []struct {
		name  string
		input dashboard.HealthFactors
		want  int
	}{
		{
			name: "all perfect",
			input: dashboard.HealthFactors{
				SuccessRate:       100,
				ErrorRateInv:      100,
				ActivityFreshness: 100,
				TaskVelocity:      100,
				CostStability:     100,
			},
			want: 100,
		},
		{
			name: "all zero",
			input: dashboard.HealthFactors{},
			want:  0,
		},
		{
			name: "typical healthy",
			input: dashboard.HealthFactors{
				SuccessRate:       92,
				ErrorRateInv:      95,
				ActivityFreshness: 80,
				TaskVelocity:      72,
				CostStability:     85,
			},
			want: 87, // 92*0.30 + 95*0.25 + 80*0.20 + 72*0.15 + 85*0.10 = 87.85 -> 87
		},
		{
			name: "values above 100 clamped",
			input: dashboard.HealthFactors{
				SuccessRate:       150,
				ErrorRateInv:      150,
				ActivityFreshness: 150,
				TaskVelocity:      150,
				CostStability:     150,
			},
			want: 100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.Score()
			if got != tt.want {
				t.Errorf("Score() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHealthLevel(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{87, "healthy"},
		{75, "healthy"},
		{74, "warning"},
		{40, "warning"},
		{39, "critical"},
		{0, "critical"},
		{100, "healthy"},
	}
	for _, tt := range tests {
		got := dashboard.HealthLevel(tt.score)
		if got != tt.want {
			t.Errorf("HealthLevel(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/domain/dashboard/ -v`
Expected: FAIL — package does not exist

**Step 3: Write minimal implementation**

```go
// internal/domain/dashboard/dashboard.go
package dashboard

import "math"

// DashboardStats holds the 7 KPI values with trend deltas.
type DashboardStats struct {
	CostTodayUSD      float64 `json:"cost_today_usd"`
	CostTodayDelta    float64 `json:"cost_today_delta_pct"`
	ActiveRuns        int     `json:"active_runs"`
	SuccessRate7d     float64 `json:"success_rate_7d_pct"`
	SuccessRateDelta  float64 `json:"success_rate_delta_pct"`
	ActiveAgents      int     `json:"active_agents"`
	AvgCostPerRun     float64 `json:"avg_cost_per_run_usd"`
	AvgCostDelta      float64 `json:"avg_cost_delta_pct"`
	TokenUsageToday   int64   `json:"token_usage_today"`
	TokenUsageDelta   float64 `json:"token_usage_delta_pct"`
	ErrorRate24h      float64 `json:"error_rate_24h_pct"`
	ErrorRateDelta    float64 `json:"error_rate_delta_pct"`
}

// HealthFactors holds the 5 contributing factors (each 0-100).
type HealthFactors struct {
	SuccessRate       float64 `json:"success_rate"`
	ErrorRateInv      float64 `json:"error_rate_inv"`
	ActivityFreshness float64 `json:"activity_freshness"`
	TaskVelocity      float64 `json:"task_velocity"`
	CostStability     float64 `json:"cost_stability"`
}

// Score computes the weighted health score (0-100).
func (f HealthFactors) Score() int {
	clamp := func(v float64) float64 {
		return math.Min(math.Max(v, 0), 100)
	}
	raw := clamp(f.SuccessRate)*0.30 +
		clamp(f.ErrorRateInv)*0.25 +
		clamp(f.ActivityFreshness)*0.20 +
		clamp(f.TaskVelocity)*0.15 +
		clamp(f.CostStability)*0.10
	score := int(math.Min(raw, 100))
	return score
}

// HealthLevel returns "healthy", "warning", or "critical" based on score.
func HealthLevel(score int) string {
	switch {
	case score >= 75:
		return "healthy"
	case score >= 40:
		return "warning"
	default:
		return "critical"
	}
}

// ProjectHealth holds the complete health response for a single project.
type ProjectHealth struct {
	Score      int                `json:"score"`
	Level      string             `json:"level"`
	Factors    HealthFactors      `json:"factors"`
	Sparkline  []float64          `json:"sparkline_7d"`
	Stats      ProjectHealthStats `json:"stats"`
}

// ProjectHealthStats holds the per-project stats for the enhanced card.
type ProjectHealthStats struct {
	SuccessRatePct float64 `json:"success_rate_pct"`
	TotalRuns7d    int     `json:"total_runs_7d"`
	TotalCostUSD   float64 `json:"total_cost_usd"`
	CostDeltaPct   float64 `json:"cost_delta_pct"`
	ActiveAgents   int     `json:"active_agents"`
	RunningTasks   int     `json:"running_tasks"`
	TasksCompleted int     `json:"tasks_completed"`
	TasksTotal     int     `json:"tasks_total"`
	LastActivity   string  `json:"last_activity"`
	LastActivityAt string  `json:"last_activity_at"`
}

// ChartData holds pre-aggregated data for a single chart type.
type ChartData struct {
	Type   string      `json:"type"`
	Points []DataPoint `json:"points"`
}

// DataPoint is a flexible chart data point.
type DataPoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Extra string  `json:"extra,omitempty"`
}

// RunOutcome holds run status counts for the donut chart.
type RunOutcome struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// AgentPerf holds agent performance for the bar chart.
type AgentPerf struct {
	AgentName   string  `json:"agent_name"`
	SuccessRate float64 `json:"success_rate"`
	TotalRuns   int     `json:"total_runs"`
}

// ModelUsage holds model cost share for the pie chart.
type ModelUsage struct {
	Model   string  `json:"model"`
	CostUSD float64 `json:"cost_usd"`
}

// ProjectCost holds per-project cost for the horizontal bar chart.
type ProjectCost struct {
	ProjectID   string  `json:"project_id"`
	ProjectName string  `json:"project_name"`
	CostUSD     float64 `json:"cost_usd"`
}
```

**Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && go test ./internal/domain/dashboard/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/domain/dashboard/
git commit -m "feat(dashboard): add domain types and health score formula"
```

---

## Task 3: Store interface + SQL queries for dashboard (Go)

**Files:**
- Modify: `internal/port/database/store.go` (add 3 methods after line 117)
- Create: `internal/adapter/postgres/store_dashboard.go`

**Step 1: Add interface methods to store.go**

After line 117 (after `CostByToolForRun`), add:

```go
	// Dashboard Aggregation
	DashboardStats(ctx context.Context) (*dashboard.DashboardStats, error)
	ProjectHealth(ctx context.Context, projectID string) (*dashboard.ProjectHealth, error)
	DashboardRunOutcomes(ctx context.Context, days int) ([]dashboard.RunOutcome, error)
	DashboardAgentPerformance(ctx context.Context) ([]dashboard.AgentPerf, error)
	DashboardModelUsage(ctx context.Context) ([]dashboard.ModelUsage, error)
	DashboardCostByProject(ctx context.Context) ([]dashboard.ProjectCost, error)
```

Add import: `"github.com/Strob0t/CodeForge/internal/domain/dashboard"`

**Step 2: Write the SQL implementation**

```go
// internal/adapter/postgres/store_dashboard.go
package postgres

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/dashboard"
)

// DashboardStats returns the 7 KPI values with trend deltas.
func (s *Store) DashboardStats(ctx context.Context) (*dashboard.DashboardStats, error) {
	tid := tenantFromCtx(ctx)
	var ds dashboard.DashboardStats

	// Cost today + yesterday for delta
	err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN r.created_at::date = CURRENT_DATE THEN r.cost_usd ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN r.created_at::date = CURRENT_DATE - 1 THEN r.cost_usd ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN r.created_at::date = CURRENT_DATE THEN r.tokens_in + r.tokens_out ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN r.created_at::date = CURRENT_DATE - 1 THEN r.tokens_in + r.tokens_out ELSE 0 END), 0)
		FROM runs r WHERE r.tenant_id = $1
			AND r.created_at >= CURRENT_DATE - 1
	`, tid).Scan(&ds.CostTodayUSD, new(float64), &ds.TokenUsageToday, new(int64))
	if err != nil {
		return nil, fmt.Errorf("dashboard cost today: %w", err)
	}

	// Full stats query (single pass over runs table)
	var costYesterday float64
	var tokensYesterday int64
	var completedCur, failedCur, timeoutCur, totalCur int
	var completedPrev, failedPrev, timeoutPrev, totalPrev int
	var avgCostCur, avgCostPrev float64
	var failedRecent, totalRecent int

	err = s.pool.QueryRow(ctx, `
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

	// Active runs + agents (separate queries for real-time counts)
	_ = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM runs WHERE tenant_id = $1 AND status = 'running'`, tid,
	).Scan(&ds.ActiveRuns)

	_ = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM agents WHERE tenant_id = $1 AND status = 'running'`, tid,
	).Scan(&ds.ActiveAgents)

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

	// Tasks
	_ = s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END), 0),
			COUNT(*)
		FROM tasks WHERE project_id = $1 AND tenant_id = $2
	`, projectID, tid).Scan(&tasksCompleted, &tasksTotal)

	// Active agents + running tasks
	_ = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM agents WHERE project_id = $1 AND tenant_id = $2 AND status = 'running'`,
		projectID, tid,
	).Scan(&activeAgents)

	_ = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tasks WHERE project_id = $1 AND tenant_id = $2 AND status = 'running'`,
		projectID, tid,
	).Scan(&runningTasks)

	// Last activity (most recent run)
	_ = s.pool.QueryRow(ctx, `
		SELECT COALESCE(status, ''), completed_at
		FROM runs WHERE project_id = $1 AND tenant_id = $2
		ORDER BY created_at DESC LIMIT 1
	`, projectID, tid).Scan(&lastActivity, &lastActivityAt)

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
		FROM runs WHERE tenant_id = $1 AND created_at >= CURRENT_DATE - $2
		GROUP BY status ORDER BY COUNT(*) DESC
	`, tid, days)
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
func (s *Store) DashboardAgentPerformance(ctx context.Context) ([]dashboard.AgentPerf, error) {
	tid := tenantFromCtx(ctx)
	rows, err := s.pool.Query(ctx, `
		SELECT a.name,
			COALESCE(a.success_rate, 0),
			COALESCE(a.total_runs, 0)
		FROM agents a
		WHERE a.tenant_id = $1 AND a.total_runs > 0
		ORDER BY a.success_rate DESC
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
```

**Step 3: Run compilation check**

Run: `cd /workspaces/CodeForge && go build ./internal/...`
Expected: PASS (may fail on interface — fix by adding methods to mock stores in tests)

**Step 4: Commit**

```bash
git add internal/port/database/store.go internal/adapter/postgres/store_dashboard.go
git commit -m "feat(dashboard): add store interface + SQL queries for dashboard aggregation"
```

---

## Task 4: Dashboard service (Go)

**Files:**
- Create: `internal/service/dashboard.go`
- Create: `internal/service/dashboard_test.go`

**Step 1: Write the failing test**

```go
// internal/service/dashboard_test.go
package service

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/dashboard"
)

func TestDashboardService_Stats(t *testing.T) {
	store := &mockStore{}
	svc := NewDashboardService(store)
	ctx := context.Background()

	stats, err := svc.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}
}

func TestDashboardService_ProjectHealth(t *testing.T) {
	store := &mockStore{}
	svc := NewDashboardService(store)
	ctx := context.Background()

	health, err := svc.ProjectHealth(ctx, "proj-1")
	if err != nil {
		t.Fatalf("ProjectHealth: %v", err)
	}
	if health == nil {
		t.Fatal("expected non-nil health")
	}
	if health.Level == "" {
		t.Error("expected non-empty level")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestDashboard -v`
Expected: FAIL — `NewDashboardService` undefined

**Step 3: Write minimal implementation**

```go
// internal/service/dashboard.go
package service

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/dashboard"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// DashboardService provides aggregated dashboard data.
type DashboardService struct {
	store database.Store
}

// NewDashboardService creates a new DashboardService.
func NewDashboardService(store database.Store) *DashboardService {
	return &DashboardService{store: store}
}

// Stats returns the 7 KPI values for the dashboard header.
func (s *DashboardService) Stats(ctx context.Context) (*dashboard.DashboardStats, error) {
	return s.store.DashboardStats(ctx)
}

// ProjectHealth returns health score + stats for a single project.
func (s *DashboardService) ProjectHealth(ctx context.Context, projectID string) (*dashboard.ProjectHealth, error) {
	return s.store.ProjectHealth(ctx, projectID)
}

// RunOutcomes returns run status counts for the donut chart.
func (s *DashboardService) RunOutcomes(ctx context.Context, days int) ([]dashboard.RunOutcome, error) {
	return s.store.DashboardRunOutcomes(ctx, days)
}

// AgentPerformance returns agent success rates for the bar chart.
func (s *DashboardService) AgentPerformance(ctx context.Context) ([]dashboard.AgentPerf, error) {
	return s.store.DashboardAgentPerformance(ctx)
}

// ModelUsage returns cost per model for the pie chart.
func (s *DashboardService) ModelUsage(ctx context.Context) ([]dashboard.ModelUsage, error) {
	return s.store.DashboardModelUsage(ctx)
}

// CostByProject returns per-project costs for the horizontal bar chart.
func (s *DashboardService) CostByProject(ctx context.Context) ([]dashboard.ProjectCost, error) {
	return s.store.DashboardCostByProject(ctx)
}

// CostTrend delegates to the cost time series with all-projects aggregation.
func (s *DashboardService) CostTrend(ctx context.Context, days int) ([]cost.DailyCost, error) {
	return s.store.CostTimeSeries(ctx, "", days)
}
```

**Step 4: Add mock methods to existing mockStore in test helpers**

Add these methods to the `mockStore` in `internal/service/` test files so the interface is satisfied:

```go
func (m *mockStore) DashboardStats(_ context.Context) (*dashboard.DashboardStats, error) {
	return &dashboard.DashboardStats{}, nil
}
func (m *mockStore) ProjectHealth(_ context.Context, _ string) (*dashboard.ProjectHealth, error) {
	return &dashboard.ProjectHealth{Level: "healthy", Sparkline: []float64{}}, nil
}
func (m *mockStore) DashboardRunOutcomes(_ context.Context, _ int) ([]dashboard.RunOutcome, error) {
	return []dashboard.RunOutcome{}, nil
}
func (m *mockStore) DashboardAgentPerformance(_ context.Context) ([]dashboard.AgentPerf, error) {
	return []dashboard.AgentPerf{}, nil
}
func (m *mockStore) DashboardModelUsage(_ context.Context) ([]dashboard.ModelUsage, error) {
	return []dashboard.ModelUsage{}, nil
}
func (m *mockStore) DashboardCostByProject(_ context.Context) ([]dashboard.ProjectCost, error) {
	return []dashboard.ProjectCost{}, nil
}
```

**Step 5: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestDashboard -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/service/dashboard.go internal/service/dashboard_test.go
git commit -m "feat(dashboard): add DashboardService with stats, health, chart methods"
```

---

## Task 5: HTTP handlers + routes for dashboard (Go)

**Files:**
- Create: `internal/adapter/http/handlers_dashboard.go`
- Modify: `internal/adapter/http/handlers.go:92` (add `Dashboard` field)
- Modify: `internal/adapter/http/routes.go:221` (add routes after cost routes)
- Modify: `cmd/codeforge/main.go:462` (wire service)

**Step 1: Create handlers**

```go
// internal/adapter/http/handlers_dashboard.go
package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/dashboard"
)

// DashboardStats handles GET /api/v1/dashboard/stats
func (h *Handlers) DashboardStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Dashboard.Stats(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// ProjectHealth handles GET /api/v1/projects/{id}/health
func (h *Handlers) ProjectHealth(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	health, err := h.Dashboard.ProjectHealth(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, health)
}

// DashboardRunOutcomes handles GET /api/v1/dashboard/charts/run-outcomes
func (h *Handlers) DashboardRunOutcomes(w http.ResponseWriter, r *http.Request) {
	days := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}
	outcomes, err := h.Dashboard.RunOutcomes(r.Context(), days)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if outcomes == nil {
		outcomes = []dashboard.RunOutcome{}
	}
	writeJSON(w, http.StatusOK, outcomes)
}

// DashboardAgentPerformance handles GET /api/v1/dashboard/charts/agent-performance
func (h *Handlers) DashboardAgentPerformance(w http.ResponseWriter, r *http.Request) {
	agents, err := h.Dashboard.AgentPerformance(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if agents == nil {
		agents = []dashboard.AgentPerf{}
	}
	writeJSON(w, http.StatusOK, agents)
}

// DashboardModelUsage handles GET /api/v1/dashboard/charts/model-usage
func (h *Handlers) DashboardModelUsage(w http.ResponseWriter, r *http.Request) {
	models, err := h.Dashboard.ModelUsage(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if models == nil {
		models = []dashboard.ModelUsage{}
	}
	writeJSON(w, http.StatusOK, models)
}

// DashboardCostByProject handles GET /api/v1/dashboard/charts/cost-by-project
func (h *Handlers) DashboardCostByProject(w http.ResponseWriter, r *http.Request) {
	costs, err := h.Dashboard.CostByProject(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if costs == nil {
		costs = []dashboard.ProjectCost{}
	}
	writeJSON(w, http.StatusOK, costs)
}

// DashboardCostTrend handles GET /api/v1/dashboard/charts/cost-trend
func (h *Handlers) DashboardCostTrend(w http.ResponseWriter, r *http.Request) {
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}
	trend, err := h.Dashboard.CostTrend(r.Context(), days)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, trend)
}
```

**Step 2: Add Dashboard field to Handlers struct**

In `internal/adapter/http/handlers.go`, after line 92 (`GoalDiscovery`), add:

```go
	Dashboard    *service.DashboardService
```

**Step 3: Register routes**

In `internal/adapter/http/routes.go`, after line 220 (after `r.Get("/runs/{id}/costs/by-tool"...)`), add:

```go
		// Dashboard aggregation
		r.Get("/dashboard/stats", h.DashboardStats)
		r.Get("/dashboard/charts/cost-trend", h.DashboardCostTrend)
		r.Get("/dashboard/charts/run-outcomes", h.DashboardRunOutcomes)
		r.Get("/dashboard/charts/agent-performance", h.DashboardAgentPerformance)
		r.Get("/dashboard/charts/model-usage", h.DashboardModelUsage)
		r.Get("/dashboard/charts/cost-by-project", h.DashboardCostByProject)
		r.Get("/projects/{id}/health", h.ProjectHealth)
```

**Step 4: Wire in main.go**

In `cmd/codeforge/main.go`, after line 462 (`costSvc := service.NewCostService(store)`), add:

```go
	// --- Dashboard Service ---
	dashboardSvc := service.NewDashboardService(store)
```

And in the Handlers struct literal (after `Cost: costSvc,` around line 628), add:

```go
		Dashboard:        dashboardSvc,
```

**Step 5: Build check**

Run: `cd /workspaces/CodeForge && go build ./...`
Expected: PASS (may need to add mock methods to handler test mockStore too)

**Step 6: Commit**

```bash
git add internal/adapter/http/handlers_dashboard.go internal/adapter/http/handlers.go internal/adapter/http/routes.go cmd/codeforge/main.go
git commit -m "feat(dashboard): add HTTP handlers and routes for dashboard API"
```

---

## Task 6: TypeScript types + API client for dashboard (Frontend)

**Files:**
- Modify: `frontend/src/api/types.ts` (add types)
- Modify: `frontend/src/api/client.ts` (add dashboard API methods)

**Step 1: Add TypeScript types**

Append to `frontend/src/api/types.ts`:

```typescript
// --- Dashboard ---

export interface DashboardStats {
  cost_today_usd: number;
  cost_today_delta_pct: number;
  active_runs: number;
  success_rate_7d_pct: number;
  success_rate_delta_pct: number;
  active_agents: number;
  avg_cost_per_run_usd: number;
  avg_cost_delta_pct: number;
  token_usage_today: number;
  token_usage_delta_pct: number;
  error_rate_24h_pct: number;
  error_rate_delta_pct: number;
}

export interface HealthFactors {
  success_rate: number;
  error_rate_inv: number;
  activity_freshness: number;
  task_velocity: number;
  cost_stability: number;
}

export interface ProjectHealthStats {
  success_rate_pct: number;
  total_runs_7d: number;
  total_cost_usd: number;
  cost_delta_pct: number;
  active_agents: number;
  running_tasks: number;
  tasks_completed: number;
  tasks_total: number;
  last_activity: string;
  last_activity_at: string;
}

export interface ProjectHealth {
  score: number;
  level: "healthy" | "warning" | "critical";
  factors: HealthFactors;
  sparkline_7d: number[];
  stats: ProjectHealthStats;
}

export interface RunOutcome {
  status: string;
  count: number;
}

export interface AgentPerf {
  agent_name: string;
  success_rate: number;
  total_runs: number;
}

export interface ModelUsage {
  model: string;
  cost_usd: number;
}

export interface ProjectCostBar {
  project_id: string;
  project_name: string;
  cost_usd: number;
}
```

**Step 2: Add API client methods**

In `frontend/src/api/client.ts`, add a `dashboard` section to the `api` object (after the `costs` section around line 556):

```typescript
  dashboard: {
    stats: () => request<DashboardStats>("/dashboard/stats"),

    projectHealth: (id: string) =>
      request<ProjectHealth>(url`/projects/${id}/health`),

    costTrend: (days = 30) =>
      request<DailyCost[]>(url`/dashboard/charts/cost-trend?days=${days}`),

    runOutcomes: (days = 7) =>
      request<RunOutcome[]>(url`/dashboard/charts/run-outcomes?days=${days}`),

    agentPerformance: () =>
      request<AgentPerf[]>("/dashboard/charts/agent-performance"),

    modelUsage: () =>
      request<ModelUsage[]>("/dashboard/charts/model-usage"),

    costByProject: () =>
      request<ProjectCostBar[]>("/dashboard/charts/cost-by-project"),
  },
```

Add the new types to the import block at the top of `client.ts`.

**Step 3: TypeScript check**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: PASS

**Step 4: Commit**

```bash
git add frontend/src/api/types.ts frontend/src/api/client.ts
git commit -m "feat(dashboard): add TypeScript types and API client for dashboard endpoints"
```

---

## Task 7: KpiStrip component (Frontend)

**Files:**
- Create: `frontend/src/features/dashboard/KpiStrip.tsx`

**Step 1: Create KpiStrip**

```typescript
// frontend/src/features/dashboard/KpiStrip.tsx
import { For, Show, type Component } from "solid-js";
import type { DashboardStats } from "~/api/types";

interface KpiCardProps {
  label: string;
  value: string;
  delta: number;
  invertDelta?: boolean; // true = up is bad (cost, errors)
}

const KpiCard: Component<KpiCardProps> = (props) => {
  const isPositive = () => props.invertDelta ? props.delta < 0 : props.delta > 0;
  const isNegative = () => props.invertDelta ? props.delta > 0 : props.delta < 0;
  const arrow = () => props.delta > 0 ? "\u2191" : props.delta < 0 ? "\u2193" : "";

  return (
    <div class="min-w-[130px] rounded-lg border border-cf-border bg-cf-bg-surface p-3 text-center">
      <Show when={props.delta !== 0}>
        <p
          class={
            "text-xs font-medium " +
            (isPositive() ? "text-cf-success" : isNegative() ? "text-cf-danger" : "text-cf-text-muted")
          }
        >
          {arrow()} {Math.abs(props.delta).toFixed(1)}%
        </p>
      </Show>
      <p class="text-xl font-bold text-cf-text-primary">{props.value}</p>
      <p class="text-xs text-cf-text-muted">{props.label}</p>
    </div>
  );
};

interface KpiStripProps {
  stats: DashboardStats | undefined;
}

const KpiStrip: Component<KpiStripProps> = (props) => {
  const cards = () => {
    const s = props.stats;
    if (!s) return [];
    return [
      { label: "Cost Today", value: `$${s.cost_today_usd.toFixed(2)}`, delta: s.cost_today_delta_pct, invertDelta: true },
      { label: "Active Runs", value: String(s.active_runs), delta: 0 },
      { label: "Success Rate (7d)", value: `${s.success_rate_7d_pct.toFixed(1)}%`, delta: s.success_rate_delta_pct },
      { label: "Active Agents", value: String(s.active_agents), delta: 0 },
      { label: "Avg Cost/Run", value: `$${s.avg_cost_per_run_usd.toFixed(2)}`, delta: s.avg_cost_delta_pct, invertDelta: true },
      { label: "Tokens Today", value: formatTokens(s.token_usage_today), delta: s.token_usage_delta_pct, invertDelta: true },
      { label: "Error Rate (24h)", value: `${s.error_rate_24h_pct.toFixed(1)}%`, delta: s.error_rate_delta_pct, invertDelta: true },
    ];
  };

  return (
    <div class="flex gap-3 overflow-x-auto pb-2">
      <For each={cards()}>
        {(card) => <KpiCard {...card} />}
      </For>
    </div>
  );
};

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

export default KpiStrip;
```

**Step 2: TypeScript check**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: PASS

**Step 3: Commit**

```bash
git add frontend/src/features/dashboard/KpiStrip.tsx
git commit -m "feat(dashboard): add KpiStrip component with 7 stat cards"
```

---

## Task 8: HealthDot component (Frontend)

**Files:**
- Create: `frontend/src/features/dashboard/HealthDot.tsx`

**Step 1: Create HealthDot with tooltip**

```typescript
// frontend/src/features/dashboard/HealthDot.tsx
import { createSignal, Show, type Component } from "solid-js";
import type { HealthFactors } from "~/api/types";

interface HealthDotProps {
  score: number;
  level: "healthy" | "warning" | "critical";
  factors: HealthFactors;
}

const HealthDot: Component<HealthDotProps> = (props) => {
  const [showTooltip, setShowTooltip] = createSignal(false);

  const color = () => {
    switch (props.level) {
      case "healthy": return "bg-cf-success";
      case "warning": return "bg-cf-warning";
      case "critical": return "bg-cf-danger";
    }
  };

  const factorRows = () => [
    { label: "Success rate (7d)", value: props.factors.success_rate },
    { label: "Error rate (24h)", value: props.factors.error_rate_inv },
    { label: "Recent activity", value: props.factors.activity_freshness },
    { label: "Task velocity", value: props.factors.task_velocity },
    { label: "Cost stability", value: props.factors.cost_stability },
  ];

  return (
    <div
      class="relative inline-flex"
      onMouseEnter={() => setShowTooltip(true)}
      onMouseLeave={() => setShowTooltip(false)}
    >
      <span
        class={`inline-block h-3 w-3 rounded-full ${color()}`}
        title={`Health: ${props.score}`}
      />
      <Show when={showTooltip()}>
        <div class="absolute left-5 top-0 z-50 w-56 rounded-lg border border-cf-border bg-cf-bg-surface p-3 shadow-lg">
          <p class="mb-2 text-sm font-bold text-cf-text-primary">
            Health Score: {props.score}
          </p>
          <div class="space-y-1.5">
            {factorRows().map((f) => (
              <div class="flex items-center gap-2 text-xs">
                <span class="w-28 text-cf-text-muted">{f.label}</span>
                <div class="h-1.5 flex-1 rounded-full bg-cf-bg-surface-alt">
                  <div
                    class={`h-1.5 rounded-full ${barColor(f.value)}`}
                    style={{ width: `${Math.min(f.value, 100)}%` }}
                  />
                </div>
                <span class="w-8 text-right font-mono text-cf-text-secondary">
                  {Math.round(f.value)}%
                </span>
              </div>
            ))}
          </div>
        </div>
      </Show>
    </div>
  );
};

function barColor(value: number): string {
  if (value >= 75) return "bg-cf-success";
  if (value >= 40) return "bg-cf-warning";
  return "bg-cf-danger";
}

export default HealthDot;
```

**Step 2: TypeScript check**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: PASS

**Step 3: Commit**

```bash
git add frontend/src/features/dashboard/HealthDot.tsx
git commit -m "feat(dashboard): add HealthDot component with hover tooltip breakdown"
```

---

## Task 9: Chart components — 5 Unovis charts (Frontend)

**Files:**
- Create: `frontend/src/features/dashboard/charts/CostTrendChart.tsx`
- Create: `frontend/src/features/dashboard/charts/RunOutcomesDonut.tsx`
- Create: `frontend/src/features/dashboard/charts/AgentPerformanceBars.tsx`
- Create: `frontend/src/features/dashboard/charts/ModelUsagePie.tsx`
- Create: `frontend/src/features/dashboard/charts/CostByProjectBars.tsx`

**This task is parallelizable** — all 5 chart components are independent.

Exact implementation will use @unovis/solid components:
- `VisXYContainer`, `VisLine`, `VisAxis` for cost trend
- `VisSingleContainer`, `VisDonut` for donut/pie charts
- `VisXYContainer`, `VisGroupedBar`, `VisAxis` for bar charts

Each chart component follows the same pattern:
1. Accept data as props (typed from api/types.ts)
2. Transform data into Unovis-compatible format
3. Render in a fixed-height container
4. Use CSS variables for theming

**Reference:** Check Unovis docs for SolidJS API at https://unovis.dev/docs/ during implementation. Use `context7` MCP tool for up-to-date API examples.

**Step 1: Create all 5 chart components** (see design doc Section 5 for spec per chart)

**Step 2: TypeScript check**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: PASS

**Step 3: Commit**

```bash
git add frontend/src/features/dashboard/charts/
git commit -m "feat(dashboard): add 5 Unovis chart components (cost trend, donut, bars, pie)"
```

---

## Task 10: ChartsPanel tabbed container (Frontend)

**Files:**
- Create: `frontend/src/features/dashboard/ChartsPanel.tsx`

**Step 1: Create tabbed panel that lazy-loads chart data per tab**

The panel has 5 tabs. Each tab fetches its data on first activation using `createResource`. The cost trend tab is the default.

Uses the existing `Tabs` component from `~/ui` if available, or a simple custom tab strip with buttons.

**Step 2: TypeScript check + Commit**

```bash
git add frontend/src/features/dashboard/ChartsPanel.tsx
git commit -m "feat(dashboard): add ChartsPanel with 5 tabbed chart views"
```

---

## Task 11: ActivityTimeline component (Frontend)

**Files:**
- Create: `frontend/src/features/dashboard/ActivityTimeline.tsx`

**Step 1: Create smart-prioritized timeline**

- Subscribes to WebSocket events via `useWebSocket()` hook
- Maintains a sorted buffer (priority-tier first, then recency)
- Each entry: colored dot, project name, summary, relative time, navigate button
- Click handler uses `useNavigate()` from `@solidjs/router`
- Max 15 visible entries, "Show more" expands

Priority tiers (from design doc Section 3):
- Tier 1 (red): agent.error, run.failed, run.stall_detected
- Tier 2 (yellow): run.budget_alert, run.qualitygate.failed
- Tier 3 (green): run.completed, run.delivery.completed
- Tier 4 (blue): agent.started, run.started
- Tier 5 (gray): agent.step_done, agent.tool_called

**Step 2: TypeScript check + Commit**

```bash
git add frontend/src/features/dashboard/ActivityTimeline.tsx
git commit -m "feat(dashboard): add smart-prioritized ActivityTimeline component"
```

---

## Task 12: CreateProjectModal (Frontend)

**Files:**
- Create: `frontend/src/features/dashboard/CreateProjectModal.tsx`
- Modify: `frontend/src/features/dashboard/DashboardPage.tsx` (extract form)

**Step 1: Extract the project creation form from DashboardPage into a Modal**

Move the form content (currently inline in DashboardPage.tsx, approximately lines 350-550) into a `CreateProjectModal` component. The modal wraps the existing `Modal` component from `~/ui`.

Props:
```typescript
interface CreateProjectModalProps {
  open: boolean;
  onClose: () => void;
  onCreated: () => void; // triggers refetch
}
```

**Step 2: Commit**

```bash
git add frontend/src/features/dashboard/CreateProjectModal.tsx
git commit -m "refactor(dashboard): extract project creation form into modal component"
```

---

## Task 13: Enhanced ProjectCard (Frontend)

**Files:**
- Modify: `frontend/src/features/dashboard/ProjectCard.tsx`

**Step 1: Enhance ProjectCard with health, sparkline, and stats**

The card now receives `ProjectHealth` data alongside the `Project` data. New props:

```typescript
interface ProjectCardProps {
  project: Project;
  health: ProjectHealth | undefined;
  onDelete: (id: string) => void;
  onEdit: (project: Project) => void;
}
```

Card layout (from design doc Section 2):
- Header: HealthDot + project name + provider badge
- Description + branch info
- Stats row: success rate bar, cost, task progress bar
- Sparkline: tiny VisLine (120x24px) showing 7-day cost trend
- Agent/task summary line + last activity
- Footer: Open Project, Edit, Delete buttons

The sparkline uses `VisXYContainer` + `VisLine` in a minimal container with no axes.

**Step 2: Commit**

```bash
git add frontend/src/features/dashboard/ProjectCard.tsx
git commit -m "feat(dashboard): enhance ProjectCard with health dot, sparkline, stats"
```

---

## Task 14: Rewrite DashboardPage layout (Frontend)

**Files:**
- Modify: `frontend/src/features/dashboard/DashboardPage.tsx`

**Step 1: Restructure to new layout**

The page now:
1. Fetches `DashboardStats` via `createResource(() => api.dashboard.stats())`
2. Fetches project list via `createResource(() => api.projects.list())`
3. Fetches health for each project via parallel `api.dashboard.projectHealth(id)` calls
4. Renders:
   - KpiStrip at top
   - Project grid in middle (enhanced ProjectCards)
   - Bottom row: ActivityTimeline (40%) + ChartsPanel (60%)
5. [+ New Project] button opens CreateProjectModal

Key reactive pattern:
```typescript
const [stats] = createResource(() => api.dashboard.stats());
const [projects, { refetch }] = createResource(() => api.projects.list());

// Fetch health for all projects once project list loads
const [healthMap] = createResource(
  () => projects(),
  async (projs) => {
    const entries = await Promise.all(
      projs.map(async (p) => [p.id, await api.dashboard.projectHealth(p.id)] as const)
    );
    return Object.fromEntries(entries);
  }
);
```

**Step 2: TypeScript check**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: PASS

**Step 3: Visual check**

Run: `cd /workspaces/CodeForge/frontend && npm run dev`
Open `http://localhost:3000` and verify layout renders.

**Step 4: Commit**

```bash
git add frontend/src/features/dashboard/DashboardPage.tsx
git commit -m "feat(dashboard): rewrite DashboardPage with KPI strip, enhanced cards, charts, timeline"
```

---

## Task 15: CSS theme integration for Unovis (Frontend)

**Files:**
- Modify: `frontend/src/index.css`

**Step 1: Add Unovis CSS variable mappings**

In `frontend/src/index.css`, add after the existing `--cf-*` token definitions:

```css
/* Unovis chart theme integration */
:root {
  --vis-color0: var(--cf-accent);
  --vis-color1: var(--cf-success);
  --vis-color2: var(--cf-danger);
  --vis-color3: var(--cf-warning);
  --vis-color4: var(--cf-text-muted);
  --vis-font-family: inherit;
}
```

This ensures all charts automatically follow the light/dark theme.

**Step 2: Commit**

```bash
git add frontend/src/index.css
git commit -m "style(dashboard): add Unovis CSS variable mappings for theme integration"
```

---

## Task 16: i18n translations for dashboard (Frontend)

**Files:**
- Modify: `frontend/src/i18n/locales/en.ts`
- Modify: `frontend/src/i18n/locales/de.ts`

**Step 1: Add dashboard translation keys**

Add to both locale files:

```typescript
// English
"dashboard.kpi.costToday": "Cost Today",
"dashboard.kpi.activeRuns": "Active Runs",
"dashboard.kpi.successRate": "Success Rate (7d)",
"dashboard.kpi.activeAgents": "Active Agents",
"dashboard.kpi.avgCostRun": "Avg Cost/Run",
"dashboard.kpi.tokenUsage": "Tokens Today",
"dashboard.kpi.errorRate": "Error Rate (24h)",
"dashboard.health.title": "Health Score",
"dashboard.health.successRate": "Success rate (7d)",
"dashboard.health.errorRate": "Error rate (24h)",
"dashboard.health.activity": "Recent activity",
"dashboard.health.taskVelocity": "Task velocity",
"dashboard.health.costStability": "Cost stability",
"dashboard.charts.costTrend": "Cost Trend",
"dashboard.charts.runOutcomes": "Run Outcomes",
"dashboard.charts.agentPerf": "Agents",
"dashboard.charts.modelUsage": "Models",
"dashboard.charts.costByProject": "Cost/Project",
"dashboard.timeline.title": "Activity",
"dashboard.timeline.showMore": "Show more...",
"dashboard.newProject": "New Project",
```

**Step 2: Commit**

```bash
git add frontend/src/i18n/locales/en.ts frontend/src/i18n/locales/de.ts
git commit -m "i18n: add dashboard KPI, health, chart, and timeline translation keys"
```

---

## Task 17: Update mock stores for Go compilation (Go)

**Files:**
- Modify: `internal/adapter/http/handlers_test.go` (add mock methods)
- Modify: any other test files with mockStore that fail to compile

**Step 1: Add dashboard mock methods to all mockStores**

Every mockStore that implements `database.Store` needs the 6 new methods. Add no-op implementations returning empty results.

**Step 2: Full test suite**

Run: `cd /workspaces/CodeForge && go test ./... 2>&1 | tail -20`
Expected: All existing tests still pass

**Step 3: Commit**

```bash
git add internal/
git commit -m "test: add dashboard mock methods to all test mockStores"
```

---

## Task 18: E2E smoke test (Frontend)

**Files:**
- Create: `frontend/e2e/dashboard.spec.ts`

**Step 1: Write basic E2E test**

```typescript
import { expect, test } from "@playwright/test";

test.describe("Dashboard", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/login");
    await page.fill('[id="email"]', "admin@localhost");
    await page.fill('[id="password"]', "Changeme123");
    await page.click('button[type="submit"]');
    await page.waitForURL("/");
  });

  test("renders KPI strip", async ({ page }) => {
    await expect(page.locator("text=Cost Today")).toBeVisible({ timeout: 10_000 });
    await expect(page.locator("text=Active Runs")).toBeVisible();
    await expect(page.locator("text=Success Rate")).toBeVisible();
  });

  test("renders project cards", async ({ page }) => {
    // At minimum the "New Project" button should be visible
    await expect(page.getByRole("button", { name: /New Project/i })).toBeVisible({ timeout: 10_000 });
  });

  test("opens create project modal", async ({ page }) => {
    await page.getByRole("button", { name: /New Project/i }).click();
    await expect(page.locator("text=Create")).toBeVisible({ timeout: 5_000 });
  });
});
```

**Step 2: Commit**

```bash
git add frontend/e2e/dashboard.spec.ts
git commit -m "test: add E2E smoke tests for dashboard KPI strip and project cards"
```

---

## Task 19: Documentation updates

**Files:**
- Modify: `docs/todo.md` (mark dashboard task complete, add new sub-tasks)
- Modify: `docs/project-status.md` (update dashboard milestone)
- Modify: `docs/features/01-project-dashboard.md` (add dashboard polish section)
- Modify: `docs/tech-stack.md` (add @unovis/solid)

**Step 1: Update all docs**

**Step 2: Commit**

```bash
git add docs/
git commit -m "docs: update todo, project-status, feature docs for dashboard polish"
```

---

## Dependency Graph

```
Task 1 (npm install)
  |
  v
Task 2 (domain types) ---> Task 3 (store + SQL) ---> Task 4 (service) ---> Task 5 (handlers + routes)
                                                                              |
                                                                              v
Task 6 (TS types + API client) ---> Task 7 (KpiStrip)     ]
                               ---> Task 8 (HealthDot)     ] parallel
                               ---> Task 9 (5 charts)      ]
                               ---> Task 10 (ChartsPanel)   ] depends on Task 9
                               ---> Task 11 (Timeline)     ]
                               ---> Task 12 (Modal extract) ]
                               ---> Task 13 (ProjectCard)   ] depends on Task 8
                                        |
                                        v
                               Task 14 (DashboardPage rewrite) -- depends on all above
                                        |
                                        v
                               Task 15 (CSS) + Task 16 (i18n) -- parallel
                                        |
                                        v
                               Task 17 (mock stores) + Task 18 (E2E) -- parallel
                                        |
                                        v
                               Task 19 (docs)
```

**Parallelizable groups:**
- Tasks 7, 8, 9, 11, 12 can run in parallel after Task 6
- Tasks 15, 16 can run in parallel
- Tasks 17, 18 can run in parallel
