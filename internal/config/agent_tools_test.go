package config

import (
	"os"
	"path/filepath"
	"testing"
)

// agent.builtin_tools was never read (KI-38) and is gone; mode tool lists
// select the tools. A config file that still sets it keeps loading, and
// agent.tool_output_max_chars is read (the worker gets it per run).
func TestLoadYAML_AgentToolSettings(t *testing.T) {
	yamlPath := filepath.Join(t.TempDir(), "codeforge.yaml")
	content := `
agent:
  builtin_tools: [read_file, bash]
  tool_output_max_chars: 4321
`
	if err := os.WriteFile(yamlPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	if err := loadYAML(&cfg, yamlPath); err != nil {
		t.Fatalf("a config with the removed builtin_tools must still load: %v", err)
	}
	if cfg.Agent.ToolOutputMaxChars != 4321 {
		t.Fatalf("tool_output_max_chars = %d, want 4321", cfg.Agent.ToolOutputMaxChars)
	}
}
