// Package cost defines domain types for cost and token aggregation.
package cost

// Summary holds aggregate cost and token metrics.
type Summary struct {
	TotalCostUSD   float64 `json:"total_cost_usd"`
	TotalTokensIn  int64   `json:"total_tokens_in"`
	TotalTokensOut int64   `json:"total_tokens_out"`
	RunCount       int     `json:"run_count"`
}

// ProjectSummary extends Summary with project identification.
type ProjectSummary struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	Summary
}

// ModelSummary breaks down cost by LLM model.
type ModelSummary struct {
	Model string `json:"model"`
	Summary
}

// DailyCost holds aggregated cost for a single day.
type DailyCost struct {
	Date      string  `json:"date"`
	CostUSD   float64 `json:"cost_usd"`
	TokensIn  int64   `json:"tokens_in"`
	TokensOut int64   `json:"tokens_out"`
	RunCount  int     `json:"run_count"`
}

// ToolSummary breaks down cost and tokens by tool name.
type ToolSummary struct {
	Tool      string  `json:"tool"`
	Model     string  `json:"model"`
	CostUSD   float64 `json:"cost_usd"`
	TokensIn  int64   `json:"tokens_in"`
	TokensOut int64   `json:"tokens_out"`
	CallCount int     `json:"call_count"`
}
