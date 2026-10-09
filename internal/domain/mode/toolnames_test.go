package mode

import (
	"slices"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/policy"
)

// Mode tool lists use the canonical policy tool names (KI-10), so that the
// policy evaluation can enforce them.
func TestBuiltinToolNamesAreCanonical(t *testing.T) {
	if !slices.Equal(BuiltinToolNames, policy.BuiltinTools()) {
		t.Fatalf("BuiltinToolNames = %v, want policy.BuiltinTools() %v", BuiltinToolNames, policy.BuiltinTools())
	}
}

func TestBuiltinModes_ToolNamesCanonical(t *testing.T) {
	for _, m := range BuiltinModes() {
		for _, list := range [][]string{m.Tools, m.DeniedTools} {
			for _, tool := range list {
				if policy.CanonicalTool(tool) != tool {
					t.Errorf("mode %q: tool %q is not canonical (want %q)", m.ID, tool, policy.CanonicalTool(tool))
				}
			}
		}
		for _, tool := range m.Tools {
			if !policy.IsBuiltinTool(tool) && tool != "propose_goal" {
				t.Errorf("mode %q: unknown tool %q in Tools", m.ID, tool)
			}
		}
	}
}

// Modes that may search files by glob may also list directories.
func TestBuiltinModes_GlobImpliesListDir(t *testing.T) {
	for _, m := range BuiltinModes() {
		if slices.Contains(m.Tools, policy.ToolGlob) && !slices.Contains(m.Tools, policy.ToolListDir) {
			t.Errorf("mode %q allows Glob but not ListDir", m.ID)
		}
	}
}

func TestValidate_DeniedToolsOverlapCanonical(t *testing.T) {
	m := Mode{
		ID:          "test",
		Name:        "Test",
		Autonomy:    2,
		Tools:       []string{"read_file", "write_file"},
		DeniedTools: []string{"Write"},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for write_file in Tools and Write in DeniedTools")
	}
}
