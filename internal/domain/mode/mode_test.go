package mode

import (
	"strings"
	"testing"
)

func TestValidate_ValidMode(t *testing.T) {
	m := Mode{ID: "test", Name: "Test", Autonomy: 3}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid mode, got error: %v", err)
	}
}

func TestValidate_MissingID(t *testing.T) {
	m := Mode{Name: "Test", Autonomy: 3}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestValidate_MissingName(t *testing.T) {
	m := Mode{ID: "test", Autonomy: 3}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidate_AutonomyZero(t *testing.T) {
	m := Mode{ID: "test", Name: "Test", Autonomy: 0}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for autonomy 0")
	}
}

func TestValidate_AutonomySix(t *testing.T) {
	m := Mode{ID: "test", Name: "Test", Autonomy: 6}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for autonomy 6")
	}
}

func TestValidate_AutonomyBoundaries(t *testing.T) {
	for _, level := range []int{1, 5} {
		m := Mode{ID: "test", Name: "Test", Autonomy: level}
		if err := m.Validate(); err != nil {
			t.Fatalf("autonomy %d should be valid, got error: %v", level, err)
		}
	}
}

func TestValidate_DeniedToolsOverlap(t *testing.T) {
	m := Mode{
		ID:          "test",
		Name:        "Test",
		Autonomy:    3,
		Tools:       []string{"Read", "Write", "Edit"},
		DeniedTools: []string{"Write"},
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for DeniedTools/Tools overlap")
	}
	if !strings.Contains(err.Error(), `"Write"`) {
		t.Fatalf("expected error to mention Write, got: %v", err)
	}
}

func TestValidate_DeniedToolsNoOverlap(t *testing.T) {
	m := Mode{
		ID:          "test",
		Name:        "Test",
		Autonomy:    3,
		Tools:       []string{"Read", "Glob", "Grep"},
		DeniedTools: []string{"Write", "Edit", "Bash"},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid mode with non-overlapping DeniedTools, got error: %v", err)
	}
}

func TestValidate_DeniedToolsEmptyTools(t *testing.T) {
	m := Mode{
		ID:          "test",
		Name:        "Test",
		Autonomy:    3,
		DeniedTools: []string{"Write", "Edit"},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("DeniedTools without Tools should be valid, got error: %v", err)
	}
}

func TestValidate_DeniedActionsPresent(t *testing.T) {
	m := Mode{
		ID:            "test",
		Name:          "Test",
		Autonomy:      3,
		DeniedActions: []string{"rm", "curl", "wget"},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("DeniedActions should not cause validation error, got: %v", err)
	}
}

func TestValidate_RequiredArtifactPresent(t *testing.T) {
	m := Mode{
		ID:               "test",
		Name:             "Test",
		Autonomy:         3,
		RequiredArtifact: "PLAN.md",
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("RequiredArtifact should not cause validation error, got: %v", err)
	}
}

func TestBuiltinModes_Count(t *testing.T) {
	modes := BuiltinModes()
	if len(modes) != 8 {
		t.Fatalf("expected 8 built-in modes, got %d", len(modes))
	}
}

func TestBuiltinModes_AllValid(t *testing.T) {
	for _, m := range BuiltinModes() {
		if err := m.Validate(); err != nil {
			t.Fatalf("built-in mode %q failed validation: %v", m.ID, err)
		}
		if !m.Builtin {
			t.Fatalf("built-in mode %q has Builtin=false", m.ID)
		}
	}
}

func TestBuiltinModes_NoDeniedToolsOverlap(t *testing.T) {
	for _, m := range BuiltinModes() {
		if len(m.DeniedTools) == 0 || len(m.Tools) == 0 {
			continue
		}
		allowed := make(map[string]bool, len(m.Tools))
		for _, tool := range m.Tools {
			allowed[tool] = true
		}
		for _, denied := range m.DeniedTools {
			if allowed[denied] {
				t.Fatalf("built-in mode %q has %q in both Tools and DeniedTools", m.ID, denied)
			}
		}
	}
}

func TestBuiltinModes_UniqueIDs(t *testing.T) {
	seen := make(map[string]bool)
	for _, m := range BuiltinModes() {
		if seen[m.ID] {
			t.Fatalf("duplicate built-in mode ID: %q", m.ID)
		}
		seen[m.ID] = true
	}
}

func TestValidate_ValidLLMScenario(t *testing.T) {
	for _, scenario := range ValidScenarios {
		m := Mode{ID: "test", Name: "Test", Autonomy: 3, LLMScenario: scenario}
		if err := m.Validate(); err != nil {
			t.Fatalf("scenario %q should be valid, got error: %v", scenario, err)
		}
	}
}

func TestValidate_EmptyLLMScenario(t *testing.T) {
	m := Mode{ID: "test", Name: "Test", Autonomy: 3, LLMScenario: ""}
	if err := m.Validate(); err != nil {
		t.Fatalf("empty llm_scenario should be valid, got error: %v", err)
	}
}

func TestValidate_InvalidLLMScenario(t *testing.T) {
	m := Mode{ID: "test", Name: "Test", Autonomy: 3, LLMScenario: "invalid-scenario"}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for invalid llm_scenario")
	}
	if !strings.Contains(err.Error(), "invalid llm_scenario") {
		t.Fatalf("expected error to mention invalid llm_scenario, got: %v", err)
	}
}

func TestBuiltinModes_ExpectedPresets(t *testing.T) {
	expected := map[string]struct {
		hasRequiredArtifact bool
		hasDeniedTools      bool
	}{
		"architect":  {true, true},
		"coder":      {true, false},
		"reviewer":   {true, true},
		"debugger":   {false, false},
		"tester":     {true, false},
		"documenter": {false, true},
		"refactorer": {true, false},
		"security":   {true, true},
	}

	for _, m := range BuiltinModes() {
		exp, ok := expected[m.ID]
		if !ok {
			t.Fatalf("unexpected built-in mode: %q", m.ID)
		}
		if exp.hasRequiredArtifact && m.RequiredArtifact == "" {
			t.Errorf("mode %q should have a RequiredArtifact", m.ID)
		}
		if exp.hasDeniedTools && len(m.DeniedTools) == 0 {
			t.Errorf("mode %q should have DeniedTools", m.ID)
		}
	}
}
