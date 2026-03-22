package plane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

func TestProviderName(t *testing.T) {
	p := &Provider{apiToken: "test-token", baseURL: "http://localhost"}
	if got := p.Name(); got != "plane" {
		t.Errorf("Name() = %q, want %q", got, "plane")
	}
}

func TestCapabilities(t *testing.T) {
	p := &Provider{apiToken: "test-token", baseURL: "http://localhost"}
	caps := p.Capabilities()

	if !caps.ListItems {
		t.Error("Capabilities().ListItems should be true")
	}
	if !caps.GetItem {
		t.Error("Capabilities().GetItem should be true")
	}
	if !caps.CreateItem {
		t.Error("Capabilities().CreateItem should be true")
	}
	if !caps.UpdateItem {
		t.Error("Capabilities().UpdateItem should be true")
	}
	if !caps.Webhooks {
		t.Error("Capabilities().Webhooks should be true")
	}
}

func TestValidateProjectRef(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantErr bool
	}{
		{name: "valid ref", ref: "my-workspace/project-id", wantErr: false},
		{name: "valid with hyphens", ref: "acme-corp/abc-123-def", wantErr: false},
		{name: "empty string", ref: "", wantErr: true},
		{name: "no slash", ref: "just-a-workspace", wantErr: true},
		{name: "trailing slash", ref: "workspace/", wantErr: true},
		{name: "leading slash", ref: "/project-id", wantErr: true},
		{name: "too many slashes", ref: "a/b/c", wantErr: true},
		{name: "only slash", ref: "/", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseProjectRef(tc.ref)
			if (err != nil) != tc.wantErr {
				t.Errorf("parseProjectRef(%q) error = %v, wantErr %v", tc.ref, err, tc.wantErr)
			}
		})
	}
}

// planeIssue mirrors the Plane API issue JSON for test helpers.
type planeIssue struct {
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	DescriptionStripped string             `json:"description_stripped"`
	StateDetail         planeStateDetail   `json:"state_detail"`
	LabelDetails        []planeLabelDetail `json:"label_details"`
	AssigneeDetails     []planeAssignee    `json:"assignee_details"`
	Priority            string             `json:"priority"`
}

type planeStateDetail struct {
	Group string `json:"group"`
}

type planeLabelDetail struct {
	Name string `json:"name"`
}

type planeAssignee struct {
	DisplayName string `json:"display_name"`
}

type planeListResponse struct {
	Results         []planeIssue `json:"results"`
	NextCursor      string       `json:"next_cursor"`
	NextPageResults bool         `json:"next_page_results"`
}

func newTestProvider(t *testing.T, serverURL string) *Provider {
	t.Helper()
	return &Provider{
		baseURL:    serverURL,
		apiToken:   "test-api-key",
		httpClient: &http.Client{},
	}
}

func TestListItems_Success(t *testing.T) {
	issues := []planeIssue{
		{
			ID:                  "issue-001",
			Name:                "Fix login bug",
			DescriptionStripped: "Users cannot login",
			StateDetail:         planeStateDetail{Group: "started"},
			LabelDetails:        []planeLabelDetail{{Name: "bug"}, {Name: "urgent"}},
			AssigneeDetails:     []planeAssignee{{DisplayName: "alice"}},
			Priority:            "high",
		},
		{
			ID:                  "issue-002",
			Name:                "Add dark mode",
			DescriptionStripped: "Theme support",
			StateDetail:         planeStateDetail{Group: "backlog"},
			LabelDetails:        []planeLabelDetail{{Name: "enhancement"}},
			AssigneeDetails:     nil,
			Priority:            "low",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if got := r.Header.Get("X-API-Key"); got != "test-api-key" {
			t.Errorf("X-API-Key = %q, want %q", got, "test-api-key")
		}
		wantPath := "/api/v1/workspaces/my-workspace/projects/proj-1/issues/"
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}

		resp := planeListResponse{Results: issues, NextCursor: "", NextPageResults: false}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	items, err := p.ListItems(context.Background(), "my-workspace/proj-1")
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}

	// First item checks
	if items[0].ID != "issue-001" {
		t.Errorf("items[0].ID = %q, want %q", items[0].ID, "issue-001")
	}
	if items[0].Title != "Fix login bug" {
		t.Errorf("items[0].Title = %q, want %q", items[0].Title, "Fix login bug")
	}
	if items[0].Description != "Users cannot login" {
		t.Errorf("items[0].Description = %q, want %q", items[0].Description, "Users cannot login")
	}
	if items[0].Status != "in_progress" {
		t.Errorf("items[0].Status = %q, want %q", items[0].Status, "in_progress")
	}
	if len(items[0].Labels) != 2 || items[0].Labels[0] != "bug" || items[0].Labels[1] != "urgent" {
		t.Errorf("items[0].Labels = %v, want [bug urgent]", items[0].Labels)
	}
	if items[0].Assignee != "alice" {
		t.Errorf("items[0].Assignee = %q, want %q", items[0].Assignee, "alice")
	}
	if items[0].Priority != "high" {
		t.Errorf("items[0].Priority = %q, want %q", items[0].Priority, "high")
	}
	if items[0].ExternalID != "my-workspace/proj-1#issue-001" {
		t.Errorf("items[0].ExternalID = %q, want %q", items[0].ExternalID, "my-workspace/proj-1#issue-001")
	}

	// Second item: no assignee
	if items[1].Assignee != "" {
		t.Errorf("items[1].Assignee = %q, want empty", items[1].Assignee)
	}
	if items[1].Status != "backlog" {
		t.Errorf("items[1].Status = %q, want %q", items[1].Status, "backlog")
	}
}

