package branchprotection

import "testing"

func TestCreateRuleRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateRuleRequest
		wantErr bool
	}{
		{"valid", CreateRuleRequest{ProjectID: "p1", BranchPattern: "main"}, false},
		{"missing project", CreateRuleRequest{BranchPattern: "main"}, true},
		{"missing pattern", CreateRuleRequest{ProjectID: "p1"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApply(t *testing.T) {
	rule := ProtectionRule{
		BranchPattern:  "main",
		RequireReviews: false,
		AllowForcePush: true,
	}
	trueVal := true
	newPattern := "release/*"
	rule.Apply(UpdateRuleRequest{
		BranchPattern:  &newPattern,
		RequireReviews: &trueVal,
	})
	if rule.BranchPattern != "release/*" {
		t.Errorf("expected branch_pattern 'release/*', got %q", rule.BranchPattern)
	}
	if !rule.RequireReviews {
		t.Error("expected require_reviews to be true")
	}
	if !rule.AllowForcePush {
		t.Error("expected allow_force_push unchanged (true)")
	}
}

func TestEvaluatePush(t *testing.T) {
	rules := []ProtectionRule{
		{BranchPattern: "main", Enabled: true, AllowForcePush: false},
		{BranchPattern: "dev", Enabled: true, AllowForcePush: true},
		{BranchPattern: "disabled", Enabled: false, AllowForcePush: false},
	}

	tests := []struct {
		name    string
		action  PushAction
		allowed bool
	}{
		{"normal push to main", PushAction{Branch: "main"}, true},
		{"force push to main denied", PushAction{Branch: "main", ForcePush: true}, false},
		{"force push to dev allowed", PushAction{Branch: "dev", ForcePush: true}, true},
		// P1-4: default-deny when enabled rules exist but none match
		{"push to unprotected branch (default deny)", PushAction{Branch: "feature/x"}, false},
		{"push to disabled rule (default deny)", PushAction{Branch: "disabled", ForcePush: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EvaluatePush(rules, tt.action)
			if result.Allowed != tt.allowed {
				t.Errorf("EvaluatePush() allowed = %v, want %v (reason: %s)", result.Allowed, tt.allowed, result.Reason)
			}
		})
	}
}

func TestEvaluatePush_NoRules(t *testing.T) {
	// No rules at all: allow (backward compat)
	result := EvaluatePush(nil, PushAction{Branch: "main"})
	if !result.Allowed {
		t.Errorf("expected allow with no rules, got deny (reason: %s)", result.Reason)
	}

	// Only disabled rules: allow (no enabled rules)
	rules := []ProtectionRule{{BranchPattern: "main", Enabled: false}}
	result = EvaluatePush(rules, PushAction{Branch: "main"})
	if !result.Allowed {
		t.Errorf("expected allow with only disabled rules, got deny (reason: %s)", result.Reason)
	}
}

func TestEvaluateMerge(t *testing.T) {
	rules := []ProtectionRule{
		{BranchPattern: "main", Enabled: true, RequireReviews: true, RequireTests: true, RequireLint: true},
		{BranchPattern: "staging", Enabled: true, RequireTests: true},
	}

	tests := []struct {
		name    string
		action  MergeAction
		allowed bool
	}{
		{"all checks pass", MergeAction{TargetBranch: "main", TestsPassed: true, LintPassed: true, HasReviews: true}, true},
		{"missing tests", MergeAction{TargetBranch: "main", LintPassed: true, HasReviews: true}, false},
		{"missing lint", MergeAction{TargetBranch: "main", TestsPassed: true, HasReviews: true}, false},
		{"missing reviews", MergeAction{TargetBranch: "main", TestsPassed: true, LintPassed: true}, false},
		{"staging tests pass", MergeAction{TargetBranch: "staging", TestsPassed: true}, true},
		{"staging tests fail", MergeAction{TargetBranch: "staging"}, false},
		// P1-4: default-deny when enabled rules exist but none match
		{"unprotected branch (default deny)", MergeAction{TargetBranch: "feature/x"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EvaluateMerge(rules, tt.action)
			if result.Allowed != tt.allowed {
				t.Errorf("EvaluateMerge() allowed = %v, want %v (reason: %s)", result.Allowed, tt.allowed, result.Reason)
			}
		})
	}
}

func TestEvaluateDelete(t *testing.T) {
	rules := []ProtectionRule{
		{BranchPattern: "main", Enabled: true, AllowDelete: false},
		{BranchPattern: "temp-*", Enabled: true, AllowDelete: true},
	}

	tests := []struct {
		name    string
		branch  string
		allowed bool
	}{
		{"delete main denied", "main", false},
		{"delete temp branch allowed", "temp-fix", true},
		// P1-4: default-deny when enabled rules exist but none match
		{"delete unprotected branch (default deny)", "feature/x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EvaluateDelete(rules, tt.branch)
			if result.Allowed != tt.allowed {
				t.Errorf("EvaluateDelete(%q) allowed = %v, want %v (reason: %s)", tt.branch, result.Allowed, tt.allowed, result.Reason)
			}
		})
	}
}

func TestMatchBranch_GlobPatterns(t *testing.T) {
	tests := []struct {
		pattern string
		branch  string
		want    bool
	}{
		{"main", "main", true},
		{"main", "staging", false},
		{"release/*", "release/1.0", true},
		{"release/*", "release/2.0.1", true},
		{"release/*", "main", false},
		{"feature-*", "feature-auth", true},
		{"*", "anything", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_vs_"+tt.branch, func(t *testing.T) {
			if got := matchBranch(tt.pattern, tt.branch); got != tt.want {
				t.Errorf("matchBranch(%q, %q) = %v, want %v", tt.pattern, tt.branch, got, tt.want)
			}
		})
	}
}
