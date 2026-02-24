package policy

import (
	"strings"
	"testing"
)

func TestPolicyProfileValidateValid(t *testing.T) {
	p := PolicyProfile{
		Name: "test",
		Mode: ModeDefault,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "Read"}, Decision: DecisionAllow},
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyProfileValidateErrors(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*PolicyProfile)
		errStr string
	}{
		{
			name:   "missing name",
			modify: func(p *PolicyProfile) { p.Name = "" },
			errStr: "name is required",
		},
		{
			name:   "invalid mode",
			modify: func(p *PolicyProfile) { p.Mode = "invalid" },
			errStr: "invalid mode",
		},
		{
			name: "bad rule - missing tool",
			modify: func(p *PolicyProfile) {
				p.Rules = []PermissionRule{{Decision: DecisionAllow}}
			},
			errStr: "tool is required",
		},
		{
			name: "bad rule - invalid decision",
			modify: func(p *PolicyProfile) {
				p.Rules = []PermissionRule{{Specifier: ToolSpecifier{Tool: "Read"}, Decision: "maybe"}}
			},
			errStr: "invalid decision",
		},
		{
			name:   "negative max_steps",
			modify: func(p *PolicyProfile) { p.Termination.MaxSteps = -1 },
			errStr: "max_steps must be >= 0",
		},
		{
			name:   "negative timeout",
			modify: func(p *PolicyProfile) { p.Termination.TimeoutSeconds = -5 },
			errStr: "timeout_seconds must be >= 0",
		},
		{
			name:   "negative max_cost",
			modify: func(p *PolicyProfile) { p.Termination.MaxCost = -0.5 },
			errStr: "max_cost must be >= 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PolicyProfile{Name: "test", Mode: ModeDefault}
			tt.modify(&p)
			err := p.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errStr) {
				t.Errorf("expected error containing %q, got %q", tt.errStr, err.Error())
			}
		})
	}
}

func TestPermissionRuleValidateValid(t *testing.T) {
	r := PermissionRule{
		Specifier: ToolSpecifier{Tool: "Bash", SubPattern: "git:*"},
		Decision:  DecisionDeny,
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsValidMode(t *testing.T) {
	valid := []PermissionMode{ModeDefault, ModeAcceptEdits, ModePlan, ModeDelegate}
	for _, m := range valid {
		if !isValidMode(m) {
			t.Errorf("expected %q to be valid", m)
		}
	}
	if isValidMode("unknown") {
		t.Error("expected 'unknown' to be invalid")
	}
}

func TestIsValidDecision(t *testing.T) {
	valid := []Decision{DecisionAllow, DecisionDeny, DecisionAsk}
	for _, d := range valid {
		if !isValidDecision(d) {
			t.Errorf("expected %q to be valid", d)
		}
	}
	if isValidDecision("maybe") {
		t.Error("expected 'maybe' to be invalid")
	}
}

// --- Evaluation tests ---

func TestEvaluateFirstMatchWins(t *testing.T) {
	p := PolicyProfile{
		Name: "test",
		Mode: ModeDefault,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "Read"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Bash"}, Decision: DecisionDeny},
		},
	}

	result := p.Evaluate(ToolCall{Tool: "Read"})
	if result.Decision != DecisionAllow {
		t.Errorf("expected allow, got %q", result.Decision)
	}
	if result.RuleIndex != 0 {
		t.Errorf("expected rule index 0, got %d", result.RuleIndex)
	}

	result = p.Evaluate(ToolCall{Tool: "Bash"})
	if result.Decision != DecisionDeny {
		t.Errorf("expected deny, got %q", result.Decision)
	}
	if result.RuleIndex != 1 {
		t.Errorf("expected rule index 1, got %d", result.RuleIndex)
	}
}

func TestEvaluateNoMatchDenyByDefault(t *testing.T) {
	p := PolicyProfile{
		Name: "test",
		Mode: ModeDefault,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "Read"}, Decision: DecisionAllow},
		},
	}

	result := p.Evaluate(ToolCall{Tool: "Write"})
	if result.Decision != DecisionDeny {
		t.Errorf("expected deny by default, got %q", result.Decision)
	}
	if result.RuleIndex != -1 {
		t.Errorf("expected rule index -1, got %d", result.RuleIndex)
	}
}

