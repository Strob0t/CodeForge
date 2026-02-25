package service

import (
	"strings"
	"testing"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
)

func TestBuildModePrompt_BuiltinArchitect(t *testing.T) {
	m := findBuiltin(t, "architect")
	prompt, sections := BuildModePrompt(m)

	if prompt == "" {
		t.Fatal("expected non-empty assembled prompt")
	}

	// Architect has Tools, DeniedTools, DeniedActions, RequiredArtifact — all 5 sections should appear.
	if len(sections) != 5 {
		t.Fatalf("expected 5 sections for architect, got %d", len(sections))
	}

	expectContains(t, prompt, "Architect")
	expectContains(t, prompt, "Read")          // in tools
	expectContains(t, prompt, "Write")         // in denied tools
	expectContains(t, prompt, "PLAN.md")       // required artifact
	expectContains(t, prompt, "rm")            // denied action
	expectContains(t, prompt, "Do not exceed") // guardrails
}

func TestBuildModePrompt_BuiltinDebugger(t *testing.T) {
	m := findBuiltin(t, "debugger")
	prompt, sections := BuildModePrompt(m)

	if prompt == "" {
		t.Fatal("expected non-empty assembled prompt")
	}

	// Debugger has no DeniedTools and no RequiredArtifact — those sections should be omitted.
	sectionNames := make(map[string]bool)
	for _, s := range sections {
		sectionNames[s.Name] = true
	}

	if sectionNames["artifact"] {
		t.Error("debugger should not have an artifact section")
	}

	expectContains(t, prompt, "Debugger")
	expectContains(t, prompt, "Diagnoses and fixes bugs")
}

func TestBuildModePrompt_BuiltinCoder(t *testing.T) {
	m := findBuiltin(t, "coder")
	prompt, sections := BuildModePrompt(m)

	if prompt == "" {
		t.Fatal("expected non-empty assembled prompt")
	}

	// Coder has no DeniedTools — "tools" section should only list available tools, not denied.
	for _, s := range sections {
		if s.Name == "tools" && strings.Contains(s.Text, "must NOT use") {
			t.Error("coder should not have denied tools in tools section")
		}
	}

	expectContains(t, prompt, "Coder")
	expectContains(t, prompt, "DIFF")
}

func TestBuildModePrompt_CustomModeWithPromptPrefix(t *testing.T) {
	m := &mode.Mode{
		ID:           "custom-1",
		Name:         "Custom",
		Builtin:      false,
		PromptPrefix: "You are a custom agent with special instructions.",
		Autonomy:     3,
	}
	prompt, sections := BuildModePrompt(m)

	if prompt != m.PromptPrefix {
		t.Fatalf("custom mode should return raw PromptPrefix, got %q", prompt)
	}
	if len(sections) != 1 || sections[0].Name != "custom" {
		t.Fatalf("expected 1 custom section, got %d", len(sections))
	}
}

func TestBuildModePrompt_CustomModeWithoutPromptPrefix(t *testing.T) {
	m := &mode.Mode{
		ID:          "custom-2",
		Name:        "Analyzer",
		Description: "Analyzes code patterns and provides insights.",
		Builtin:     false,
		Tools:       []string{"Read", "Grep"},
		Autonomy:    2,
	}
	prompt, sections := BuildModePrompt(m)

	if prompt == "" {
		t.Fatal("expected non-empty assembled prompt")
	}

	expectContains(t, prompt, "Analyzer")
	expectContains(t, prompt, "Analyzes code patterns")

	// Should have at least role, tools, and guardrails sections.
	if len(sections) < 3 {
		t.Fatalf("expected at least 3 sections, got %d", len(sections))
	}
}

func TestBuildModePrompt_TokenCounting(t *testing.T) {
	m := findBuiltin(t, "reviewer")
	_, sections := BuildModePrompt(m)

	for _, s := range sections {
		if s.Tokens <= 0 {
			t.Errorf("section %q should have positive token count, got %d", s.Name, s.Tokens)
		}
		expected := cfcontext.EstimateTokens(s.Text)
		if s.Tokens != expected {
			t.Errorf("section %q token count mismatch: got %d, want %d", s.Name, s.Tokens, expected)
		}
	}
}

func TestBuildModePrompt_AllBuiltins(t *testing.T) {
	builtins := mode.BuiltinModes()
	for i := range builtins {
		prompt, sections := BuildModePrompt(&builtins[i])
		if prompt == "" {
			t.Errorf("built-in mode %q produced empty prompt", builtins[i].ID)
		}
		if len(sections) == 0 {
			t.Errorf("built-in mode %q produced no sections", builtins[i].ID)
		}
	}
}

