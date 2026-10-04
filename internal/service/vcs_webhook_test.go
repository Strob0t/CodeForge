package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/webhook"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// tenantBroadcaster records each event with the tenant it is sent in.
type tenantBroadcaster struct {
	mu      sync.Mutex
	types   []string
	tenants []string
}

func (b *tenantBroadcaster) BroadcastEvent(ctx context.Context, eventType string, _ any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.types = append(b.types, eventType)
	b.tenants = append(b.tenants, tenantctx.FromContext(ctx))
}

func (b *tenantBroadcaster) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.types)
}

// tenantsOf returns the tenants the events of eventType were sent in.
func (b *tenantBroadcaster) tenantsOf(eventType string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var tenants []string
	for i, t := range b.types {
		if t == eventType {
			tenants = append(tenants, b.tenants[i])
		}
	}
	return tenants
}

var (
	vcsGitHubProject = &project.Project{ID: "p-gh", RepoURL: "https://github.com/owner/repo.git"}
	vcsGitLabProject = &project.Project{ID: "p-gl", RepoURL: "git@gitlab.com:group/project.git"}
)

func TestHandleGitHubPush(t *testing.T) {
	bc := &tenantBroadcaster{}
	svc := NewVCSWebhookService(bc)

	payload := []byte(`{
		"ref": "refs/heads/main",
		"before": "aaa",
		"after": "bbb",
		"forced": false,
		"repository": {"full_name": "owner/repo", "html_url": "https://github.com/owner/repo"},
		"sender": {"login": "user1"},
		"commits": [
			{
				"id": "bbb",
				"message": "fix bug",
				"author": {"name": "User One"},
				"added": ["new.txt"],
				"modified": ["old.txt"],
				"removed": []
			}
		]
	}`)

	ev, err := svc.HandleGitHubPush(tenantctx.WithTenant(context.Background(), otherTenantID), vcsGitHubProject, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Repository != "owner/repo" || ev.ProjectID != "p-gh" {
		t.Fatalf("event repository %q project %q", ev.Repository, ev.ProjectID)
	}
	if ev.Branch != "main" {
		t.Fatalf("expected 'main', got %q", ev.Branch)
	}
	if len(ev.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(ev.Commits))
	}
	if ev.FileCount != 2 {
		t.Fatalf("expected 2 files, got %d", ev.FileCount)
	}
	if bc.count() != 1 || bc.tenants[0] != otherTenantID {
		t.Fatalf("broadcasts %v in tenants %v, want one in the webhook's tenant", bc.types, bc.tenants)
	}
}

