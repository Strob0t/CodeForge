package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestName(t *testing.T) {
	p := NewProvider("tok", "")
	if got := p.Name(); got != "github-api" {
		t.Fatalf("Name() = %q, want %q", got, "github-api")
	}
}

func TestCapabilities(t *testing.T) {
	p := NewProvider("tok", "")
	c := p.Capabilities()
	if !c.Clone || !c.Push || !c.PullRequest || !c.Webhook || !c.Issues {
		t.Fatalf("unexpected capabilities: %+v", c)
	}
}

func TestCloneURL(t *testing.T) {
	p := NewProvider("my-token", "")
	url, err := p.CloneURL(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("CloneURL() error: %v", err)
	}
	expected := "https://x-access-token:my-token@github.com/owner/repo.git"
	if url != expected {
		t.Fatalf("CloneURL() = %q, want %q", url, expected)
	}
}

func TestCloneURL_Empty(t *testing.T) {
	p := NewProvider("tok", "")
	_, err := p.CloneURL(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty repo")
	}
}

func TestCloneURL_CustomBaseURL(t *testing.T) {
	p := NewProvider("tok", "https://ghe.example.com/api/v3")
	url, err := p.CloneURL(context.Background(), "org/repo")
	if err != nil {
		t.Fatalf("CloneURL() error: %v", err)
	}
	expected := "https://x-access-token:tok@ghe.example.com/org/repo.git"
	if url != expected {
		t.Fatalf("CloneURL() = %q, want %q", url, expected)
	}
}

func TestListRepos(t *testing.T) {
	repos := []ghRepo{
		{FullName: "owner/repo1"},
		{FullName: "owner/repo2"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos)
	}))
	defer srv.Close()

	p := NewProvider("test-token", srv.URL)
	got, err := p.ListRepos(context.Background())
	if err != nil {
		t.Fatalf("ListRepos() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListRepos() returned %d repos, want 2", len(got))
	}
	if got[0] != "owner/repo1" || got[1] != "owner/repo2" {
		t.Fatalf("ListRepos() = %v, want [owner/repo1, owner/repo2]", got)
	}
}

func TestListRepos_Pagination(t *testing.T) {
	page1 := []ghRepo{{FullName: "a/1"}}
	page2 := []ghRepo{{FullName: "b/2"}}

	// We need srv.URL inside the handler, so use a pointer to hold it.
	var srvURL string
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			w.Header().Set("Link", `<`+srvURL+`/page2>; rel="next"`)
			_ = json.NewEncoder(w).Encode(page1)
		} else {
			_ = json.NewEncoder(w).Encode(page2)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	p := NewProvider("tok", srv.URL)
	got, err := p.ListRepos(context.Background())
	if err != nil {
		t.Fatalf("ListRepos() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListRepos() returned %d repos, want 2", len(got))
	}
	if got[0] != "a/1" || got[1] != "b/2" {
		t.Fatalf("ListRepos() = %v, want [a/1, b/2]", got)
	}
}

func TestParseLinkNext(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"", ""},
		{`<https://api.github.com/repos?page=2>; rel="next"`, "https://api.github.com/repos?page=2"},
		{`<https://api.github.com/repos?page=1>; rel="prev", <https://api.github.com/repos?page=3>; rel="next"`, "https://api.github.com/repos?page=3"},
		{`<https://api.github.com/repos?page=1>; rel="prev"`, ""},
	}
	for _, tt := range tests {
		got := parseLinkNext(tt.header)
		if got != tt.want {
			t.Errorf("parseLinkNext(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}
