// Package mode defines the Mode domain entity for agent specialization.
package mode

import (
	"fmt"
	"slices"
)

// ValidScenarios lists the allowed LLM scenario values for mode configuration.
var ValidScenarios = []string{"default", "background", "think", "longContext", "review", "plan"}

// Mode represents an agent specialization with its own tools, LLM scenario, and autonomy level.
type Mode struct {
	ID               string   `json:"id" yaml:"id"`
	Name             string   `json:"name" yaml:"name"`
	Description      string   `json:"description" yaml:"description"`
	Builtin          bool     `json:"builtin" yaml:"-"`
	Tools            []string `json:"tools" yaml:"tools"`
	DeniedTools      []string `json:"denied_tools" yaml:"denied_tools"`
	DeniedActions    []string `json:"denied_actions" yaml:"denied_actions"`
	RequiredArtifact string   `json:"required_artifact" yaml:"required_artifact"`
	LLMScenario      string   `json:"llm_scenario" yaml:"llm_scenario"`
	Autonomy         int      `json:"autonomy" yaml:"autonomy"`
	PromptPrefix     string   `json:"prompt_prefix" yaml:"prompt_prefix"`
	OutputSchema     string   `json:"output_schema,omitempty" yaml:"output_schema"`
}

// Validate checks that a Mode has all required fields and valid values.
func (m *Mode) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("id is required")
	}
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.Autonomy < 1 || m.Autonomy > 5 {
		return fmt.Errorf("autonomy must be between 1 and 5, got %d", m.Autonomy)
	}
	if m.LLMScenario != "" && !slices.Contains(ValidScenarios, m.LLMScenario) {
		return fmt.Errorf("invalid llm_scenario %q: must be one of %v", m.LLMScenario, ValidScenarios)
	}
	// DeniedTools must not overlap with Tools.
	if len(m.DeniedTools) > 0 && len(m.Tools) > 0 {
		allowed := make(map[string]bool, len(m.Tools))
		for _, t := range m.Tools {
			allowed[t] = true
		}
		for _, d := range m.DeniedTools {
			if allowed[d] {
				return fmt.Errorf("tool %q appears in both tools and denied_tools", d)
			}
		}
	}
	return nil
}
