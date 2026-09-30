package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	// The PM providers the webhooks sync with (main registers them the same way).
	_ "github.com/Strob0t/CodeForge/internal/adapter/githubpm"
	_ "github.com/Strob0t/CodeForge/internal/adapter/gitlab"
	_ "github.com/Strob0t/CodeForge/internal/adapter/plane"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
)

// KI-56: webhook-triggered roadmap syncs always failed - GitHub asked for
// provider "github" (registered "github-issues"), Plane got no api_token,
// GitLab an empty base URL, and the webhook answered 200 anyway.

type recordingSyncer struct {
	mu    sync.Mutex
	calls []roadmap.SyncConfig
	err   error
	done  chan struct{}
}

func (r *recordingSyncer) Sync(_ context.Context, cfg *roadmap.SyncConfig) (*roadmap.SyncResult, error) {
	r.mu.Lock()
	r.calls = append(r.calls, *cfg)
	r.mu.Unlock()
	defer func() { r.done <- struct{}{} }()
	if r.err != nil {
		return nil, r.err
	}
	return &roadmap.SyncResult{Created: 1}, nil
}

func (r *recordingSyncer) waitCall(t *testing.T) roadmap.SyncConfig {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(5 * time.Second):
		t.Fatal("no sync started")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[len(r.calls)-1]
}

type pmWebhookStore struct {
	mockStore
	projects []project.Project
}

func (s *pmWebhookStore) ListProjects(_ context.Context) ([]project.Project, error) {
	return s.projects, nil
}

// GetProjectByRepoName matches like the postgres store: the first project
// whose repository URL contains the name.
func (s *pmWebhookStore) GetProjectByRepoName(_ context.Context, name string) (*project.Project, error) {
	for i := range s.projects {
		if strings.Contains(s.projects[i].RepoURL, name) {
			return &s.projects[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func newPMWebhookEnv(configs map[string]map[string]string) (*PMWebhookService, *recordingSyncer) {
	store := &pmWebhookStore{projects: []project.Project{
		{ID: "gh", RepoURL: "https://github.com/acme/app.git"},
		{ID: "gl", RepoURL: "https://gitlab.example.com/group/sub/app.git"},
		{ID: "pl", Config: map[string]string{"plane_workspace": "acme", "plane_project_id": "p-1"}},
	}}
	syncer := &recordingSyncer{done: make(chan struct{}, 4)}
	return NewPMWebhookService(nil, syncer, store, configs), syncer
}

func TestPMWebhook_StartsASyncTheProviderCanRun(t *testing.T) {
	ctx := context.Background()
	svc, syncer := newPMWebhookEnv(map[string]map[string]string{"plane": {"api_token": "plane-token"}, "gitlab": {"token": "gl-token"}})

	if _, err := svc.HandleGitHubIssueWebhook(ctx, []byte(`{"action":"opened","issue":{"number":7},"repository":{"full_name":"acme/app"}}`)); err != nil {
		t.Fatalf("github webhook: %v", err)
	}
	if got := syncer.waitCall(t); got.Provider != "github-issues" || got.ProjectID != "gh" || got.ProjectRef != "acme/app" {
		t.Fatalf("github sync = %+v", got)
	}

	if _, err := svc.HandleGitLabIssueWebhook(ctx, []byte(`{"object_attributes":{"iid":3,"action":"open"},"project":{"path_with_namespace":"group/sub/app"}}`)); err != nil {
		t.Fatalf("gitlab webhook: %v", err)
	}
	got := syncer.waitCall(t)
	if got.Provider != "gitlab" || got.ProjectID != "gl" || got.ProviderConfig["base_url"] != "https://gitlab.example.com" || got.ProviderConfig["token"] != "gl-token" {
		t.Fatalf("gitlab sync = %+v", got)
	}

	// Plane's webhook names the workspace by its ID; the sync uses the
	// workspace slug of the project config.
	if _, err := svc.HandlePlaneWebhook(ctx, []byte(`{"event":"issue.created","data":{"id":"i-9","workspace":"ws-uuid","project":"p-1"}}`)); err != nil {
		t.Fatalf("plane webhook: %v", err)
	}
	got = syncer.waitCall(t)
	if got.Provider != "plane" || got.ProjectID != "pl" || got.ProjectRef != "acme/p-1" || got.ProviderConfig["api_token"] != "plane-token" {
		t.Fatalf("plane sync = %+v", got)
	}
}

func TestPMWebhook_AFailedSyncIsAnnounced(t *testing.T) {
	svc, syncer := newPMWebhookEnv(nil)
	hub := &internalMockBroadcaster{}
	svc.hub = hub
	syncer.err = errors.New("github api: 502")

	if _, err := svc.HandleGitHubIssueWebhook(context.Background(), []byte(`{"action":"opened","issue":{"number":7},"repository":{"full_name":"acme/app"}}`)); err != nil {
		t.Fatalf("github webhook: %v", err)
	}
	syncer.waitCall(t)
	deadline := time.Now().Add(5 * time.Second)
	for {
		hub.mu.Lock()
		events := slices.Clone(hub.events)
		hub.mu.Unlock()
		if len(events) > 0 {
			ev, ok := events[0].data.(event.PMSyncEvent)
			if events[0].eventType != "pm.sync" || !ok || ev.Status != "failed" || ev.ProjectID != "gh" || !strings.Contains(ev.Error, "502") {
				t.Fatalf("event = %+v", events[0])
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the failed sync was not announced")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPMWebhook_ProblemsAreErrorsNotSilentSuccess(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		configs map[string]map[string]string
		call    func(s *PMWebhookService) error
		want    error
	}{
		{name: "plane without api_token", configs: nil, want: domain.ErrValidation, call: func(s *PMWebhookService) error {
			_, err := s.HandlePlaneWebhook(ctx, []byte(`{"event":"issue.created","data":{"id":"i","workspace":"acme","project":"p-1"}}`))
			return err
		}},
		{name: "unknown github repository", want: domain.ErrNotFound, call: func(s *PMWebhookService) error {
			_, err := s.HandleGitHubIssueWebhook(ctx, []byte(`{"action":"opened","issue":{"number":1},"repository":{"full_name":"acme/other"}}`))
			return err
		}},
		{name: "unknown plane project", configs: map[string]map[string]string{"plane": {"api_token": "t"}}, want: domain.ErrNotFound, call: func(s *PMWebhookService) error {
			_, err := s.HandlePlaneWebhook(ctx, []byte(`{"event":"issue.created","data":{"id":"i","workspace":"acme","project":"nope"}}`))
			return err
		}},
		{name: "repository only part of a project's URL", want: domain.ErrNotFound, call: func(s *PMWebhookService) error {
			_, err := s.HandleGitHubIssueWebhook(ctx, []byte(`{"action":"opened","issue":{"number":1},"repository":{"full_name":"acme/ap"}}`))
			return err
		}},
		{name: "gitlab repository without a project", want: domain.ErrNotFound, call: func(s *PMWebhookService) error {
			_, err := s.HandleGitLabIssueWebhook(ctx, []byte(`{"object_attributes":{"iid":1},"project":{"path_with_namespace":"x/y"}}`))
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, syncer := newPMWebhookEnv(tc.configs)
			if err := tc.call(svc); !errors.Is(err, tc.want) {
				t.Fatalf("webhook = %v, want %v", err, tc.want)
			}
			select {
			case <-syncer.done:
				t.Fatal("a sync started although the webhook failed")
			case <-time.After(50 * time.Millisecond):
			}
		})
	}
}
