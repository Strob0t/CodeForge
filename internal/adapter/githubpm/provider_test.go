package githubpm

import (
	"context"
	"os/exec"
	"testing"
)

func TestValidateProjectRef(t *testing.T) {
	tests := []struct {
		ref   string
		valid bool
	}{
		{"owner/repo", true},
		{"org/my-project", true},
		{"", false},
		{"noslash", false},
		{"/repo", false},
		{"owner/", false},
		{"a/b/c", false},
	}

	for _, tt := range tests {
		err := validateProjectRef(tt.ref)
		if tt.valid && err != nil {
			t.Errorf("expected %q to be valid, got error: %v", tt.ref, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("expected %q to be invalid, got nil error", tt.ref)
		}
	}
}

func TestIssueToItem(t *testing.T) {
	issue := &ghIssue{
		Number:    42,
		Title:     "Fix login bug",
		Body:      "The login form crashes",
		State:     "OPEN",
		Labels:    []ghLabel{{Name: "bug"}, {Name: "priority:high"}},
		Assignees: []ghUser{{Login: "alice"}},
	}

	item := issueToItem(issue, "owner/repo")

	if item.ID != "42" {
		t.Errorf("expected ID '42', got %q", item.ID)
	}
	if item.ExternalID != "owner/repo#42" {
		t.Errorf("expected ExternalID 'owner/repo#42', got %q", item.ExternalID)
	}
	if item.Title != "Fix login bug" {
		t.Errorf("expected title 'Fix login bug', got %q", item.Title)
	}
	if item.Status != "open" {
		t.Errorf("expected status 'open', got %q", item.Status)
	}
	if len(item.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(item.Labels))
	}
	if item.Assignee != "alice" {
		t.Errorf("expected assignee 'alice', got %q", item.Assignee)
	}
}

func TestIssueToItem_NoAssignee(t *testing.T) {
	issue := &ghIssue{
		Number: 1,
		Title:  "Test",
		State:  "CLOSED",
	}

	item := issueToItem(issue, "org/proj")

	if item.Assignee != "" {
		t.Errorf("expected empty assignee, got %q", item.Assignee)
	}
	if item.Status != "closed" {
		t.Errorf("expected status 'closed', got %q", item.Status)
	}
}

func TestProviderName(t *testing.T) {
	p := newProvider()
	if p.Name() != "github-issues" {
		t.Fatalf("expected name 'github-issues', got %q", p.Name())
	}
}

func TestProviderCapabilities(t *testing.T) {
	p := newProvider()
	caps := p.Capabilities()
	if !caps.ListItems || !caps.GetItem {
		t.Fatal("expected ListItems=true, GetItem=true")
	}
	if caps.CreateItem || caps.UpdateItem || caps.Webhooks {
		t.Fatal("expected CreateItem=false, UpdateItem=false, Webhooks=false")
	}
}

func TestListItems_InvalidRef(t *testing.T) {
	p := newProvider()
	_, err := p.ListItems(context.Background(), "invalid")
	if err == nil {
		t.Fatal("expected error for invalid project ref")
	}
}

func TestListItems_CommandConstruction(t *testing.T) {
	var capturedArgs []string
	p := &Provider{
		execCommand: func(_ context.Context, name string, args ...string) *exec.Cmd {
			capturedArgs = append([]string{name}, args...)
			// Return a command that outputs an empty JSON array.
			return exec.Command("echo", "[]")
		},
	}

	items, err := p.ListItems(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}

	// Verify command construction.
	expected := []string{"gh", "issue", "list", "--repo", "owner/repo", "--json", "number,title,body,state,labels,assignees", "--limit", "100"}
	if len(capturedArgs) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(capturedArgs), capturedArgs)
	}
	for i, exp := range expected {
		if capturedArgs[i] != exp {
			t.Errorf("arg[%d]: expected %q, got %q", i, exp, capturedArgs[i])
		}
	}
}

func TestGetItem_CommandConstruction(t *testing.T) {
	p := &Provider{
		execCommand: func(_ context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.Command("echo", `{"number":42,"title":"Test","body":"desc","state":"OPEN","labels":[],"assignees":[]}`)
		},
	}

	item, err := p.GetItem(context.Background(), "owner/repo", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.ID != "42" {
		t.Errorf("expected ID '42', got %q", item.ID)
	}
	if item.Title != "Test" {
		t.Errorf("expected title 'Test', got %q", item.Title)
	}
}

func TestGetItem_InvalidRef(t *testing.T) {
	p := newProvider()
	_, err := p.GetItem(context.Background(), "invalid", "1")
	if err == nil {
		t.Fatal("expected error for invalid project ref")
	}
}