func TestListItems_CursorPagination(t *testing.T) {
	page1Issues := []planeIssue{
		{ID: "issue-a", Name: "First", StateDetail: planeStateDetail{Group: "started"}},
	}
	page2Issues := []planeIssue{
		{ID: "issue-b", Name: "Second", StateDetail: planeStateDetail{Group: "completed"}},
	}

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		cursor := r.URL.Query().Get("cursor")
		w.Header().Set("Content-Type", "application/json")

		switch cursor {
		case "":
			resp := planeListResponse{Results: page1Issues, NextCursor: "cursor-page2", NextPageResults: true}
			_ = json.NewEncoder(w).Encode(resp)
		case "cursor-page2":
			resp := planeListResponse{Results: page2Issues, NextCursor: "", NextPageResults: false}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Errorf("unexpected cursor: %q", cursor)
			http.Error(w, "bad cursor", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	items, err := p.ListItems(context.Background(), "ws/proj")
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].ID != "issue-a" {
		t.Errorf("items[0].ID = %q, want %q", items[0].ID, "issue-a")
	}
	if items[1].ID != "issue-b" {
		t.Errorf("items[1].ID = %q, want %q", items[1].ID, "issue-b")
	}
}

func TestListItems_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := planeListResponse{Results: []planeIssue{}, NextCursor: "", NextPageResults: false}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	items, err := p.ListItems(context.Background(), "ws/proj")
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if len(items) != 0 {
		t.Errorf("got %d items, want 0", len(items))
	}
}

func TestListItems_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	_, err := p.ListItems(context.Background(), "ws/proj")
	if err == nil {
		t.Fatal("ListItems() expected error for 500 response, got nil")
	}
}

func TestListItems_InvalidProjectRef(t *testing.T) {
	p := &Provider{apiToken: "tok", baseURL: "http://localhost", httpClient: &http.Client{}}
	_, err := p.ListItems(context.Background(), "bad-ref")
	if err == nil {
		t.Fatal("ListItems() expected error for invalid project ref, got nil")
	}
}

func TestGetItem_Success(t *testing.T) {
	issue := planeIssue{
		ID:                  "issue-42",
		Name:                "Implement feature X",
		DescriptionStripped: "Details here",
		StateDetail:         planeStateDetail{Group: "unstarted"},
		LabelDetails:        []planeLabelDetail{{Name: "feature"}},
		AssigneeDetails:     []planeAssignee{{DisplayName: "bob"}},
		Priority:            "medium",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		wantPath := "/api/v1/workspaces/ws/projects/proj/issues/issue-42/"
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issue)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	item, err := p.GetItem(context.Background(), "ws/proj", "issue-42")
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}

	if item.ID != "issue-42" {
		t.Errorf("ID = %q, want %q", item.ID, "issue-42")
	}
	if item.Title != "Implement feature X" {
		t.Errorf("Title = %q, want %q", item.Title, "Implement feature X")
	}
	if item.Status != "planned" {
		t.Errorf("Status = %q, want %q", item.Status, "planned")
	}
	if item.Assignee != "bob" {
		t.Errorf("Assignee = %q, want %q", item.Assignee, "bob")
	}
	if item.Priority != "medium" {
		t.Errorf("Priority = %q, want %q", item.Priority, "medium")
	}
	if item.ExternalID != "ws/proj#issue-42" {
		t.Errorf("ExternalID = %q, want %q", item.ExternalID, "ws/proj#issue-42")
	}
}

func TestGetItem_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	_, err := p.GetItem(context.Background(), "ws/proj", "nonexistent")
	if err == nil {
		t.Fatal("GetItem() expected error for 404 response, got nil")
	}
}

func TestCreateItem_Success(t *testing.T) {
	createdIssue := planeIssue{
		ID:          "new-issue-id",
		Name:        "New feature",
		StateDetail: planeStateDetail{Group: "backlog"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		wantPath := "/api/v1/workspaces/ws/projects/proj/issues/"
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		// Decode the request body to verify it was sent correctly
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body["name"] != "New feature" {
			t.Errorf("body.name = %v, want %q", body["name"], "New feature")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createdIssue)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	item := &pmprovider.Item{
		Title:       "New feature",
		Description: "Implement this new feature",
	}
	created, err := p.CreateItem(context.Background(), "ws/proj", item)
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}

	if created.ID != "new-issue-id" {
		t.Errorf("ID = %q, want %q", created.ID, "new-issue-id")
	}
	if created.Title != "New feature" {
		t.Errorf("Title = %q, want %q", created.Title, "New feature")
	}
}

func TestCreateItem_ValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"detail":"name is required"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	_, err := p.CreateItem(context.Background(), "ws/proj", &pmprovider.Item{})
	if err == nil {
		t.Fatal("CreateItem() expected error for 400 response, got nil")
	}
}

