package markdownspec

import (
	"strings"
	"testing"
)

func TestRenderMarkdown_Basic(t *testing.T) {
	items := []SpecItem{
		{Title: "Roadmap", Level: LevelH1, Status: StatusTodo},
		{Title: "Phase 1", Level: LevelH2, Status: StatusTodo, Description: "Foundation work."},
		{Title: "Setup project", Level: LevelCheckbox, Status: StatusDone},
		{Title: "Write tests", Level: LevelCheckbox, Status: StatusTodo},
	}

	result := string(RenderMarkdown(items))

	if !strings.Contains(result, "# Roadmap") {
		t.Error("expected H1 heading")
	}
	if !strings.Contains(result, "## Phase 1") {
		t.Error("expected H2 heading")
	}
	if !strings.Contains(result, "Foundation work.") {
		t.Error("expected description")
	}
	if !strings.Contains(result, "- [x] Setup project") {
		t.Error("expected done checkbox")
	}
	if !strings.Contains(result, "- [ ] Write tests") {
		t.Error("expected todo checkbox")
	}
}

func TestRenderMarkdown_ListItems(t *testing.T) {
	items := []SpecItem{
		{Title: "Features", Level: LevelH2, Status: StatusTodo},
		{Title: "Authentication", Level: LevelListItem, Status: StatusTodo},
		{Title: "Dashboard", Level: LevelListItem, Status: StatusTodo},
	}

	result := string(RenderMarkdown(items))

	if !strings.Contains(result, "- Authentication") {
		t.Error("expected plain list item 'Authentication'")
	}
	if !strings.Contains(result, "- Dashboard") {
		t.Error("expected plain list item 'Dashboard'")
	}
}

func TestRoundTrip_ParseThenRender(t *testing.T) {
	original := `# Project TODO

## Phase 1

- [x] Set up repo
- [x] Add CI
- [ ] Write docs

## Phase 2

- [ ] Build API
- [ ] Add tests
`
	items := ParseMarkdown([]byte(original))
	rendered := string(RenderMarkdown(items))

	// Re-parse the rendered output.
	reparsed := ParseMarkdown([]byte(rendered))

	if len(items) != len(reparsed) {
		t.Fatalf("round-trip item count mismatch: %d vs %d", len(items), len(reparsed))
	}

	for i := range items {
		if items[i].Title != reparsed[i].Title {
			t.Errorf("item %d title mismatch: %q vs %q", i, items[i].Title, reparsed[i].Title)
		}
		if items[i].Status != reparsed[i].Status {
			t.Errorf("item %d status mismatch: %s vs %s", i, items[i].Status, reparsed[i].Status)
		}
		if items[i].Level != reparsed[i].Level {
			t.Errorf("item %d level mismatch: %s vs %s", i, items[i].Level, reparsed[i].Level)
		}
	}
}

func TestRoundTrip_StatusChange(t *testing.T) {
	original := `# Tasks

- [ ] Item A
- [ ] Item B
- [x] Item C
`
	items := ParseMarkdown([]byte(original))

	// Toggle Item A to done.
	items[1].Status = StatusDone
	// Toggle Item C to todo.
	items[3].Status = StatusTodo

	rendered := string(RenderMarkdown(items))

	if !strings.Contains(rendered, "- [x] Item A") {
		t.Error("expected Item A to be checked after toggle")
	}
	if !strings.Contains(rendered, "- [ ] Item C") {
		t.Error("expected Item C to be unchecked after toggle")
	}
}

func TestRenderMarkdown_Empty(t *testing.T) {
	result := RenderMarkdown(nil)
	if len(result) != 0 {
		t.Errorf("expected empty output for nil items, got %q", string(result))
	}
}