func TestBuildModePrompt_StartsWithYouAre(t *testing.T) {
	builtins := mode.BuiltinModes()
	for i := range builtins {
		prompt, _ := BuildModePrompt(&builtins[i])
		if !strings.HasPrefix(prompt, "You are a") {
			t.Errorf("built-in mode %q prompt should start with 'You are a', got prefix %q", builtins[i].ID, prompt[:min(30, len(prompt))])
		}
	}
}

func TestWarnIfOverBudget_Under(t *testing.T) {
	sections := []PromptSection{
		{Name: "role", Text: "short", Tokens: 10},
		{Name: "tools", Text: "short", Tokens: 10},
	}
	// Should not panic or log error — budget is not exceeded.
	WarnIfOverBudget("test", sections, 100)
}

func TestWarnIfOverBudget_Over(t *testing.T) {
	sections := []PromptSection{
		{Name: "role", Text: "long text", Tokens: 600},
		{Name: "tools", Text: "long text", Tokens: 500},
	}
	// Total = 1100, budget = 1024. Should log a warning (no panic).
	WarnIfOverBudget("test", sections, 1024)
}

func TestPruneToFitBudget_UnderBudget(t *testing.T) {
	sections := []PromptSection{
		{Name: "role", Text: "role text", Tokens: 30, Priority: PriorityRole, Enabled: true},
		{Name: "tools", Text: "tools text", Tokens: 20, Priority: PriorityTools, Enabled: true},
	}
	result := PruneToFitBudget(sections, 100)
	if len(result) != 2 {
		t.Fatalf("expected 2 sections (under budget), got %d", len(result))
	}
}

func TestPruneToFitBudget_RemovesLowestPriority(t *testing.T) {
	sections := []PromptSection{
		{Name: "role", Text: "role text", Tokens: 50, Priority: PriorityRole, Enabled: true},
		{Name: "user", Text: "user text", Tokens: 40, Priority: PriorityUser, Enabled: true},
		{Name: "safety", Text: "safety text", Tokens: 30, Priority: PrioritySafety, Enabled: true},
	}
	// Total = 120, budget = 80. Should remove "user" (lowest priority=40) first.
	result := PruneToFitBudget(sections, 80)
	if len(result) != 2 {
		t.Fatalf("expected 2 sections after pruning, got %d", len(result))
	}
	for _, s := range result {
		if s.Name == "user" {
			t.Error("lowest-priority section 'user' should have been removed")
		}
	}
}

func TestPruneToFitBudget_PreservesOrder(t *testing.T) {
	sections := []PromptSection{
		{Name: "safety", Text: "safety", Tokens: 30, Priority: PrioritySafety, Enabled: true},
		{Name: "role", Text: "role", Tokens: 30, Priority: PriorityRole, Enabled: true},
		{Name: "user", Text: "user", Tokens: 30, Priority: PriorityUser, Enabled: true},
	}
	// Total = 90, budget = 65. Remove "user" (priority=40), keep safety+role in original order.
	result := PruneToFitBudget(sections, 65)
	if len(result) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(result))
	}
	if result[0].Name != "safety" || result[1].Name != "role" {
		t.Errorf("expected [safety, role] order, got [%s, %s]", result[0].Name, result[1].Name)
	}
}

func TestPruneToFitBudget_ZeroBudget(t *testing.T) {
	sections := []PromptSection{
		{Name: "role", Text: "text", Tokens: 50, Priority: PriorityRole, Enabled: true},
	}
	// Budget <= 0 means no pruning.
	result := PruneToFitBudget(sections, 0)
	if len(result) != 1 {
		t.Fatalf("expected 1 section (zero budget = no pruning), got %d", len(result))
	}
}

func TestAssembleSections_SkipsDisabledAndEmpty(t *testing.T) {
	sections := []PromptSection{
		{Name: "role", Text: "Role text.", Enabled: true},
		{Name: "hidden", Text: "Secret.", Enabled: false},
		{Name: "empty", Text: "", Enabled: true},
		{Name: "tools", Text: "Tools text.", Enabled: true},
	}
	result := AssembleSections(sections)
	if !strings.Contains(result, "Role text.") {
		t.Error("expected 'Role text.' in result")
	}
	if strings.Contains(result, "Secret.") {
		t.Error("disabled section should be excluded")
	}
	if !strings.Contains(result, "Tools text.") {
		t.Error("expected 'Tools text.' in result")
	}
	// Should be joined with double newlines.
	if !strings.Contains(result, "Role text.\n\nTools text.") {
		t.Errorf("sections should be joined with \\n\\n, got %q", result)
	}
}

// --- helpers ---

func findBuiltin(t *testing.T, id string) *mode.Mode {
	t.Helper()
	builtins := mode.BuiltinModes()
	for i := range builtins {
		if builtins[i].ID == id {
			return &builtins[i]
		}
	}
	t.Fatalf("built-in mode %q not found", id)
	return nil
}

func expectContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected prompt to contain %q, got:\n%s", substr, s[:min(200, len(s))])
	}
}
