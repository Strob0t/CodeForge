package config

import (
	"os"
	"path/filepath"
	"strings"
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

// agent.tool_output_max_chars is 0 (the worker's default) to
// maxToolOutputMaxChars (S8-B review): a negative value made every
// runs.qualitygate.request fail the worker's validation and go to the
// DLQ, and a huge one removed the bound that keeps tool results and gate
// results below the NATS max payload.
func TestValidate_ToolOutputMaxChars(t *testing.T) {
	tests := []struct {
		value   int
		wantErr bool
	}{
		{-1, true},
		{0, false},
		{1, false},
		{10_000, false},
		{maxToolOutputMaxChars, false},
		{maxToolOutputMaxChars + 1, true},
		{1 << 30, true},
	}
	for _, tt := range tests {
		cfg := Defaults()
		cfg.Auth.JWTSecret = strongTestSecret
		cfg.Agent.ToolOutputMaxChars = tt.value
		err := validate(&cfg)
		if tt.wantErr && (err == nil || !strings.Contains(err.Error(), "agent.tool_output_max_chars")) {
			t.Errorf("value %d: expected an agent.tool_output_max_chars error, got %v", tt.value, err)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("value %d: unexpected error %v", tt.value, err)
		}
	}
}

// The bound keeps a quality gate result, which carries two outputs, below
// the NATS default max payload of 1 MiB even when every character needs a
// 6-byte JSON escape; the server config does not raise that limit.
func TestMaxToolOutputMaxChars_FitsTheNATSMaxPayload(t *testing.T) {
	const natsDefaultMaxPayload = 1 << 20
	if 2*6*maxToolOutputMaxChars >= natsDefaultMaxPayload {
		t.Fatalf("maxToolOutputMaxChars %d: two outputs may exceed %d bytes", maxToolOutputMaxChars, natsDefaultMaxPayload)
	}
	conf, err := os.ReadFile(filepath.Join("..", "..", "configs", "nats", "nats-server.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(conf), "max_payload") {
		t.Fatal("configs/nats/nats-server.conf sets max_payload: check maxToolOutputMaxChars against it")
	}
}
