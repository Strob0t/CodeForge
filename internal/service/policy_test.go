package service

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/policy"
)

func TestEvaluateExactToolMatch(t *testing.T) {
	svc := NewPolicyService("plan-readonly", nil)

	d, err := svc.Evaluate(context.Background(), "plan-readonly", policy.ToolCall{Tool: "Read"})
	if err != nil {
		t.Fatal(err)
	}
	if d != policy.DecisionAllow {
		t.Errorf("expected allow for Read in plan-readonly, got %q", d)
	}
}

func TestEvaluateFirstMatchWins(t *testing.T) {
	profile := policy.PolicyProfile{
		Name: "test",
		Mode: policy.ModeDefault,
		Rules: []policy.PermissionRule{
			{Specifier: policy.ToolSpecifier{Tool: "Bash"}, Decision: policy.DecisionDeny},
			{Specifier: policy.ToolSpecifier{Tool: "Bash"}, Decision: policy.DecisionAllow},
		},
	}
	svc := NewPolicyService("test", []policy.PolicyProfile{profile})

	d, err := svc.Evaluate(context.Background(), "test", policy.ToolCall{Tool: "Bash", Command: "ls"})
	if err != nil {
		t.Fatal(err)
	}
	if d != policy.DecisionDeny {
		t.Errorf("expected deny (first match), got %q", d)
	}
}

func TestEvaluatePathDeny(t *testing.T) {
	profile := policy.PolicyProfile{
		Name: "test",
		Mode: policy.ModeDefault,
		Rules: []policy.PermissionRule{
			{
				Specifier: policy.ToolSpecifier{Tool: "Edit"},
				Decision:  policy.DecisionAllow,
				PathDeny:  []string{".env", "secrets/**"},
			},
		},
	}
	svc := NewPolicyService("test", []policy.PolicyProfile{profile})
	ctx := context.Background()

	// Denied path
	d, _ := svc.Evaluate(ctx, "test", policy.ToolCall{Tool: "Edit", Path: ".env"})
	if d != policy.DecisionAsk {
		t.Errorf("expected ask (path denied, falls to mode default), got %q", d)
	}

	// Denied by ** pattern
	d, _ = svc.Evaluate(ctx, "test", policy.ToolCall{Tool: "Edit", Path: "secrets/api.key"})
	if d != policy.DecisionAsk {
		t.Errorf("expected ask for secrets/** path, got %q", d)
	}

	// Allowed path
	d, _ = svc.Evaluate(ctx, "test", policy.ToolCall{Tool: "Edit", Path: "src/main.go"})
	if d != policy.DecisionAllow {
		t.Errorf("expected allow for src/main.go, got %q", d)
	}
}

func TestEvaluatePathAllow(t *testing.T) {
	profile := policy.PolicyProfile{
		Name: "test",
		Mode: policy.ModeDefault,
		Rules: []policy.PermissionRule{
			{
				Specifier: policy.ToolSpecifier{Tool: "Edit"},
				Decision:  policy.DecisionAllow,
				PathAllow: []string{"src/**", "tests/**"},
			},
		},
	}
	svc := NewPolicyService("test", []policy.PolicyProfile{profile})
	ctx := context.Background()

	// Allowed
	d, _ := svc.Evaluate(ctx, "test", policy.ToolCall{Tool: "Edit", Path: "src/main.go"})
	if d != policy.DecisionAllow {
		t.Errorf("expected allow for src/main.go, got %q", d)
	}

	// Not in allow list
	d, _ = svc.Evaluate(ctx, "test", policy.ToolCall{Tool: "Edit", Path: "config/app.yaml"})
	if d != policy.DecisionAsk {
		t.Errorf("expected ask (not in path_allow), got %q", d)
	}
}

