package githubpm

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
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

func TestGitHubPM_ProviderName(t *testing.T) {
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
	if !caps.CreateItem || !caps.UpdateItem {
		t.Fatal("expected CreateItem=true, UpdateItem=true")
	}
	if caps.Webhooks {
		t.Fatal("expected Webhooks=false")
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

func TestCreateItem_CommandConstruction(t *testing.T) {
	var capturedArgs []string
	p := &Provider{
		execCommand: func(_ context.Context, name string, args ...string) *exec.Cmd {
			capturedArgs = append([]string{name}, args...)
			// gh issue create prints the new issue URL.
			return exec.Command("echo", "https://github.com/owner/repo/issues/99")
		},
	}

	item := &pmprovider.Item{
		Title:       "New bug",
		Description: "Something broke",
		Labels:      []string{"bug"},
	}
	created, err := p.CreateItem(context.Background(), "owner/repo", item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.ID != "99" {
		t.Errorf("expected ID '99', got %q", created.ID)
	}
	if created.ExternalID != "owner/repo#99" {
		t.Errorf("expected ExternalID 'owner/repo#99', got %q", created.ExternalID)
	}

	// Verify command includes title, body, and label flags.
	argsStr := strings.Join(capturedArgs, " ")
	if !strings.Contains(argsStr, "--title") {
		t.Error("expected --title flag in command")
	}
	if !strings.Contains(argsStr, "--body") {
		t.Error("expected --body flag in command")
	}
	if !strings.Contains(argsStr, "--label") {
		t.Error("expected --label flag in command")
	}
}

func TestCreateItem_InvalidRef(t *testing.T) {
	p := newProvider()
	_, err := p.CreateItem(context.Background(), "invalid", &pmprovider.Item{Title: "test"})
	if err == nil {
		t.Fatal("expected error for invalid project ref")
	}
}

func TestUpdateItem_CommandConstruction(t *testing.T) {
	var capturedArgs []string
	p := &Provider{
		execCommand: func(_ context.Context, name string, args ...string) *exec.Cmd {
			capturedArgs = append([]string{name}, args...)
			return exec.Command("true")
		},
	}

	item := &pmprovider.Item{
		ID:          "42",
		Title:       "Updated title",
		Description: "Updated body",
	}
	result, err := p.UpdateItem(context.Background(), "owner/repo", item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "42" {
		t.Errorf("expected ID '42', got %q", result.ID)
	}

	argsStr := strings.Join(capturedArgs, " ")
	if !strings.Contains(argsStr, "issue edit 42") {
		t.Errorf("expected 'issue edit 42' in command, got: %s", argsStr)
	}
}

func TestUpdateItem_MissingID(t *testing.T) {
	p := newProvider()
	_, err := p.UpdateItem(context.Background(), "owner/repo", &pmprovider.Item{Title: "test"})
	if err == nil {
		t.Fatal("expected error for missing item ID")
	}
}

func TestUpdateItem_InvalidRef(t *testing.T) {
	p := newProvider()
	_, err := p.UpdateItem(context.Background(), "invalid", &pmprovider.Item{ID: "1", Title: "test"})
	if err == nil {
		t.Fatal("expected error for invalid project ref")
	}
}
