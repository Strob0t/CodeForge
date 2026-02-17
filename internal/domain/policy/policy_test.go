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
