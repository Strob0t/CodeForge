package gitea

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

// Compile-time interface check.
var _ pmprovider.Provider = (*Provider)(nil)

func TestProviderName(t *testing.T) {
	p := NewProvider("http://localhost", "")
	if p.Name() != "gitea" {
		t.Fatalf("expected 'gitea', got %q", p.Name())
	}
}

func TestCapabilities(t *testing.T) {
	p := NewProvider("http://localhost", "")
	caps := p.Capabilities()
	if !caps.ListItems {
		t.Fatal("expected ListItems=true")
	}
	if !caps.GetItem {
		t.Fatal("expected GetItem=true")
	}
	if !caps.CreateItem {
		t.Fatal("expected CreateItem=true")
	}
	if !caps.UpdateItem {
		t.Fatal("expected UpdateItem=true")
	}
}

func TestListItems(t *testing.T) {
	issues := []giteaIssue{
		{Number: 1, Title: "Bug fix", Body: "Fix the thing", State: "open", Labels: []giteaLabel{{Name: "bug"}}},
		{Number: 2, Title: "Feature", Body: "Add feature", State: "open"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issues)
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "test-token")
	items, err := p.ListItems(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Title != "Bug fix" {
		t.Fatalf("expected 'Bug fix', got %q", items[0].Title)
	}
	if len(items[0].Labels) != 1 || items[0].Labels[0] != "bug" {
		t.Fatalf("expected label 'bug', got %v", items[0].Labels)
	}
}

func TestGetItem(t *testing.T) {
	issue := giteaIssue{Number: 42, Title: "Test Issue", Body: "Details here", State: "closed"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issue)
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "test-token")
	item, err := p.GetItem(context.Background(), "owner/repo", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Title != "Test Issue" {
		t.Fatalf("expected 'Test Issue', got %q", item.Title)
	}
	if item.Status != "closed" {
		t.Fatalf("expected 'closed', got %q", item.Status)
	}
}

func TestCreateItem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(giteaIssue{Number: 99, Title: "New Issue", State: "open"})
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "test-token")
	result, err := p.CreateItem(context.Background(), "owner/repo", &pmprovider.Item{
		Title:       "New Issue",
		Description: "Some description",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "99" {
		t.Fatalf("expected ID '99', got %q", result.ID)
	}
}

func TestInvalidProjectRef(t *testing.T) {
	p := NewProvider("http://localhost", "")
	_, err := p.ListItems(context.Background(), "invalid-ref")
	if err == nil {
		t.Fatal("expected error for invalid project ref")
	}
}

func TestParseProjectRef(t *testing.T) {
	tests := []struct {
		ref       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{"owner/repo", "owner", "repo", false},
		{"org/project", "org", "project", false},
		{"invalid", "", "", true},
		{"/repo", "", "", true},
		{"owner/", "", "", true},
	}
	for _, tt := range tests {
		owner, repo, err := parseProjectRef(tt.ref)
		if tt.wantErr && err == nil {
			t.Errorf("parseProjectRef(%q): expected error", tt.ref)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("parseProjectRef(%q): unexpected error: %v", tt.ref, err)
		}
		if owner != tt.wantOwner || repo != tt.wantRepo {
			t.Errorf("parseProjectRef(%q) = (%q, %q), want (%q, %q)", tt.ref, owner, repo, tt.wantOwner, tt.wantRepo)
		}
	}
}
