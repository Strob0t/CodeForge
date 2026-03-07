// Package autoagent contains the domain model for the auto-agent feature
// that iterates over pending roadmap features and processes them via the
// agentic conversation loop.
package autoagent

import "time"

// Status represents the lifecycle state of an auto-agent run.
type Status string

const (
	StatusIdle     Status = "idle"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
	StatusFailed   Status = "failed"
)

// AutoAgent tracks the state of an auto-agent run for a project.
type AutoAgent struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	ProjectID        string    `json:"project_id"`
	Status           Status    `json:"status"`
	CurrentFeatureID string    `json:"current_feature_id,omitempty"`
	ConversationID   string    `json:"conversation_id,omitempty"`
	FeaturesTotal    int       `json:"features_total"`
	FeaturesComplete int       `json:"features_complete"`
	FeaturesFailed   int       `json:"features_failed"`
	TotalCostUSD     float64   `json:"total_cost_usd"`
	Error            string    `json:"error,omitempty"`
	StartedAt        time.Time `json:"started_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Safety constants for auto-agent execution.
const (
	MaxIterationsPerFeature = 50
	FeatureTimeoutMinutes   = 30
	DefaultMaxBudgetUSD     = 10.0
)