func TestHandleGitLabPush(t *testing.T) {
	bc := &tenantBroadcaster{}
	svc := NewVCSWebhookService(bc)

	payload := []byte(`{
		"ref": "refs/heads/develop",
		"before": "ccc",
		"after": "ddd",
		"project": {"path_with_namespace": "group/project", "web_url": "https://gitlab.com/group/project"},
		"user_username": "dev1",
		"commits": [
			{
				"id": "ddd",
				"message": "add feature",
				"author": {"name": "Dev One"},
				"added": ["feature.go"],
				"modified": [],
				"removed": []
			}
		]
	}`)

	ev, err := svc.HandleGitLabPush(defaultTenantCtx(), vcsGitLabProject, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Provider != "gitlab" || ev.ProjectID != "p-gl" {
		t.Fatalf("event provider %q project %q", ev.Provider, ev.ProjectID)
	}
	if ev.Branch != "develop" {
		t.Fatalf("expected 'develop', got %q", ev.Branch)
	}
}

func TestHandleGitHubPullRequest(t *testing.T) {
	bc := &tenantBroadcaster{}
	svc := NewVCSWebhookService(bc)

	payload := []byte(`{
		"action": "opened",
		"pull_request": {
			"number": 42,
			"title": "Add new feature",
			"draft": false,
			"head": {"ref": "feature/x", "sha": "abc123"},
			"base": {"ref": "main"}
		},
		"repository": {"full_name": "owner/repo"},
		"sender": {"login": "contributor"}
	}`)

	ev, err := svc.HandleGitHubPullRequest(defaultTenantCtx(), vcsGitHubProject, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.PRNumber != 42 {
		t.Fatalf("expected PR 42, got %d", ev.PRNumber)
	}
	if ev.Action != "opened" {
		t.Fatalf("expected 'opened', got %q", ev.Action)
	}
	if ev.HeadBranch != "feature/x" {
		t.Fatalf("expected 'feature/x', got %q", ev.HeadBranch)
	}
}

// KI-85 (D3): a VCS event acts on the webhook's project only when it names
// the project's repository exactly (host and full path, case-insensitive);
// the old lookup matched any project whose URL contained the name.
func TestVCSWebhook_EventMustBeForTheProjectsRepository(t *testing.T) {
	tests := []struct {
		name  string
		push  func(s *VCSWebhookService) error
		match bool
	}{
		{"github same repository", func(s *VCSWebhookService) error {
			_, err := s.HandleGitHubPush(defaultTenantCtx(), vcsGitHubProject, []byte(`{"ref":"refs/heads/main","repository":{"full_name":"owner/repo"}}`))
			return err
		}, true},
		{"github other case", func(s *VCSWebhookService) error {
			_, err := s.HandleGitHubPush(defaultTenantCtx(), vcsGitHubProject, []byte(`{"ref":"refs/heads/main","repository":{"full_name":"Owner/Repo"}}`))
			return err
		}, true},
		{"github substring of the name", func(s *VCSWebhookService) error {
			_, err := s.HandleGitHubPush(defaultTenantCtx(), vcsGitHubProject, []byte(`{"ref":"refs/heads/main","repository":{"full_name":"owner/rep"}}`))
			return err
		}, false},
		{"github longer name", func(s *VCSWebhookService) error {
			_, err := s.HandleGitHubPush(defaultTenantCtx(), vcsGitHubProject, []byte(`{"ref":"refs/heads/main","repository":{"full_name":"owner/repo-fork"}}`))
			return err
		}, false},
		{"github another host", func(s *VCSWebhookService) error {
			_, err := s.HandleGitHubPush(defaultTenantCtx(), vcsGitHubProject, []byte(`{"ref":"refs/heads/main","repository":{"full_name":"owner/repo","html_url":"https://ghe.example.com/owner/repo"}}`))
			return err
		}, false},
		{"github pull request for another repository", func(s *VCSWebhookService) error {
			_, err := s.HandleGitHubPullRequest(defaultTenantCtx(), vcsGitHubProject, []byte(`{"action":"opened","repository":{"full_name":"evil/repo"}}`))
			return err
		}, false},
		{"gitlab ssh project URL", func(s *VCSWebhookService) error {
			_, err := s.HandleGitLabPush(defaultTenantCtx(), vcsGitLabProject, []byte(`{"ref":"refs/heads/main","project":{"path_with_namespace":"group/project"}}`))
			return err
		}, true},
		{"gitlab subgroup of the project path", func(s *VCSWebhookService) error {
			_, err := s.HandleGitLabPush(defaultTenantCtx(), vcsGitLabProject, []byte(`{"ref":"refs/heads/main","project":{"path_with_namespace":"group/project/sub"}}`))
			return err
		}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bc := &tenantBroadcaster{}
			err := tc.push(NewVCSWebhookService(bc))
			if tc.match {
				if err != nil || bc.count() != 1 {
					t.Fatalf("err %v, %d broadcasts; want the event handled", err, bc.count())
				}
				return
			}
			if !errors.Is(err, webhook.ErrRepositoryMismatch) || bc.count() != 0 {
				t.Fatalf("err %v, %d broadcasts; want ErrRepositoryMismatch and nothing broadcast", err, bc.count())
			}
		})
	}
}

func TestExtractBranchFromRef(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"refs/heads/main", "main"},
		{"refs/heads/feature/x", "feature/x"},
		{"main", "main"},
	}
	for _, tt := range tests {
		got := extractBranchFromRef(tt.ref)
		if got != tt.want {
			t.Errorf("extractBranchFromRef(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}
