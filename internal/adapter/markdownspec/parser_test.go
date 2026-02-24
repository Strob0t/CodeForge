package markdownspec

import (
	"testing"
)

func TestParseMarkdown_Headings(t *testing.T) {
	content := []byte(`# Roadmap

## Phase 1
Some description text.

### Sub-task A
`)
	items := ParseMarkdown(content)

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	if items[0].Title != "Roadmap" || items[0].Level != LevelH1 {
		t.Errorf("item 0: expected H1 'Roadmap', got %s %q", items[0].Level, items[0].Title)
	}
	if items[1].Title != "Phase 1" || items[1].Level != LevelH2 {
		t.Errorf("item 1: expected H2 'Phase 1', got %s %q", items[1].Level, items[1].Title)
	}
	if items[1].Description != "Some description text." {
		t.Errorf("item 1 description: expected 'Some description text.', got %q", items[1].Description)
	}
	if items[2].Title != "Sub-task A" || items[2].Level != LevelH3 {
		t.Errorf("item 2: expected H3 'Sub-task A', got %s %q", items[2].Level, items[2].Title)
	}
}

func TestParseMarkdown_Checkboxes(t *testing.T) {
	content := []byte(`# TODO

- [ ] Implement feature A
- [x] Write tests for B
- [X] Deploy to staging
- [ ] Review PR
`)
	items := ParseMarkdown(content)

	if len(items) != 5 { // 1 heading + 4 checkboxes
		t.Fatalf("expected 5 items, got %d", len(items))
	}

	tests := []struct {
		idx    int
		title  string
		status ItemStatus
		level  ItemLevel
	}{
		{1, "Implement feature A", StatusTodo, LevelCheckbox},
		{2, "Write tests for B", StatusDone, LevelCheckbox},
		{3, "Deploy to staging", StatusDone, LevelCheckbox},
		{4, "Review PR", StatusTodo, LevelCheckbox},
	}

	for _, tc := range tests {
		item := items[tc.idx]
		if item.Title != tc.title {
			t.Errorf("item %d: expected title %q, got %q", tc.idx, tc.title, item.Title)
		}
		if item.Status != tc.status {
			t.Errorf("item %d: expected status %s, got %s", tc.idx, tc.status, item.Status)
		}
		if item.Level != tc.level {
			t.Errorf("item %d: expected level %s, got %s", tc.idx, tc.level, item.Level)
		}
	}
}

func TestParseMarkdown_PlainListItems(t *testing.T) {
	content := []byte(`## Features

- Authentication
- Dashboard
* Settings page
`)
	items := ParseMarkdown(content)

	if len(items) != 4 { // 1 heading + 3 list items
		t.Fatalf("expected 4 items, got %d", len(items))
	}

	if items[1].Title != "Authentication" || items[1].Level != LevelListItem {
		t.Errorf("item 1: expected list_item 'Authentication', got %s %q", items[1].Level, items[1].Title)
	}
	if items[3].Title != "Settings page" || items[3].Level != LevelListItem {
		t.Errorf("item 3: expected list_item 'Settings page', got %s %q", items[3].Level, items[3].Title)
	}
}

func TestParseMarkdown_MixedContent(t *testing.T) {
	content := []byte(`# Project Roadmap

## Phase 1: Foundation

- [x] Set up project structure
- [x] Configure CI/CD
- [ ] Write documentation

## Phase 2: Features

- [ ] User authentication
- [ ] Dashboard page
- REST API endpoints

## Phase 3: Polish

- [ ] Performance optimization
- [ ] Accessibility audit
`)
	items := ParseMarkdown(content)

	// Count: 1 H1 + 3 H2 + 8 checkboxes/list items = 12
	if len(items) != 12 {
		t.Fatalf("expected 12 items, got %d", len(items))
	}

	// Verify sort order is sequential.
	for i, item := range items {
		if item.SortOrder != i+1 {
			t.Errorf("item %d: expected sort_order %d, got %d", i, i+1, item.SortOrder)
		}
	}

	// First two checkboxes should be done.
	if items[2].Status != StatusDone {
		t.Errorf("'Set up project structure' should be done, got %s", items[2].Status)
	}
	if items[3].Status != StatusDone {
		t.Errorf("'Configure CI/CD' should be done, got %s", items[3].Status)
	}
	// Third checkbox should be todo.
	if items[4].Status != StatusTodo {
		t.Errorf("'Write documentation' should be todo, got %s", items[4].Status)
	}
}

func TestParseMarkdown_Empty(t *testing.T) {
	items := ParseMarkdown([]byte{})
	if len(items) != 0 {
		t.Fatalf("expected 0 items for empty content, got %d", len(items))
	}
}

func TestParseMarkdown_SourceLines(t *testing.T) {
	content := []byte(`# Title

- [ ] First item
- [ ] Second item
`)
	items := ParseMarkdown(content)

	if items[0].SourceLine != 1 {
		t.Errorf("heading should be on line 1, got %d", items[0].SourceLine)
	}
	if items[1].SourceLine != 3 {
		t.Errorf("first checkbox should be on line 3, got %d", items[1].SourceLine)
	}
	if items[2].SourceLine != 4 {
		t.Errorf("second checkbox should be on line 4, got %d", items[2].SourceLine)
	}
}

func TestParseMarkdown_DescriptionMultiline(t *testing.T) {
	content := []byte(`## Feature X
This is the first line of description.
This is the second line.

## Feature Y
`)
	items := ParseMarkdown(content)

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	expected := "This is the first line of description.\nThis is the second line."
	if items[0].Description != expected {
		t.Errorf("expected description %q, got %q", expected, items[0].Description)
	}
}

func TestParseMarkdown_AsteriskCheckbox(t *testing.T) {
	content := []byte(`* [ ] Todo item
* [x] Done item
`)
	items := ParseMarkdown(content)

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Status != StatusTodo {
		t.Errorf("expected todo, got %s", items[0].Status)
	}
	if items[1].Status != StatusDone {
		t.Errorf("expected done, got %s", items[1].Status)
	}
}
