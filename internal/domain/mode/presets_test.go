package mode

import "testing"

func TestBuiltinModesContainReviewRefactorModes(t *testing.T) {
	modes := BuiltinModes()
	found := map[string]bool{"boundary-analyzer": false, "contract-reviewer": false}
	for _, m := range modes {
		if _, ok := found[m.ID]; ok {
			found[m.ID] = true
		}
	}
	for id, ok := range found {
		if !ok {
			t.Errorf("mode %q not found in BuiltinModes()", id)
		}
	}
}

func TestBoundaryAnalyzerModeProperties(t *testing.T) {
	modes := BuiltinModes()
	var ba Mode
	for _, m := range modes {
		if m.ID == "boundary-analyzer" {
			ba = m
			break
		}
	}
	if ba.ID == "" {
		t.Fatal("boundary-analyzer mode not found")
	}
	if ba.LLMScenario != "plan" {
		t.Errorf("LLMScenario = %q, want 'plan'", ba.LLMScenario)
	}
	if ba.Autonomy != 4 {
		t.Errorf("Autonomy = %d, want 4", ba.Autonomy)
	}
	if ba.RequiredArtifact != "BOUNDARIES.json" {
		t.Errorf("RequiredArtifact = %q, want 'BOUNDARIES.json'", ba.RequiredArtifact)
	}
}