func TestEvaluateCommandAllow(t *testing.T) {
	profile := policy.PolicyProfile{
		Name: "test",
		Mode: policy.ModeDefault,
		Rules: []policy.PermissionRule{
			{
				Specifier:    policy.ToolSpecifier{Tool: "Bash"},
				Decision:     policy.DecisionAllow,
				CommandAllow: []string{"git status", "go test"},
			},
			{Specifier: policy.ToolSpecifier{Tool: "Bash"}, Decision: policy.DecisionDeny},
		},
	}
	svc := NewPolicyService("test", []policy.PolicyProfile{profile})
	ctx := context.Background()

	// Allowed command
	d, _ := svc.Evaluate(ctx, "test", policy.ToolCall{Tool: "Bash", Command: "git status"})
	if d != policy.DecisionAllow {
		t.Errorf("expected allow for 'git status', got %q", d)
	}

	// Allowed command with args
	d, _ = svc.Evaluate(ctx, "test", policy.ToolCall{Tool: "Bash", Command: "go test ./..."})
	if d != policy.DecisionAllow {
		t.Errorf("expected allow for 'go test ./...', got %q", d)
	}

	// Denied command
	d, _ = svc.Evaluate(ctx, "test", policy.ToolCall{Tool: "Bash", Command: "rm -rf /"})
	if d != policy.DecisionDeny {
		t.Errorf("expected deny for 'rm -rf /', got %q", d)
	}
}

func TestEvaluateCommandDeny(t *testing.T) {
	// To deny specific commands: use CommandAllow on a deny rule.
	// CommandAllow restricts the rule to only match those commands.
	profile := policy.PolicyProfile{
		Name: "test",
		Mode: policy.ModeAcceptEdits,
		Rules: []policy.PermissionRule{
			{
				Specifier:    policy.ToolSpecifier{Tool: "Bash"},
				Decision:     policy.DecisionDeny,
				CommandAllow: []string{"curl", "wget", "ssh"},
			},
			{Specifier: policy.ToolSpecifier{Tool: "Bash"}, Decision: policy.DecisionAllow},
		},
	}
	svc := NewPolicyService("test", []policy.PolicyProfile{profile})
	ctx := context.Background()

	// Denied (matches deny rule's CommandAllow)
	d, _ := svc.Evaluate(ctx, "test", policy.ToolCall{Tool: "Bash", Command: "curl https://example.com"})
	if d != policy.DecisionDeny {
		t.Errorf("expected deny for 'curl', got %q", d)
	}

	// Allowed (doesn't match deny rule's CommandAllow, falls to allow rule)
	d, _ = svc.Evaluate(ctx, "test", policy.ToolCall{Tool: "Bash", Command: "ls -la"})
	if d != policy.DecisionAllow {
		t.Errorf("expected allow for 'ls -la', got %q", d)
	}
}

func TestEvaluateSubPattern(t *testing.T) {
	profile := policy.PolicyProfile{
		Name: "test",
		Mode: policy.ModeDefault,
		Rules: []policy.PermissionRule{
			{
				Specifier: policy.ToolSpecifier{Tool: "Bash", SubPattern: "git*"},
				Decision:  policy.DecisionAllow,
			},
			{Specifier: policy.ToolSpecifier{Tool: "Bash"}, Decision: policy.DecisionDeny},
		},
	}
	svc := NewPolicyService("test", []policy.PolicyProfile{profile})
	ctx := context.Background()

	// Matches sub-pattern
	d, _ := svc.Evaluate(ctx, "test", policy.ToolCall{Tool: "Bash", Command: "git status"})
	if d != policy.DecisionAllow {
		t.Errorf("expected allow for 'git status' matching 'git*', got %q", d)
	}

	// Does not match sub-pattern, falls to deny
	d, _ = svc.Evaluate(ctx, "test", policy.ToolCall{Tool: "Bash", Command: "rm -rf"})
	if d != policy.DecisionDeny {
		t.Errorf("expected deny for 'rm -rf', got %q", d)
	}
}

