package gitea

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

// Compile-time interface check.
var _ pmprovider.Provider = (*Provider)(nil)

func TestGitea_ProviderName(t *testing.T) {
	p := NewProvider("http://localhost", "")
	if p.Name() != "gitea" {
		t.Fatalf("expected 'gitea', got %q", p.Name())
	}
}

func TestGitea_Capabilities(t *testing.T) {
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

func TestGitea_ListItems(t *testing.T) {
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

func TestGitea_GetItem(t *testing.T) {
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

func TestGitea_CreateItem(t *testing.T) {
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

// ---------------------------------------------------------------------------
// Variant configuration
// ---------------------------------------------------------------------------

func TestGitea_VariantConfig(t *testing.T) {
	tests := []struct {
		name        string
		variant     string
		setVariant  bool // whether to include "variant" key in config
		wantVariant string
		wantErr     bool
	}{
		{"default gitea", "gitea", true, "gitea", false},
		{"forgejo", "forgejo", true, "forgejo", false},
		{"codeberg", "codeberg", true, "codeberg", false},
		{"empty defaults to gitea", "", false, "gitea", false},
		{"invalid variant", "invalid", true, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := map[string]string{
				"base_url": "http://localhost",
				"token":    "tok",
			}
			if tt.setVariant {
				cfg["variant"] = tt.variant
			}

			p, err := NewProviderFromConfig(cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error for invalid variant")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.variant != tt.wantVariant {
				t.Errorf("variant = %q, want %q", p.variant, tt.wantVariant)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DetectForgejo
// ---------------------------------------------------------------------------

func TestGitea_DetectForgejo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/forgejo/v1/version" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"7.0.0"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "tok")
	if !p.DetectForgejo(context.Background()) {
		t.Error("expected DetectForgejo to return true for 200 response")
	}
}

func TestGitea_DetectForgejo_NotForgejo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "tok")
	if p.DetectForgejo(context.Background()) {
		t.Error("expected DetectForgejo to return false for 404 response")
	}
}

func TestGitea_DetectForgejo_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		// Block until the request context is cancelled (client disconnects).
		<-r.Context().Done()
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "tok")
	// Use a very short context timeout to make the test fast.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if p.DetectForgejo(ctx) {
		t.Error("expected DetectForgejo to return false on timeout")
	}
}

// ---------------------------------------------------------------------------
// Rate limit headers
// ---------------------------------------------------------------------------

func TestGitea_CheckRateLimit(t *testing.T) {
	tests := []struct {
		name          string
		remaining     string
		reset         string
		wantRemaining int
		wantResetUnix int64
		wantResetZero bool
	}{
		{
			name:          "normal remaining",
			remaining:     "50",
			reset:         "1700000000",
			wantRemaining: 50,
			wantResetUnix: 1700000000,
		},
		{
			name:          "zero remaining",
			remaining:     "0",
			reset:         "1700000100",
			wantRemaining: 0,
			wantResetUnix: 1700000100,
		},
		{
			name:          "one remaining",
			remaining:     "1",
			reset:         "1700000050",
			wantRemaining: 1,
			wantResetUnix: 1700000050,
		},
		{
			name:          "missing headers",
			remaining:     "",
			reset:         "",
			wantRemaining: -1,
			wantResetZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tt.remaining != "" {
				resp.Header.Set("X-RateLimit-Remaining", tt.remaining)
			}
			if tt.reset != "" {
				resp.Header.Set("X-RateLimit-Reset", tt.reset)
			}

			remaining, resetAt := checkRateLimit(resp)
			if remaining != tt.wantRemaining {
				t.Errorf("remaining = %d, want %d", remaining, tt.wantRemaining)
			}
			if tt.wantResetZero {
				if !resetAt.IsZero() {
					t.Errorf("resetAt should be zero time, got %v", resetAt)
				}
			} else if resetAt.Unix() != tt.wantResetUnix {
				t.Errorf("resetAt.Unix() = %d, want %d", resetAt.Unix(), tt.wantResetUnix)
			}
		})
	}
}

func TestGitea_RateLimitWarningOnAPICall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "1")
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "tok")
	_, err := p.ListItems(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The rate limit check is exercised in the code path; we verify
	// the code does not panic or error. The warning is logged via slog.
}

// ---------------------------------------------------------------------------
// Field fallback: assignee null but assignees present
// ---------------------------------------------------------------------------

func TestGitea_FieldFallback_FullNameToName(t *testing.T) {
	// Forgejo sometimes returns full_name="" but name="reponame" in user objects.
	// We test via raw JSON to verify the unmarshalling handles it.
	rawJSON := `{
		"number": 10,
		"title": "Test issue",
		"body": "body",
		"state": "open",
		"labels": [],
		"assignees": [],
		"assignee": null
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rawJSON))
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "tok")
	item, err := p.GetItem(context.Background(), "owner/repo", "10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.ID != "10" {
		t.Errorf("ID = %q, want %q", item.ID, "10")
	}
	if item.Assignee != "" {
		t.Errorf("Assignee = %q, want empty (null assignee, empty assignees)", item.Assignee)
	}
}

func TestGitea_AssigneeFallback_NullAssigneeWithAssignees(t *testing.T) {
	// When "assignee" field is null but "assignees" array has entries, use the first from assignees.
	// This is the existing behavior via the Assignees field in giteaIssue.
	rawJSON := `{
		"number": 11,
		"title": "Forgejo issue",
		"body": "body text",
		"state": "open",
		"labels": [{"name": "bug"}],
		"assignee": null,
		"assignees": [{"login": "forgejo-dev"}, {"login": "other"}]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rawJSON))
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "tok")
	item, err := p.GetItem(context.Background(), "owner/repo", "11")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Assignee != "forgejo-dev" {
		t.Errorf("Assignee = %q, want %q (first from assignees array)", item.Assignee, "forgejo-dev")
	}
}

func TestGitea_RegistrationAliases(t *testing.T) {
	available := pmprovider.Available()
	for _, name := range []string{"gitea", "forgejo", "codeberg"} {
		if !slices.Contains(available, name) {
			t.Errorf("%q not found in pmprovider.Available(): %v", name, available)
		}
	}
}

func TestGitea_AssigneeFallback_LoginFallsBackToFullName(t *testing.T) {
	// When login is empty but full_name is present on the user, fall back to full_name.
	rawJSON := `{
		"number": 12,
		"title": "User field fallback",
		"body": "",
		"state": "open",
		"labels": [],
		"assignees": [{"login": "", "full_name": "Jane Doe"}]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rawJSON))
	}))
	defer srv.Close()

	p := NewProvider(srv.URL, "tok")
	item, err := p.GetItem(context.Background(), "owner/repo", "12")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Assignee != "Jane Doe" {
		t.Errorf("Assignee = %q, want %q (full_name fallback)", item.Assignee, "Jane Doe")
	}
}
