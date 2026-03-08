package dashboard

import "math"

// DashboardStats holds the 7 KPI values with trend deltas.
type DashboardStats struct {
	CostTodayUSD     float64 `json:"cost_today_usd"`
	CostTodayDelta   float64 `json:"cost_today_delta_pct"`
	ActiveRuns       int     `json:"active_runs"`
	SuccessRate7d    float64 `json:"success_rate_7d_pct"`
	SuccessRateDelta float64 `json:"success_rate_delta_pct"`
	ActiveAgents     int     `json:"active_agents"`
	AvgCostPerRun    float64 `json:"avg_cost_per_run_usd"`
	AvgCostDelta     float64 `json:"avg_cost_delta_pct"`
	TokenUsageToday  int64   `json:"token_usage_today"`
	TokenUsageDelta  float64 `json:"token_usage_delta_pct"`
	ErrorRate24h     float64 `json:"error_rate_24h_pct"`
	ErrorRateDelta   float64 `json:"error_rate_delta_pct"`
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
	Score     int                `json:"score"`
	Level     string             `json:"level"`
	Factors   HealthFactors      `json:"factors"`
	Sparkline []float64          `json:"sparkline_7d"`
	Stats     ProjectHealthStats `json:"stats"`
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
