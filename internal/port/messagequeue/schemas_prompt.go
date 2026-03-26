package messagequeue

import "encoding/json"

// --- Prompt Evolution payloads (Phase 33) ---

// PromptEvolutionTacticalFix carries a single failure-specific fix.
type PromptEvolutionTacticalFix struct {
	TaskID             string  `json:"task_id"`
	FailureDescription string  `json:"failure_description"`
	RootCause          string  `json:"root_cause"`
	ProposedAddition   string  `json:"proposed_addition"`
	Confidence         float64 `json:"confidence"`
}

// PromptEvolutionReflectPayload is published to request failure reflection.
type PromptEvolutionReflectPayload struct {
	TenantID      string                       `json:"tenant_id"`
	ModeID        string                       `json:"mode_id"`
	ModelFamily   string                       `json:"model_family"`
	CurrentPrompt string                       `json:"current_prompt"`
	Failures      []map[string]json.RawMessage `json:"failures"`
}

// PromptEvolutionReflectCompletePayload carries reflection results back.
type PromptEvolutionReflectCompletePayload struct {
	TenantID            string                       `json:"tenant_id"`
	ModeID              string                       `json:"mode_id"`
	ModelFamily         string                       `json:"model_family"`
	TacticalFixes       []PromptEvolutionTacticalFix `json:"tactical_fixes"`
	StrategicPrinciples []string                     `json:"strategic_principles"`
	Error               string                       `json:"error,omitempty"`
}

// PromptEvolutionMutateCompletePayload carries mutation results back.
type PromptEvolutionMutateCompletePayload struct {
	TenantID         string `json:"tenant_id"`
	ModeID           string `json:"mode_id"`
	ModelFamily      string `json:"model_family"`
	VariantContent   string `json:"variant_content"`
	Version          int    `json:"version"`
	ParentID         string `json:"parent_id,omitempty"`
	MutationSource   string `json:"mutation_source"`
	ValidationPassed bool   `json:"validation_passed"`
	Error            string `json:"error,omitempty"`
}

// PromptEvolutionEventPayload is published for promoted/reverted events.
type PromptEvolutionEventPayload struct {
	TenantID   string `json:"tenant_id"`
	ModeID     string `json:"mode_id"`
	VariantID  string `json:"variant_id,omitempty"`
	Action     string `json:"action"` // "promoted" | "reverted" | "retired"
	OldVersion int    `json:"old_version,omitempty"`
	NewVersion int    `json:"new_version,omitempty"`
}

// --- A2A payloads (Phase 27) ---

// A2ATaskCreatedPayload is published when an inbound A2A task is received.
type A2ATaskCreatedPayload struct {
	TaskID   string `json:"task_id"`
	TenantID string `json:"tenant_id"`
	SkillID  string `json:"skill_id"`
	Prompt   string `json:"prompt"`
}

// A2ATaskCompletePayload is published when an A2A task completes.
type A2ATaskCompletePayload struct {
	TaskID   string `json:"task_id"`
	TenantID string `json:"tenant_id,omitempty"`
	State    string `json:"state"`
	Error    string `json:"error,omitempty"`
}
