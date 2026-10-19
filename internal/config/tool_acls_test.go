package config

import "testing"

// KI-96: workspace.tool_acls selects per-tenant tool identities. The default
// keeps development unchanged; an unknown value fails closed.
func TestWorkspaceToolACLs(t *testing.T) {
	cfg := Defaults()
	if required, known := cfg.Workspace.ToolACLsRequired(); required || !known {
		t.Fatalf("default: required %v, known %v; want off", required, known)
	}

	t.Setenv("CODEFORGE_WORKSPACE_TOOL_ACLS", "required")
	cfg = Defaults()
	mustLoadEnv(t, &cfg)
	if cfg.Workspace.ToolACLs != "required" {
		t.Fatalf("from env: %q", cfg.Workspace.ToolACLs)
	}

	tests := []struct {
		raw             string
		required, known bool
	}{
		{"", false, true},
		{"off", false, true},
		{" OFF ", false, true},
		{"required", true, true},
		{"Required\n", true, true},
		{"on", true, false},
		{"false", true, false},
		{"0", true, false},
	}
	for _, tt := range tests {
		w := Workspace{ToolACLs: tt.raw}
		if required, known := w.ToolACLsRequired(); required != tt.required || known != tt.known {
			t.Errorf("%q: required %v known %v, want %v %v", tt.raw, required, known, tt.required, tt.known)
		}
	}
}