func TestEvaluateWildcardMatchesAll(t *testing.T) {
	p := PolicyProfile{
		Name: "test",
		Mode: ModeDefault,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "*"}, Decision: DecisionAllow},
		},
	}

	for _, tool := range []string{"Read", "Bash", "Edit", "mcp:filesystem:read_file"} {
		result := p.Evaluate(ToolCall{Tool: tool})
		if result.Decision != DecisionAllow {
			t.Errorf("wildcard should match %q, got decision %q", tool, result.Decision)
		}
	}
}

// --- MCP tool specifier tests ---

func TestMatchToolMCPWildcard(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		tool    string
		want    bool
	}{
		{
			name:    "mcp:* matches mcp:filesystem:read_file",
			pattern: "mcp:*",
			tool:    "mcp:filesystem:read_file",
			want:    true,
		},
		{
			name:    "mcp:* matches mcp:database:query",
			pattern: "mcp:*",
			tool:    "mcp:database:query",
			want:    true,
		},
		{
			name:    "mcp:filesystem:* matches mcp:filesystem:read_file",
			pattern: "mcp:filesystem:*",
			tool:    "mcp:filesystem:read_file",
			want:    true,
		},
		{
			name:    "mcp:filesystem:* matches mcp:filesystem:write_file",
			pattern: "mcp:filesystem:*",
			tool:    "mcp:filesystem:write_file",
			want:    true,
		},
		{
			name:    "mcp:filesystem:* does NOT match mcp:database:query",
			pattern: "mcp:filesystem:*",
			tool:    "mcp:database:query",
			want:    false,
		},
		{
			name:    "exact mcp:filesystem:read_file matches itself",
			pattern: "mcp:filesystem:read_file",
			tool:    "mcp:filesystem:read_file",
			want:    true,
		},
		{
			name:    "exact mcp:filesystem:read_file does NOT match write_file",
			pattern: "mcp:filesystem:read_file",
			tool:    "mcp:filesystem:write_file",
			want:    false,
		},
		{
			name:    "global wildcard * matches mcp: prefixed tool",
			pattern: "*",
			tool:    "mcp:filesystem:read_file",
			want:    true,
		},
		{
			name:    "non-mcp pattern does not match mcp tool",
			pattern: "Read",
			tool:    "mcp:filesystem:read_file",
			want:    false,
		},
		{
			name:    "mcp pattern does not match non-mcp tool",
			pattern: "mcp:*",
			tool:    "Read",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchTool(tt.pattern, tt.tool)
			if got != tt.want {
				t.Errorf("matchTool(%q, %q) = %v, want %v", tt.pattern, tt.tool, got, tt.want)
			}
		})
	}
}