func TestEvaluateNoRulesMatch(t *testing.T) {
	profile := policy.PolicyProfile{
		Name: "test",
		Mode: policy.ModeDefault,
		Rules: []policy.PermissionRule{
			{Specifier: policy.ToolSpecifier{Tool: "Read"}, Decision: policy.DecisionAllow},
		},
	}
	svc := NewPolicyService("test", []policy.PolicyProfile{profile})

	// Tool not covered by any rule -> mode default
	d, _ := svc.Evaluate(context.Background(), "test", policy.ToolCall{Tool: "Write"})
	if d != policy.DecisionAsk {
		t.Errorf("expected ask (mode default), got %q", d)
	}
}

func TestEvaluateDefaultDecisionByMode(t *testing.T) {
	tests := []struct {
		mode     policy.PermissionMode
		expected policy.Decision
	}{
		{policy.ModePlan, policy.DecisionDeny},
		{policy.ModeDefault, policy.DecisionAsk},
		{policy.ModeAcceptEdits, policy.DecisionAllow},
		{policy.ModeDelegate, policy.DecisionAllow},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			got := defaultDecisionForMode(tt.mode)
			if got != tt.expected {
				t.Errorf("mode %q: expected %q, got %q", tt.mode, tt.expected, got)
			}
		})
	}
}

// --- Glob matching tests ---

func TestMatchGlobExact(t *testing.T) {
	if !matchGlob(".env", ".env") {
		t.Error("expected .env to match .env")
	}
	if matchGlob(".env", ".env.local") {
		t.Error("expected .env not to match .env.local")
	}
}

func TestMatchGlobStar(t *testing.T) {
	if !matchGlob("*.go", "main.go") {
		t.Error("expected *.go to match main.go")
	}
	if matchGlob("*.go", "src/main.go") {
		t.Error("expected *.go not to match src/main.go (single *)")
	}
}

func TestMatchGlobDoubleStar(t *testing.T) {
	if !matchGlob("**/*.go", "src/main.go") {
		t.Error("expected **/*.go to match src/main.go")
	}
	if !matchGlob("**/*.go", "internal/service/policy.go") {
		t.Error("expected **/*.go to match internal/service/policy.go")
	}
	if !matchGlob("secrets/**", "secrets/api.key") {
		t.Error("expected secrets/** to match secrets/api.key")
	}
	if !matchGlob("secrets/**", "secrets/nested/deep.key") {
		t.Error("expected secrets/** to match secrets/nested/deep.key")
	}
	if matchGlob("secrets/**", "other/file.txt") {
		t.Error("expected secrets/** not to match other/file.txt")
	}
}

func TestMatchGlobNoMatch(t *testing.T) {
	if matchGlob("*.ts", "main.go") {
		t.Error("expected *.ts not to match main.go")
	}
}

func TestMatchGlobDoubleStarEnv(t *testing.T) {
	if !matchGlob("**/.env", "src/.env") {
		t.Error("expected **/.env to match src/.env")
	}
	if !matchGlob("**/.env", "deep/nested/.env") {
		t.Error("expected **/.env to match deep/nested/.env")
	}
}

// --- PolicyService tests ---

func TestPolicyServiceListProfiles(t *testing.T) {
	custom := []policy.PolicyProfile{
		{Name: "custom-one", Mode: policy.ModeDefault},
	}
	svc := NewPolicyService("headless-safe-sandbox", custom)
	names := svc.ListProfiles()

	if len(names) != 5 {
		t.Fatalf("expected 5 profiles (4 presets + 1 custom), got %d: %v", len(names), names)
	}

	found := false
	for _, n := range names {
		if n == "custom-one" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'custom-one' in profile list")
	}
}

func TestPolicyServiceGetProfile(t *testing.T) {
	svc := NewPolicyService("headless-safe-sandbox", nil)

	p, ok := svc.GetProfile("plan-readonly")
	if !ok {
		t.Fatal("expected to find plan-readonly")
	}
	if p.Name != "plan-readonly" {
		t.Errorf("expected name 'plan-readonly', got %q", p.Name)
	}
}