func TestUpdateItem_Success(t *testing.T) {
	updatedIssue := planeIssue{
		ID:          "issue-99",
		Name:        "Updated title",
		StateDetail: planeStateDetail{Group: "completed"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		wantPath := "/api/v1/workspaces/ws/projects/proj/issues/issue-99/"
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updatedIssue)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	item := &pmprovider.Item{
		ID:    "issue-99",
		Title: "Updated title",
	}
	updated, err := p.UpdateItem(context.Background(), "ws/proj", item)
	if err != nil {
		t.Fatalf("UpdateItem() error = %v", err)
	}

	if updated.ID != "issue-99" {
		t.Errorf("ID = %q, want %q", updated.ID, "issue-99")
	}
	if updated.Title != "Updated title" {
		t.Errorf("Title = %q, want %q", updated.Title, "Updated title")
	}
	if updated.Status != "done" {
		t.Errorf("Status = %q, want %q", updated.Status, "done")
	}
}

func TestUpdateItem_MissingID(t *testing.T) {
	p := &Provider{apiToken: "tok", baseURL: "http://localhost", httpClient: &http.Client{}}
	_, err := p.UpdateItem(context.Background(), "ws/proj", &pmprovider.Item{Title: "no id"})
	if err == nil {
		t.Fatal("UpdateItem() expected error when item.ID is empty, got nil")
	}
}

func TestStatusMapping(t *testing.T) {
	tests := []struct {
		group string
		want  string
	}{
		{group: "backlog", want: "backlog"},
		{group: "unstarted", want: "planned"},
		{group: "started", want: "in_progress"},
		{group: "completed", want: "done"},
		{group: "cancelled", want: "cancelled"},
		{group: "unknown-state", want: "unknown-state"},
		{group: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("group_%s", tc.group), func(t *testing.T) {
			got := mapStatus(tc.group)
			if got != tc.want {
				t.Errorf("mapStatus(%q) = %q, want %q", tc.group, got, tc.want)
			}
		})
	}
}

func TestNewProvider_FromConfig(t *testing.T) {
	config := map[string]string{
		"api_token": "my-secret-token",
		"base_url":  "https://custom.plane.example.com",
	}
	p, err := newProvider(config)
	if err != nil {
		t.Fatalf("newProvider() error = %v", err)
	}
	if p.apiToken != "my-secret-token" {
		t.Errorf("apiToken = %q, want %q", p.apiToken, "my-secret-token")
	}
	if p.baseURL != "https://custom.plane.example.com" {
		t.Errorf("baseURL = %q, want %q", p.baseURL, "https://custom.plane.example.com")
	}
	if p.httpClient == nil {
		t.Error("httpClient should not be nil")
	}
}

func TestNewProvider_DefaultBaseURL(t *testing.T) {
	config := map[string]string{
		"api_token": "my-token",
	}
	p, err := newProvider(config)
	if err != nil {
		t.Fatalf("newProvider() error = %v", err)
	}
	if p.baseURL != "https://api.plane.so" {
		t.Errorf("baseURL = %q, want %q", p.baseURL, "https://api.plane.so")
	}
}

func TestNewProvider_MissingToken(t *testing.T) {
	// Clear env var to ensure no fallback
	t.Setenv("CODEFORGE_PLANE_API_TOKEN", "")

	_, err := newProvider(map[string]string{})
	if err == nil {
		t.Fatal("newProvider() expected error when no token, got nil")
	}
}

func TestNewProvider_TokenFromConfig(t *testing.T) {
	// Token is now provided via config map (sourced from cfg.Plane.APIToken
	// which reads CODEFORGE_PLANE_API_TOKEN via the config loader).
	p, err := newProvider(map[string]string{"api_token": "config-token-value"})
	if err != nil {
		t.Fatalf("newProvider() error = %v", err)
	}
	if p.apiToken != "config-token-value" {
		t.Errorf("apiToken = %q, want %q", p.apiToken, "config-token-value")
	}
}

func TestNewProvider_TrailingSlashTrimmed(t *testing.T) {
	config := map[string]string{
		"api_token": "tok",
		"base_url":  "https://api.plane.so/",
	}
	p, err := newProvider(config)
	if err != nil {
		t.Fatalf("newProvider() error = %v", err)
	}
	if p.baseURL != "https://api.plane.so" {
		t.Errorf("baseURL = %q, want trailing slash trimmed", p.baseURL)
	}
}

func TestListItems_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// This handler should not be reached if context is canceled
		resp := planeListResponse{Results: []planeIssue{}, NextCursor: "", NextPageResults: false}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := p.ListItems(ctx, "ws/proj")
	if err == nil {
		t.Fatal("ListItems() expected error for canceled context, got nil")
	}
}

// TestProviderImplementsInterface verifies at compile time that *Provider satisfies pmprovider.Provider.
var _ pmprovider.Provider = (*Provider)(nil)
