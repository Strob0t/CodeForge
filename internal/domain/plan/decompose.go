package plan

import "errors"

// OrchestratorMode controls how the meta-agent operates.
type OrchestratorMode string

const (
	ModeManual   OrchestratorMode = "manual"    // User creates plans manually (current behavior)
	ModeSemiAuto OrchestratorMode = "semi_auto" // Meta-agent creates plan, user approves before start
	ModeFullAuto OrchestratorMode = "full_auto" // Meta-agent creates and starts plan automatically
)

// ValidOrchestratorMode reports whether m is a known orchestrator mode.
func ValidOrchestratorMode(m string) bool {
	switch OrchestratorMode(m) {
	case ModeManual, ModeSemiAuto, ModeFullAuto:
		return true
	}
	return false
}

// AgentStrategy describes how agents are assigned to a feature.
type AgentStrategy string

const (
	StrategySingle AgentStrategy = "single" // One agent, sequential protocol
	StrategyPair   AgentStrategy = "pair"   // Two agents, ping_pong protocol
	StrategyTeam   AgentStrategy = "team"   // Multiple agents, parallel protocol
)

// StrategyToProtocol maps an agent strategy to the default scheduling protocol.
func StrategyToProtocol(s AgentStrategy) Protocol {
	switch s {
	case StrategyPair:
		return ProtocolPingPong
	case StrategyTeam:
		return ProtocolParallel
	default:
		return ProtocolSequential
	}
}

// DecomposeRequest holds the input for LLM-based feature decomposition.
type DecomposeRequest struct {
	ProjectID string `json:"project_id"`
	Feature   string `json:"feature"`           // High-level feature description
	Context   string `json:"context,omitempty"` // Optional additional context (repo structure, TODOs, etc.)
	Model     string `json:"model,omitempty"`   // LLM model override (empty = use config default)
	AutoStart bool   `json:"auto_start"`        // Start plan immediately regardless of orchestrator mode
}

// Validate checks that the decompose request is well-formed.
func (r *DecomposeRequest) Validate() error {
	if r.ProjectID == "" {
		return errors.New("project_id is required")
	}
	if r.Feature == "" {
		return errors.New("feature description is required")
	}
	return nil
}

// DecomposeResult is the structured output parsed from the LLM response.
type DecomposeResult struct {
	PlanName    string              `json:"plan_name"`
	Description string              `json:"description"`
	Strategy    AgentStrategy       `json:"strategy"`
	Protocol    Protocol            `json:"protocol"`
	Subtasks    []SubtaskDefinition `json:"subtasks"`
}

// SubtaskDefinition describes a single subtask from feature decomposition.
type SubtaskDefinition struct {
	Title     string `json:"title"`
	Prompt    string `json:"prompt"`
	DependsOn []int  `json:"depends_on"` // indices into the Subtasks array
	AgentHint string `json:"agent_hint"` // optional: preferred backend (e.g. "aider", "openhands")
}

// PlanFeatureRequest holds the input for context-optimized feature planning.
// It extends DecomposeRequest with options for automatic team assembly.
type PlanFeatureRequest struct {
	ProjectID string `json:"project_id"`
	Feature   string `json:"feature"`           // High-level feature description
	Context   string `json:"context,omitempty"` // Optional additional context
	Model     string `json:"model,omitempty"`   // LLM model override
	AutoStart bool   `json:"auto_start"`        // Start plan immediately
	AutoTeam  bool   `json:"auto_team"`         // Auto-assemble team based on strategy
}

// Validate checks that a PlanFeatureRequest is well-formed.
func (r *PlanFeatureRequest) Validate() error {
	if r.ProjectID == "" {
		return errors.New("project_id is required")
	}
	if r.Feature == "" {
		return errors.New("feature description is required")
	}
	return nil
}

// ValidateResult checks that a DecomposeResult is structurally valid.
func (r *DecomposeResult) ValidateResult() error {
	if r.PlanName == "" {
		return errors.New("plan_name is required in decompose result")
	}
	if len(r.Subtasks) == 0 {
		return errors.New("at least one subtask is required")
	}
	for i, st := range r.Subtasks {
		if st.Title == "" {
			return errors.New("subtask title is required")
		}
		if st.Prompt == "" {
			return errors.New("subtask prompt is required")
		}
		for _, dep := range st.DependsOn {
			if dep < 0 || dep >= len(r.Subtasks) || dep == i {
				return errors.New("invalid dependency index in subtask")
			}
		}
	}
	return nil
}