func TestPolicyServiceGetProfileUnknown(t *testing.T) {
	svc := NewPolicyService("headless-safe-sandbox", nil)

	_, ok := svc.GetProfile("nonexistent")
	if ok {
		t.Error("expected false for unknown profile")
	}
}

func TestPolicyServiceDefaultProfile(t *testing.T) {
	svc := NewPolicyService("plan-readonly", nil)
	if svc.DefaultProfile() != "plan-readonly" {
		t.Errorf("expected 'plan-readonly', got %q", svc.DefaultProfile())
	}
}

func TestPolicyServiceEvaluateUnknownProfile(t *testing.T) {
	svc := NewPolicyService("headless-safe-sandbox", nil)

	_, err := svc.Evaluate(context.Background(), "nonexistent", policy.ToolCall{Tool: "Read"})
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestPolicyServiceCustomOverridesPreset(t *testing.T) {
	custom := policy.PolicyProfile{
		Name: "plan-readonly",
		Mode: policy.ModeAcceptEdits, // Override with different mode
	}
	svc := NewPolicyService("plan-readonly", []policy.PolicyProfile{custom})

	p, ok := svc.GetProfile("plan-readonly")
	if !ok {
		t.Fatal("expected profile")
	}
	if p.Mode != policy.ModeAcceptEdits {
		t.Errorf("expected custom override mode, got %q", p.Mode)
	}
}

// --- Integration-style tests with real presets ---

func TestEvaluatePlanReadonlyDeniesEdit(t *testing.T) {
	svc := NewPolicyService("plan-readonly", nil)
	d, _ := svc.Evaluate(context.Background(), "plan-readonly", policy.ToolCall{Tool: "Edit", Path: "src/main.go"})
	if d != policy.DecisionDeny {
		t.Errorf("plan-readonly should deny Edit, got %q", d)
	}
}

func TestEvaluatePlanReadonlyAllowsRead(t *testing.T) {
	svc := NewPolicyService("plan-readonly", nil)
	d, _ := svc.Evaluate(context.Background(), "plan-readonly", policy.ToolCall{Tool: "Read", Path: "src/main.go"})
	if d != policy.DecisionAllow {
		t.Errorf("plan-readonly should allow Read, got %q", d)
	}
}

func TestEvaluateHeadlessSafeDeniesUnknownBash(t *testing.T) {
	svc := NewPolicyService("headless-safe-sandbox", nil)
	d, _ := svc.Evaluate(context.Background(), "headless-safe-sandbox", policy.ToolCall{Tool: "Bash", Command: "rm -rf /"})
	if d != policy.DecisionDeny {
		t.Errorf("headless-safe-sandbox should deny 'rm -rf /', got %q", d)
	}
}

func TestEvaluateHeadlessSafeAllowsGitStatus(t *testing.T) {
	svc := NewPolicyService("headless-safe-sandbox", nil)
	d, _ := svc.Evaluate(context.Background(), "headless-safe-sandbox", policy.ToolCall{Tool: "Bash", Command: "git status"})
	if d != policy.DecisionAllow {
		t.Errorf("headless-safe-sandbox should allow 'git status', got %q", d)
	}
}

func TestEvaluateTrustedMountAllowsEverything(t *testing.T) {
	svc := NewPolicyService("trusted-mount-autonomous", nil)
	d, _ := svc.Evaluate(context.Background(), "trusted-mount-autonomous", policy.ToolCall{Tool: "Bash", Command: "anything"})
	if d != policy.DecisionAllow {
		t.Errorf("trusted-mount should allow Bash, got %q", d)
	}
}

func TestEvaluateTrustedMountDeniesSecrets(t *testing.T) {
	svc := NewPolicyService("trusted-mount-autonomous", nil)
	d, _ := svc.Evaluate(context.Background(), "trusted-mount-autonomous", policy.ToolCall{Tool: "Edit", Path: "secrets/api.key"})
	if d != policy.DecisionDeny {
		t.Errorf("trusted-mount should deny Edit on secrets/**, got %q", d)
	}
}
