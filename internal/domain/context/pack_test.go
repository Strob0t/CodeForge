package context_test

import (
	"strings"
	"testing"

	cfctx "github.com/Strob0t/CodeForge/internal/domain/context"
)

func validPack() *cfctx.ContextPack {
	return &cfctx.ContextPack{
		TaskID:      "task-1",
		ProjectID:   "proj-1",
		TokenBudget: 4096,
		Entries: []cfctx.ContextEntry{
			{Kind: cfctx.EntryFile, Path: "main.go", Content: "package main", Tokens: 3, Priority: 80},
		},
	}
}

func TestContextPack_Validate_Valid(t *testing.T) {
	if err := validPack().Validate(); err != nil {
		t.Fatalf("expected valid, got error: %v", err)
	}
}

func TestContextPack_Validate_MissingTaskID(t *testing.T) {
	p := validPack()
	p.TaskID = ""
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), "task_id") {
		t.Fatalf("expected task_id error, got: %v", err)
	}
}

func TestContextPack_Validate_MissingProjectID(t *testing.T) {
	p := validPack()
	p.ProjectID = ""
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf("expected project_id error, got: %v", err)
	}
}

func TestContextPack_Validate_ZeroBudget(t *testing.T) {
	p := validPack()
	p.TokenBudget = 0
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), "token_budget") {
		t.Fatalf("expected token_budget error, got: %v", err)
	}
}

func TestContextPack_Validate_NoEntries(t *testing.T) {
	p := validPack()
	p.Entries = nil
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), "entry") {
		t.Fatalf("expected entry error, got: %v", err)
	}
}

func TestContextPack_Validate_EmptyContent(t *testing.T) {
	p := validPack()
	p.Entries[0].Content = ""
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), "content") {
		t.Fatalf("expected content error, got: %v", err)
	}
}

func TestContextPack_Validate_InvalidKind(t *testing.T) {
	p := validPack()
	p.Entries[0].Kind = "unknown"
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid entry kind") {
		t.Fatalf("expected kind error, got: %v", err)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hi", 1},                         // 2 chars / 4 = 0 but len > 0 → 1
		{"hello", 1},                      // 5 / 4 = 1
		{"hello world this is a test", 6}, // 26 / 4 = 6
	}
	for _, tc := range tests {
		got := cfctx.EstimateTokens(tc.input)
		if got != tc.expected {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}

func TestValidEntryKind(t *testing.T) {
	valid := []cfctx.EntryKind{cfctx.EntryFile, cfctx.EntrySnippet, cfctx.EntrySummary, cfctx.EntryShared}
	for _, k := range valid {
		if !cfctx.ValidEntryKind(k) {
			t.Errorf("expected %q to be valid", k)
		}
	}
	if cfctx.ValidEntryKind("bogus") {
		t.Error("expected 'bogus' to be invalid")
	}
}