func TestEvaluateMCPToolsFirstMatchWins(t *testing.T) {
	p := PolicyProfile{
		Name: "mcp-mixed-policy",
		Mode: ModeDefault,
		Rules: []PermissionRule{
			// Allow all filesystem MCP tools
			{Specifier: ToolSpecifier{Tool: "mcp:filesystem:*"}, Decision: DecisionAllow},
			// Deny all database MCP tools
			{Specifier: ToolSpecifier{Tool: "mcp:database:*"}, Decision: DecisionDeny},
			// Ask for any other MCP tool
			{Specifier: ToolSpecifier{Tool: "mcp:*"}, Decision: DecisionAsk},
			// Allow standard tools
			{Specifier: ToolSpecifier{Tool: "Read"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Edit"}, Decision: DecisionAllow},
		},
	}

	tests := []struct {
		name     string
		call     ToolCall
		decision Decision
		ruleIdx  int
	}{
		{
			name:     "mcp filesystem read allowed by rule 0",
			call:     ToolCall{Tool: "mcp:filesystem:read_file"},
			decision: DecisionAllow,
			ruleIdx:  0,
		},
		{
			name:     "mcp filesystem write allowed by rule 0",
			call:     ToolCall{Tool: "mcp:filesystem:write_file"},
			decision: DecisionAllow,
			ruleIdx:  0,
		},
		{
			name:     "mcp database query denied by rule 1",
			call:     ToolCall{Tool: "mcp:database:query"},
			decision: DecisionDeny,
			ruleIdx:  1,
		},
		{
			name:     "mcp git tool falls through to mcp:* ask rule",
			call:     ToolCall{Tool: "mcp:git:status"},
			decision: DecisionAsk,
			ruleIdx:  2,
		},
		{
			name:     "standard Read tool allowed by rule 3",
			call:     ToolCall{Tool: "Read"},
			decision: DecisionAllow,
			ruleIdx:  3,
		},
		{
			name:     "unmatched tool denied by default",
			call:     ToolCall{Tool: "Bash"},
			decision: DecisionDeny,
			ruleIdx:  -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.Evaluate(tt.call)
			if result.Decision != tt.decision {
				t.Errorf("expected decision %q, got %q", tt.decision, result.Decision)
			}
			if result.RuleIndex != tt.ruleIdx {
				t.Errorf("expected rule index %d, got %d", tt.ruleIdx, result.RuleIndex)
			}
		})
	}
}

func TestEvaluateMCPDenyByDefaultInRestrictiveProfile(t *testing.T) {
	// A restrictive profile with only specific tools allowed.
	// MCP tools with no matching rule get denied by default.
	p := PolicyProfile{
		Name: "restrictive",
		Mode: ModePlan,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "Read"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Glob"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Grep"}, Decision: DecisionAllow},
		},
	}

	mcpTools := []string{
		"mcp:filesystem:read_file",
		"mcp:database:query",
		"mcp:git:status",
	}

	for _, tool := range mcpTools {
		t.Run(tool, func(t *testing.T) {
			result := p.Evaluate(ToolCall{Tool: tool})
			if result.Decision != DecisionDeny {
				t.Errorf("mcp tool %q should be denied in restrictive profile, got %q", tool, result.Decision)
			}
			if result.RuleIndex != -1 {
				t.Errorf("expected no rule match (index -1), got %d", result.RuleIndex)
			}
		})
	}
}

func TestEvaluateMCPValidation(t *testing.T) {
	// MCP tool specifiers should pass validation just like regular tools.
	tests := []struct {
		name string
		rule PermissionRule
	}{
		{
			name: "mcp wildcard rule validates",
			rule: PermissionRule{
				Specifier: ToolSpecifier{Tool: "mcp:*"},
				Decision:  DecisionAllow,
			},
		},
		{
			name: "mcp server wildcard rule validates",
			rule: PermissionRule{
				Specifier: ToolSpecifier{Tool: "mcp:filesystem:*"},
				Decision:  DecisionDeny,
			},
		},
		{
			name: "mcp exact tool rule validates",
			rule: PermissionRule{
				Specifier: ToolSpecifier{Tool: "mcp:filesystem:read_file"},
				Decision:  DecisionAsk,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.rule.Validate(); err != nil {
				t.Errorf("expected valid MCP rule, got error: %v", err)
			}
		})
	}
}

func TestEvaluateProfileNameInResult(t *testing.T) {
	p := PolicyProfile{
		Name:  "test-profile",
		Mode:  ModeDefault,
		Scope: ScopeProject,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "mcp:*"}, Decision: DecisionAllow},
		},
	}

	result := p.Evaluate(ToolCall{Tool: "mcp:filesystem:read_file"})
	if result.Profile != "test-profile" {
		t.Errorf("expected profile name %q, got %q", "test-profile", result.Profile)
	}
	if result.Scope != ScopeProject {
		t.Errorf("expected scope %q, got %q", ScopeProject, result.Scope)
	}
}
