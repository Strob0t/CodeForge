// Package orchestration provides domain models for agent-to-agent handoffs
// and multi-agent coordination.
package orchestration

import "errors"

// HandoffMessage represents an explicit agent-to-agent handoff with context.
type HandoffMessage struct {
	SourceAgentID string            `json:"source_agent_id"`
	TargetAgentID string            `json:"target_agent_id"`
	TargetModeID  string            `json:"target_mode_id,omitempty"`
	Context       string            `json:"context"`
	Artifacts     []string          `json:"artifacts,omitempty"`
	PlanID        string            `json:"plan_id,omitempty"`
	StepID        string            `json:"step_id,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// Validate checks that a HandoffMessage has all required fields.
func (m *HandoffMessage) Validate() error {
	if m.SourceAgentID == "" {
		return errors.New("source_agent_id is required")
	}
	if m.TargetAgentID == "" {
		return errors.New("target_agent_id is required")
	}
	if m.Context == "" {
		return errors.New("context is required")
	}
	return nil
}
