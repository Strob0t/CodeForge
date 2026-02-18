package service

import (
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
)

func TestRenderMarkdown(t *testing.T) {
	r := &roadmap.Roadmap{
		Title:       "Test Roadmap",
		Description: "A test description",
		Status:      roadmap.StatusActive,
		Milestones: []roadmap.Milestone{
			{
				Title:  "Phase 1",
				Status: roadmap.StatusActive,
				Features: []roadmap.Feature{
					{Title: "Feature A", Status: roadmap.FeatureInProgress, Labels: []string{"backend"}},
					{Title: "Feature B", Status: roadmap.FeatureDone},
				},
			},
		},
	}

	result := renderMarkdown(r)

	if !strings.Contains(result, "# Test Roadmap") {
		t.Error("expected roadmap title in markdown")
	}
	if !strings.Contains(result, "## Phase 1 [active]") {
		t.Error("expected milestone title in markdown")
	}
	if !strings.Contains(result, "[ ] Feature A [in_progress]") {
		t.Error("expected unchecked feature in markdown")
	}
	if !strings.Contains(result, "[x] Feature B [done]") {
		t.Error("expected checked feature in markdown")
	}
	if !strings.Contains(result, "(backend)") {
		t.Error("expected labels in markdown")
	}
}

func TestRenderYAML(t *testing.T) {
	r := &roadmap.Roadmap{
		Title:  "Test Roadmap",
		Status: roadmap.StatusDraft,
		Milestones: []roadmap.Milestone{
			{
				Title:  "Phase 1",
				Status: roadmap.StatusDraft,
				Features: []roadmap.Feature{
					{Title: "Feature A", Status: roadmap.FeatureBacklog, Labels: []string{"ui"}},
				},
			},
		},
	}

	result := renderYAML(r)

	if !strings.Contains(result, `title: "Test Roadmap"`) {
		t.Error("expected roadmap title in YAML")
	}
	if !strings.Contains(result, "status: draft") {
		t.Error("expected status in YAML")
	}
	if !strings.Contains(result, `- "ui"`) {
		t.Error("expected label in YAML")
	}
}
