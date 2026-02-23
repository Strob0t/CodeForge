package project

import (
	"testing"
)

func TestManifestMapNotEmpty(t *testing.T) {
	if len(manifestMap) == 0 {
		t.Fatal("manifestMap is empty")
	}
	for filename, lang := range manifestMap {
		if filename == "" {
			t.Error("empty filename in manifestMap")
		}
		if lang == "" {
			t.Errorf("empty language for manifest %q", filename)
		}
	}
}

func TestFrameworkDetectorsReferenceValidLanguages(t *testing.T) {
	// Collect all languages known from manifestMap.
	knownLangs := make(map[string]bool)
	for _, lang := range manifestMap {
		knownLangs[lang] = true
	}

	for lang, rules := range frameworkDetectors {
		if !knownLangs[lang] {
			t.Errorf("frameworkDetectors references unknown language %q", lang)
		}
		for _, rule := range rules {
			if rule.Manifest == "" {
				t.Errorf("empty manifest in frameworkDetectors[%q]", lang)
			}
			if rule.Substring == "" {
				t.Errorf("empty substring in frameworkDetectors[%q]", lang)
			}
			if rule.Framework == "" {
				t.Errorf("empty framework in frameworkDetectors[%q]", lang)
			}
		}
	}
}

func TestToolRecommendationsReferenceValidLanguages(t *testing.T) {
	knownLangs := make(map[string]bool)
	for _, lang := range manifestMap {
		knownLangs[lang] = true
	}

	for lang, recs := range toolRecommendations {
		if !knownLangs[lang] {
			t.Errorf("toolRecommendations references unknown language %q", lang)
		}
		for _, rec := range recs {
			if rec.Category == "" || rec.ID == "" || rec.Name == "" {
				t.Errorf("incomplete recommendation in toolRecommendations[%q]: %+v", lang, rec)
			}
		}
	}
}

func TestCoreModeRecommendations(t *testing.T) {
	recs := coreModeRecommendations("go")
	if len(recs) == 0 {
		t.Fatal("expected non-empty core mode recommendations")
	}

	// All known mode IDs from presets.
	expectedModes := map[string]bool{
		"coder": true, "reviewer": true, "tester": true,
		"security": true, "architect": true,
	}
	for _, rec := range recs {
		if rec.Category != "mode" {
			t.Errorf("expected category 'mode', got %q", rec.Category)
		}
		if !expectedModes[rec.ID] {
			t.Errorf("unexpected mode ID %q", rec.ID)
		}
	}
}

func TestCorePipelineRecommendations(t *testing.T) {
	recs := corePipelineRecommendations("go")
	if len(recs) == 0 {
		t.Fatal("expected non-empty core pipeline recommendations")
	}

	expectedPipelines := map[string]bool{
		"standard-dev": true, "review-only": true,
	}
	for _, rec := range recs {
		if rec.Category != "pipeline" {
			t.Errorf("expected category 'pipeline', got %q", rec.Category)
		}
		if !expectedPipelines[rec.ID] {
			t.Errorf("unexpected pipeline ID %q", rec.ID)
		}
	}
}

func TestRecommendationsForLanguage(t *testing.T) {
	recs := RecommendationsForLanguage("go")
	categories := make(map[string]bool)
	for _, rec := range recs {
		categories[rec.Category] = true
	}

	// Should have mode, pipeline, and tool recommendations.
	for _, cat := range []string{"mode", "pipeline", "linter", "formatter"} {
		if !categories[cat] {
			t.Errorf("expected category %q in recommendations for go", cat)
		}
	}
}
