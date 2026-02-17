// Package mode defines the Mode domain entity for agent specialization.
package mode

import "fmt"

// Mode represents an agent specialization with its own tools, LLM scenario, and autonomy level.
type Mode struct {
	ID           string   `json:"id" yaml:"id"`
	Name         string   `json:"name" yaml:"name"`
	Description  string   `json:"description" yaml:"description"`
	Builtin      bool     `json:"builtin" yaml:"-"`
	Tools        []string `json:"tools" yaml:"tools"`
	LLMScenario  string   `json:"llm_scenario" yaml:"llm_scenario"`
	Autonomy     int      `json:"autonomy" yaml:"autonomy"`
	PromptPrefix string   `json:"prompt_prefix" yaml:"prompt_prefix"`
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
	return nil
}
