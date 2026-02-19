package gitlab

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
	if p.Name() != "gitlab" {
		t.Fatalf("expected 'gitlab', got %q", p.Name())
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
	issues := []gitlabIssue{
		{IID: 1, Title: "Bug fix", Description: "Fix the thing", State: "opened", Labels: []string{"bug"}},
		{IID: 2, Title: "Feature", Description: "Add feature", State: "opened"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "test-token" {
			t.Errorf("expected PRIVATE-TOKEN header, got %q", r.Header.Get("PRIVATE-TOKEN"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issues)
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "test-token")
	items, err := p.ListItems(context.Background(), "mygroup/myproject")
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
	issue := gitlabIssue{
		IID:         42,
		Title:       "Test Issue",
		Description: "Details here",
		State:       "closed",
		Assignees:   []gitlabUser{{Username: "alice"}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issue)
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "test-token")
	item, err := p.GetItem(context.Background(), "mygroup/myproject", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Title != "Test Issue" {
		t.Fatalf("expected 'Test Issue', got %q", item.Title)
	}
	if item.Status != "closed" {
		t.Fatalf("expected 'closed', got %q", item.Status)
	}
	if item.Assignee != "alice" {
		t.Fatalf("expected assignee 'alice', got %q", item.Assignee)
	}
}

func TestCreateItem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gitlabIssue{IID: 99, Title: "New Issue", State: "opened"})
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "test-token")
	result, err := p.CreateItem(context.Background(), "mygroup/myproject", &pmprovider.Item{
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

func TestUpdateItem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gitlabIssue{IID: 5, Title: "Updated", State: "closed"})
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "test-token")
	result, err := p.UpdateItem(context.Background(), "mygroup/myproject", &pmprovider.Item{
		ID:          "5",
		Title:       "Updated",
		Description: "Updated desc",
		Status:      "closed",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "closed" {
		t.Fatalf("expected 'closed', got %q", result.Status)
	}
}

func TestAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Project Not Found"}`))
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "test-token")
	_, err := p.ListItems(context.Background(), "nonexistent/project")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestMapStatusToStateEvent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"closed", "close"},
		{"Closed", "close"},
		{"opened", "reopen"},
		{"open", "reopen"},
		{"custom", "custom"},
	}
	for _, tt := range tests {
		got := mapStatusToStateEvent(tt.input)
		if got != tt.want {
			t.Errorf("mapStatusToStateEvent(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
