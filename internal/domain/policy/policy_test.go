package policy

import (
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/trust"
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
			name:   "max_steps exceeds upper bound",
			modify: func(p *PolicyProfile) { p.Termination.MaxSteps = MaxStepsLimit + 1 },
			errStr: "max_steps must not exceed",
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

func TestValidatePolicy_MaxStepsUpperBound(t *testing.T) {
	tests := []struct {
		name    string
		steps   int
		wantErr bool
	}{
		{name: "zero (unlimited)", steps: 0, wantErr: false},
		{name: "normal value", steps: 500, wantErr: false},
		{name: "at limit", steps: MaxStepsLimit, wantErr: false},
		{name: "over limit", steps: MaxStepsLimit + 1, wantErr: true},
		{name: "way over limit", steps: 100_000, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PolicyProfile{
				Name: "test",
				Mode: ModeDefault,
				Termination: TerminationCondition{
					MaxSteps: tt.steps,
				},
			}
			err := p.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for MaxSteps=%d, got nil", tt.steps)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for MaxSteps=%d: %v", tt.steps, err)
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

// KI-204: a misspelled trust_minimum used to accept every trust level; it is
// now rejected when the profile is validated.
func TestPermissionRuleValidateTrustMinimum(t *testing.T) {
	tests := []struct {
		name    string
		min     trust.Level
		wantErr bool
	}{
		{"unset", "", false},
		{"full", trust.LevelFull, false},
		{"verified", trust.LevelVerified, false},
		{"partial", trust.LevelPartial, false},
		{"untrusted", trust.LevelUntrusted, false},
		{"wrong case", "Verified", true},
		{"unknown", "high", true},
		{"whitespace", " ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := PermissionRule{
				Specifier:    ToolSpecifier{Tool: "Read"},
				Decision:     DecisionAllow,
				TrustMinimum: tt.min,
			}
			err := r.Validate()
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "trust_minimum") {
					t.Fatalf("Validate() = %v, want an error naming trust_minimum", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
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

func TestPolicyDomain_EvaluateFirstMatchWins(t *testing.T) {
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

// When no rule matches, the profile's permission mode decides (ADR-007).
func TestEvaluateNoMatchUsesModeDefault(t *testing.T) {
	tests := []struct {
		mode PermissionMode
		want Decision
	}{
		{ModePlan, DecisionDeny},
		{ModeDefault, DecisionAsk},
		{ModeAcceptEdits, DecisionAllow},
		{ModeDelegate, DecisionAllow},
	}
	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			p := PolicyProfile{
				Name: "test",
				Mode: tt.mode,
				Rules: []PermissionRule{
					{Specifier: ToolSpecifier{Tool: "Read"}, Decision: DecisionAllow},
				},
			}
			result := p.Evaluate(ToolCall{Tool: "Write"})
			if result.Decision != tt.want {
				t.Errorf("expected %q, got %q", tt.want, result.Decision)
			}
			if result.RuleIndex != -1 {
				t.Errorf("expected rule index -1, got %d", result.RuleIndex)
			}
		})
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
			name:     "unmatched tool falls to the mode default (ask)",
			call:     ToolCall{Tool: "Bash"},
			decision: DecisionAsk,
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

// --- Trust-aware evaluation tests (Phase 23A) ---

func TestEvaluateTrustMinimumDenied(t *testing.T) {
	p := PolicyProfile{
		Name: "trust-test",
		Mode: ModeDefault,
		Rules: []PermissionRule{
			{
				Specifier:    ToolSpecifier{Tool: "Bash"},
				Decision:     DecisionAllow,
				TrustMinimum: trust.LevelVerified,
			},
		},
	}

	// The allow rule does not apply (untrusted < verified), so the mode
	// default of ModeDefault (ask) decides instead of the rule.
	ann := &trust.Annotation{TrustLevel: trust.LevelUntrusted, Origin: "a2a"}
	result := p.Evaluate(ToolCall{Tool: "Bash"}, WithTrust(ann))
	if result.Decision != DecisionAsk {
		t.Errorf("expected ask (untrusted < verified, mode default), got %q", result.Decision)
	}
	if result.RuleIndex != -1 {
		t.Errorf("expected no rule match, got index %d", result.RuleIndex)
	}

	p.Mode = ModePlan
	if result := p.Evaluate(ToolCall{Tool: "Bash"}, WithTrust(ann)); result.Decision != DecisionDeny {
		t.Errorf("expected deny (untrusted < verified, plan mode default), got %q", result.Decision)
	}
}

func TestEvaluateTrustMinimumAllowed(t *testing.T) {
	p := PolicyProfile{
		Name: "trust-test",
		Mode: ModeDefault,
		Rules: []PermissionRule{
			{
				Specifier:    ToolSpecifier{Tool: "Bash"},
				Decision:     DecisionAllow,
				TrustMinimum: trust.LevelVerified,
			},
		},
	}

	ann := &trust.Annotation{TrustLevel: trust.LevelVerified, Origin: "a2a"}
	result := p.Evaluate(ToolCall{Tool: "Bash"}, WithTrust(ann))
	if result.Decision != DecisionAllow {
		t.Errorf("expected allow (verified >= verified), got %q", result.Decision)
	}
	if result.RuleIndex != 0 {
		t.Errorf("expected rule index 0, got %d", result.RuleIndex)
	}
}

// An allow rule with a trust minimum must not allow a call that carries no
// trust annotation (fail closed); previously a missing annotation skipped
// the check and the rule allowed untrusted calls.
func TestEvaluateNoTrustAnnotationFailsClosed(t *testing.T) {
	p := PolicyProfile{
		Name: "trust-test",
		Mode: ModeDefault,
		Rules: []PermissionRule{
			{
				Specifier:    ToolSpecifier{Tool: "Bash"},
				Decision:     DecisionAllow,
				TrustMinimum: trust.LevelVerified,
			},
		},
	}

	result := p.Evaluate(ToolCall{Tool: "Bash"})
	if result.Decision == DecisionAllow {
		t.Errorf("expected no allow without a trust annotation, got %q", result.Decision)
	}
	if result.RuleIndex != -1 {
		t.Errorf("expected no rule match, got index %d", result.RuleIndex)
	}
	result = p.Evaluate(ToolCall{Tool: "Bash"}, WithTrust(nil))
	if result.Decision == DecisionAllow {
		t.Errorf("expected no allow with a nil trust annotation, got %q", result.Decision)
	}
}

// Trust minimums only restrict what an allow rule grants: deny and ask
// rules apply to every caller.
func TestEvaluateTrustMinimumNeverSkipsRestrictiveRules(t *testing.T) {
	p := PolicyProfile{
		Name: "trust-test",
		Mode: ModeAcceptEdits,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "Bash"}, Decision: DecisionDeny, TrustMinimum: trust.LevelVerified},
			{Specifier: ToolSpecifier{Tool: "Edit"}, Decision: DecisionAsk, TrustMinimum: trust.LevelFull},
		},
	}
	untrusted := &trust.Annotation{TrustLevel: trust.LevelUntrusted}
	if res := p.Evaluate(ToolCall{Tool: "Bash"}, WithTrust(untrusted)); res.Decision != DecisionDeny {
		t.Errorf("deny rule skipped for an untrusted caller: %s", res.Decision)
	}
	if res := p.Evaluate(ToolCall{Tool: "Edit", Path: "a"}); res.Decision != DecisionAsk {
		t.Errorf("ask rule skipped without annotation: %s", res.Decision)
	}
}

// --- HasRule tests ---

func TestHasRule(t *testing.T) {
	bashDeny := PermissionRule{Specifier: ToolSpecifier{Tool: "Bash"}, Decision: DecisionDeny}
	bashGitSpec := PermissionRule{Specifier: ToolSpecifier{Tool: "Bash", SubPattern: "git*"}, Decision: DecisionAllow}
	bashGoAllow := PermissionRule{Specifier: ToolSpecifier{Tool: "Bash"}, Decision: DecisionAllow, CommandAllow: []string{"go"}}

	profileWithRules := PolicyProfile{
		Name:  "test",
		Mode:  ModeDefault,
		Rules: []PermissionRule{bashDeny, bashGitSpec, bashGoAllow},
	}
	emptyProfile := PolicyProfile{Name: "empty", Mode: ModeDefault}

	tests := []struct {
		name    string
		profile PolicyProfile
		rule    PermissionRule
		want    bool
	}{
		{"found exact tool rule", profileWithRules, bashDeny, true},
		{"found rule with sub-pattern", profileWithRules, bashGitSpec, true},
		{"found rule with command list", profileWithRules, bashGoAllow, true},
		{"not found - different tool", profileWithRules, PermissionRule{Specifier: ToolSpecifier{Tool: "Read"}, Decision: DecisionDeny}, false},
		{"not found - same tool different sub-pattern", profileWithRules, PermissionRule{Specifier: ToolSpecifier{Tool: "Bash", SubPattern: "npm*"}, Decision: DecisionAllow}, false},
		// A deny rule with the same specifier must not hide a new allow rule.
		{"not found - same specifier different decision", profileWithRules, PermissionRule{Specifier: ToolSpecifier{Tool: "Bash"}, Decision: DecisionAllow}, false},
		{"not found - same specifier different command list", profileWithRules, PermissionRule{Specifier: ToolSpecifier{Tool: "Bash"}, Decision: DecisionAllow, CommandAllow: []string{"npm"}}, false},
		{"not found - empty profile rules", emptyProfile, bashDeny, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.profile.HasRule(&tt.rule)
			if got != tt.want {
				t.Errorf("HasRule(%+v) = %v, want %v", tt.rule, got, tt.want)
			}
		})
	}
}

func TestPermissionRuleEqualNilAndEmptyLists(t *testing.T) {
	a := PermissionRule{Specifier: ToolSpecifier{Tool: "Edit"}, Decision: DecisionAllow}
	b := PermissionRule{Specifier: ToolSpecifier{Tool: "Edit"}, Decision: DecisionAllow, PathDeny: []string{}}
	if !a.Equal(&b) {
		t.Error("nil and empty path_deny should be equal")
	}
}
