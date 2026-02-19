package ws

import (
	"context"
	"encoding/json"
	"log/slog"
)

// Event type constants for WebSocket messages.
const (
	EventTaskStatus  = "task.status"
	EventTaskOutput  = "task.output"
	EventAgentStatus = "agent.status"

	// Run protocol events (Phase 4B)
	EventRunStatus      = "run.status"
	EventToolCallStatus = "run.toolcall"

	// Phase 4C events
	EventQualityGate = "run.qualitygate"
	EventDelivery    = "run.delivery"

	// Phase 5A: orchestration plan events
	EventPlanStatus     = "plan.status"
	EventPlanStepStatus = "plan.step.status"

	// Phase 5E: team + shared context events
	EventTeamStatus          = "team.status"
	EventSharedContextUpdate = "shared.updated"

	// Phase 6A: repo map events
	EventRepoMapStatus = "repomap.status"

	// Phase 6B: retrieval events
	EventRetrievalStatus = "retrieval.status"

	// Phase 6D: graph events
	EventGraphStatus = "graph.status"

	// Phase 7: cost transparency events
	EventBudgetAlert = "run.budget_alert"

	// Phase 8: roadmap events
	EventRoadmapStatus = "roadmap.status"

	// Phase 8B: VCS webhook events
	EventVCSPush        = "vcs.push"
	EventVCSPullRequest = "vcs.pull_request"
)

// TaskStatusEvent is broadcast when a task's status changes.
type TaskStatusEvent struct {
	TaskID    string `json:"task_id"`
	ProjectID string `json:"project_id"`
	Status    string `json:"status"`
	AgentID   string `json:"agent_id,omitempty"`
}

// TaskOutputEvent is broadcast when a task produces streaming output.
type TaskOutputEvent struct {
	TaskID string `json:"task_id"`
	Line   string `json:"line"`
	Stream string `json:"stream"` // "stdout" or "stderr"
}

// AgentStatusEvent is broadcast when an agent's status changes.
type AgentStatusEvent struct {
	AgentID   string `json:"agent_id"`
	ProjectID string `json:"project_id"`
	Status    string `json:"status"`
}

// RunStatusEvent is broadcast when a run's status or metrics change.
type RunStatusEvent struct {
	RunID     string  `json:"run_id"`
	TaskID    string  `json:"task_id"`
	ProjectID string  `json:"project_id"`
	Status    string  `json:"status"`
	StepCount int     `json:"step_count"`
	CostUSD   float64 `json:"cost_usd,omitempty"`
	TokensIn  int64   `json:"tokens_in,omitempty"`
	TokensOut int64   `json:"tokens_out,omitempty"`
	Model     string  `json:"model,omitempty"`
}

// ToolCallStatusEvent is broadcast for tool call lifecycle events.
type ToolCallStatusEvent struct {
	RunID    string `json:"run_id"`
	CallID   string `json:"call_id"`
	Tool     string `json:"tool"`
	Decision string `json:"decision,omitempty"`
	Phase    string `json:"phase"` // "requested", "approved", "denied", "result"
}

// QualityGateEvent is broadcast when a quality gate starts, passes, or fails.
type QualityGateEvent struct {
	RunID       string `json:"run_id"`
	TaskID      string `json:"task_id"`
	ProjectID   string `json:"project_id"`
	Status      string `json:"status"` // "started", "passed", "failed"
	TestsPassed *bool  `json:"tests_passed,omitempty"`
	LintPassed  *bool  `json:"lint_passed,omitempty"`
	Error       string `json:"error,omitempty"`
}

// DeliveryEvent is broadcast when output delivery starts, completes, or fails.
type DeliveryEvent struct {
	RunID      string `json:"run_id"`
	TaskID     string `json:"task_id"`
	ProjectID  string `json:"project_id"`
	Status     string `json:"status"` // "started", "completed", "failed"
	Mode       string `json:"mode"`
	PatchPath  string `json:"patch_path,omitempty"`
	CommitHash string `json:"commit_hash,omitempty"`
	BranchName string `json:"branch_name,omitempty"`
	PRURL      string `json:"pr_url,omitempty"`
	Error      string `json:"error,omitempty"`
}

// PlanStatusEvent is broadcast when an execution plan's status changes.
type PlanStatusEvent struct {
	PlanID    string `json:"plan_id"`
	ProjectID string `json:"project_id"`
	Status    string `json:"status"`
}

// PlanStepStatusEvent is broadcast when a plan step's status changes.
type PlanStepStatusEvent struct {
	PlanID    string `json:"plan_id"`
	StepID    string `json:"step_id"`
	ProjectID string `json:"project_id"`
	Status    string `json:"status"`
	RunID     string `json:"run_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// TeamStatusEvent is broadcast when a team's status changes.
type TeamStatusEvent struct {
	TeamID    string `json:"team_id"`
	ProjectID string `json:"project_id"`
	Status    string `json:"status"`
	Name      string `json:"name"`
}

// SharedContextUpdateEvent is broadcast when a shared context item is added or updated.
type SharedContextUpdateEvent struct {
	TeamID  string `json:"team_id"`
	Key     string `json:"key"`
	Author  string `json:"author"`
	Version int    `json:"version"`
}

// RepoMapStatusEvent is broadcast when a repo map's generation status changes.
type RepoMapStatusEvent struct {
	ProjectID   string `json:"project_id"`
	Status      string `json:"status"` // "generating", "ready", "failed"
	TokenCount  int    `json:"token_count,omitempty"`
	FileCount   int    `json:"file_count,omitempty"`
	SymbolCount int    `json:"symbol_count,omitempty"`
	Error       string `json:"error,omitempty"`
}

// GraphStatusEvent is broadcast when a graph's build status changes.
type GraphStatusEvent struct {
	ProjectID string   `json:"project_id"`
	Status    string   `json:"status"` // "building", "ready", "error"
	NodeCount int      `json:"node_count,omitempty"`
	EdgeCount int      `json:"edge_count,omitempty"`
	Languages []string `json:"languages,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// RetrievalStatusEvent is broadcast when a retrieval index's status changes.
type RetrievalStatusEvent struct {
	ProjectID      string `json:"project_id"`
	Status         string `json:"status"` // "building", "ready", "error"
	FileCount      int    `json:"file_count,omitempty"`
	ChunkCount     int    `json:"chunk_count,omitempty"`
	EmbeddingModel string `json:"embedding_model,omitempty"`
	Error          string `json:"error,omitempty"`
}

// BudgetAlertEvent is broadcast when a run's cost reaches a budget threshold (e.g. 80%, 90%).
type BudgetAlertEvent struct {
	RunID      string  `json:"run_id"`
	TaskID     string  `json:"task_id"`
	ProjectID  string  `json:"project_id"`
	CostUSD    float64 `json:"cost_usd"`
	MaxCost    float64 `json:"max_cost"`
	Percentage float64 `json:"percentage"`
}

// RoadmapStatusEvent is broadcast when a roadmap's status changes.
type RoadmapStatusEvent struct {
	RoadmapID string `json:"roadmap_id"`
	ProjectID string `json:"project_id"`
	Status    string `json:"status"`
	Title     string `json:"title"`
}

// BroadcastEvent is a convenience method that marshals a typed event and broadcasts it.
func (h *Hub) BroadcastEvent(ctx context.Context, eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("marshal ws event payload", "type", eventType, "error", err)
		return
	}

	h.Broadcast(ctx, Message{
		Type:    eventType,
		Payload: json.RawMessage(data),
	})
}
