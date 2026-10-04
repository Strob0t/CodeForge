package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	// The PM providers the webhooks sync with (main registers them the same way).
	_ "github.com/Strob0t/CodeForge/internal/adapter/githubpm"
	"github.com/Strob0t/CodeForge/internal/adapter/gitlab"
	_ "github.com/Strob0t/CodeForge/internal/adapter/plane"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/domain/webhook"
	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-56: webhook-triggered roadmap syncs always failed - GitHub asked for
// provider "github" (registered "github-issues"), Plane got no api_token,
// GitLab an empty base URL, and the webhook answered 200 anyway. KI-85: the
// webhook's registration names its project; the event must be for that
// project's repository, and the operator's credentials serve only the
// default tenant.

const otherTenantID = "bbbbbbbb-0000-4000-8000-000000000002"

type recordingSyncer struct {
	mu      sync.Mutex
	calls   []roadmap.SyncConfig
	tenants []string
	err     error
	done    chan struct{}
}

func (r *recordingSyncer) Sync(ctx context.Context, cfg *roadmap.SyncConfig) (*roadmap.SyncResult, error) {
	r.mu.Lock()
	r.calls = append(r.calls, *cfg)
	r.tenants = append(r.tenants, tenantctx.FromContext(ctx))
	r.mu.Unlock()
	defer func() { r.done <- struct{}{} }()
	if r.err != nil {
		return nil, r.err
	}
	return &roadmap.SyncResult{Created: 1}, nil
}

func (r *recordingSyncer) waitCall(t *testing.T) (cfg roadmap.SyncConfig, tenantID string) {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(5 * time.Second):
		t.Fatal("no sync started")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[len(r.calls)-1], r.tenants[len(r.tenants)-1]
}

func (r *recordingSyncer) assertNoSync(t *testing.T) {
	t.Helper()
	select {
	case <-r.done:
		t.Fatal("a sync started")
	case <-time.After(50 * time.Millisecond):
	}
}

func newPMWebhookEnv(configs map[string]map[string]string) (*PMWebhookService, *recordingSyncer) {
	syncer := &recordingSyncer{done: make(chan struct{}, 8)}
	return NewPMWebhookService(nil, syncer, configs), syncer
}

var (
	ghProject    = &project.Project{ID: "gh", RepoURL: "https://github.com/acme/app.git"}
	gheProject   = &project.Project{ID: "ghe", RepoURL: "https://ghe.example.com/acme/app.git"}
	glProject    = &project.Project{ID: "gl", RepoURL: "https://gitlab.example.com/group/sub/app.git"}
	planeProject = &project.Project{ID: "pl", Config: map[string]string{"plane_workspace": "acme", "plane_project_id": "p-1"}}
)

const (
	githubIssueEvent = `{"action":"opened","issue":{"number":7},"repository":{"full_name":"acme/app"}}`
	gitlabIssueEvent = `{"object_attributes":{"iid":3,"action":"open"},"project":{"path_with_namespace":"group/sub/app","web_url":"https://gitlab.example.com/group/sub/app"}}`
	planeIssueEvent  = `{"event":"issue.created","data":{"id":"i-9","workspace":"ws-uuid","project":"p-1"}}`
)

func defaultTenantCtx() context.Context {
	return tenantctx.WithTenant(context.Background(), tenantctx.DefaultTenantID)
}

func TestPMWebhook_StartsASyncTheProviderCanRun(t *testing.T) {
	ctx := defaultTenantCtx()
	svc, syncer := newPMWebhookEnv(map[string]map[string]string{"plane": {"api_token": "plane-token"}})

	if _, err := svc.HandleEvent(ctx, "github", ghProject, "", []byte(githubIssueEvent)); err != nil {
		t.Fatalf("github webhook: %v", err)
	}
	if got, tenant := syncer.waitCall(t); got.Provider != "github-issues" || got.ProjectID != "gh" || got.ProjectRef != "acme/app" ||
		got.ProviderConfig["token"] != "" || tenant != tenantctx.DefaultTenantID {
		t.Fatalf("github sync = %+v in tenant %s", got, tenant)
	}

	if _, err := svc.HandleEvent(ctx, "gitlab", glProject, "glpat-x", []byte(gitlabIssueEvent)); err != nil {
		t.Fatalf("gitlab webhook: %v", err)
	}
	got, _ := syncer.waitCall(t)
	if got.Provider != "gitlab" || got.ProjectID != "gl" || got.ProjectRef != "group/sub/app" ||
		got.ProviderConfig["base_url"] != "https://gitlab.example.com" || got.ProviderConfig["token"] != "glpat-x" {
		t.Fatalf("gitlab sync = %+v", got)
	}

	// Plane's webhook names the workspace by its ID; the sync uses the
	// workspace slug of the project config.
	if _, err := svc.HandleEvent(ctx, "plane", planeProject, "", []byte(planeIssueEvent)); err != nil {
		t.Fatalf("plane webhook: %v", err)
	}
	got, _ = syncer.waitCall(t)
	if got.Provider != "plane" || got.ProjectID != "pl" || got.ProjectRef != "acme/p-1" || got.ProviderConfig["api_token"] != "plane-token" {
		t.Fatalf("plane sync = %+v", got)
	}
}

// KI-85 (D3): the event must name the webhook's project's repository
// exactly - host and full path, case-insensitive - never a substring.
func TestPMWebhook_EventMustBeForTheProjectsRepository(t *testing.T) {
	ctx := defaultTenantCtx()
	tests := []struct {
		name   string
		source string
		proj   *project.Project
		body   string
		match  bool
	}{
		{"same repository", "github", ghProject, `{"action":"opened","issue":{"number":1},"repository":{"full_name":"acme/app","html_url":"https://github.com/acme/app"}}`, true},
		{"other case", "github", ghProject, `{"action":"opened","issue":{"number":1},"repository":{"full_name":"ACME/App"}}`, true},
		{"enterprise host", "github", gheProject, `{"action":"opened","issue":{"number":1},"repository":{"full_name":"acme/app","html_url":"https://ghe.example.com/acme/app"}}`, true},
		{"prefix of the name", "github", ghProject, `{"action":"opened","issue":{"number":1},"repository":{"full_name":"acme/ap"}}`, false},
		{"longer name", "github", ghProject, `{"action":"opened","issue":{"number":1},"repository":{"full_name":"acme/app-private"}}`, false},
		{"other owner", "github", ghProject, `{"action":"opened","issue":{"number":1},"repository":{"full_name":"evil/app"}}`, false},
		{"same path on another host", "github", ghProject, `{"action":"opened","issue":{"number":1},"repository":{"full_name":"acme/app","html_url":"https://ghe.example.com/acme/app"}}`, false},
		{"no repository", "github", ghProject, `{"action":"opened","issue":{"number":1}}`, false},
		{"gitlab subgroup", "gitlab", glProject, gitlabIssueEvent, true},
		{"gitlab parent group", "gitlab", glProject, `{"object_attributes":{"iid":1},"project":{"path_with_namespace":"group/sub","web_url":"https://gitlab.example.com/group/sub"}}`, false},
		{"gitlab on another host", "gitlab", glProject, `{"object_attributes":{"iid":1},"project":{"path_with_namespace":"group/sub/app","web_url":"https://gitlab.other.example/group/sub/app"}}`, false},
		{"plane project", "plane", planeProject, planeIssueEvent, true},
		{"other plane project", "plane", planeProject, `{"event":"issue.created","data":{"id":"i","workspace":"w","project":"p-2"}}`, false},
		{"project without a repository", "github", &project.Project{ID: "local"}, githubIssueEvent, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, syncer := newPMWebhookEnv(map[string]map[string]string{"plane": {"api_token": "t"}})
			_, err := svc.HandleEvent(ctx, tc.source, tc.proj, "", []byte(tc.body))
			if tc.match {
				if err != nil {
					t.Fatalf("HandleEvent: %v", err)
				}
				if got, _ := syncer.waitCall(t); got.ProjectID != tc.proj.ID {
					t.Fatalf("synced project %s, want %s", got.ProjectID, tc.proj.ID)
				}
				return
			}
			if !errors.Is(err, webhook.ErrRepositoryMismatch) {
				t.Fatalf("HandleEvent = %v, want ErrRepositoryMismatch", err)
			}
			syncer.assertNoSync(t)
		})
	}
}

// S3-F security review S3: the operator's Plane token goes only to the
// operator's Plane (plane.base_url); a project-set plane_base_url that
// points elsewhere is not synced by the webhook.
func TestPMWebhook_PlaneTokenGoesOnlyToTheOperatorsPlane(t *testing.T) {
	ctx := defaultTenantCtx()
	svc, syncer := newPMWebhookEnv(map[string]map[string]string{"plane": {"api_token": "plane-token", "base_url": "https://plane.example.com"}})
	hub := &internalMockBroadcaster{}
	svc.hub = hub

	same := &project.Project{ID: "pl-same", Config: map[string]string{"plane_workspace": "acme", "plane_project_id": "p-3", "plane_base_url": "https://plane.example.com/"}}
	for _, proj := range []*project.Project{planeProject, same} {
		body := `{"event":"issue.created","data":{"id":"i","workspace":"w","project":"` + proj.Config["plane_project_id"] + `"}}`
		if _, err := svc.HandleEvent(ctx, "plane", proj, "", []byte(body)); err != nil {
			t.Fatalf("plane webhook %s: %v", proj.ID, err)
		}
		if got, _ := syncer.waitCall(t); got.ProviderConfig["base_url"] != "https://plane.example.com" || got.ProviderConfig["api_token"] != "plane-token" {
			t.Fatalf("plane sync %s = %+v", proj.ID, got)
		}
	}

	evil := &project.Project{ID: "pl-evil", Config: map[string]string{"plane_workspace": "acme", "plane_project_id": "p-2", "plane_base_url": "https://evil.example"}}
	_, err := svc.HandleEvent(ctx, "plane", evil, "", []byte(`{"event":"issue.created","data":{"id":"i","workspace":"w","project":"p-2"}}`))
	if !errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), "plane_base_url") {
		t.Fatalf("plane webhook for a project with another plane_base_url = %v, want a validation error naming plane_base_url", err)
	}
	syncer.assertNoSync(t)
	hub.mu.Lock()
	defer hub.mu.Unlock()
	var failed bool
	for _, e := range hub.events {
		if ev, ok := e.data.(event.PMSyncEvent); ok && e.eventType == "pm.sync" && ev.ProjectID == "pl-evil" && ev.Status == "failed" {
			failed = true
		}
	}
	if !failed {
		t.Fatalf("no pm.sync failed event for the refused project: %+v", hub.events)
	}
}

// KI-85: the operator's PM credentials - the Plane token, the Go Core's gh
// login - serve only the default tenant. Another tenant's integration syncs
// with its own API token or not at all; GitLab without a token syncs
// anonymously (public projects).
func TestPMWebhook_OperatorCredentialsServeOnlyTheDefaultTenant(t *testing.T) {
	other := tenantctx.WithTenant(context.Background(), otherTenantID)
	operator := map[string]map[string]string{"plane": {"api_token": "operator-plane-token", "base_url": "https://api.plane.so"}}
	tests := []struct {
		name      string
		source    string
		proj      *project.Project
		body      string
		apiToken  string
		wantErr   bool
		tokenKey  string
		wantToken string
	}{
		{name: "github without a token", source: "github", proj: ghProject, body: githubIssueEvent, wantErr: true},
		{name: "github with the integration's token", source: "github", proj: ghProject, body: githubIssueEvent, apiToken: "ghp_b", tokenKey: "token", wantToken: "ghp_b"},
		{name: "plane without a token", source: "plane", proj: planeProject, body: planeIssueEvent, wantErr: true},
		{name: "plane with the integration's token", source: "plane", proj: planeProject, body: planeIssueEvent, apiToken: "plane_b", tokenKey: "api_token", wantToken: "plane_b"},
		{name: "gitlab without a token", source: "gitlab", proj: glProject, body: gitlabIssueEvent, tokenKey: "token", wantToken: ""},
		{name: "gitlab with the integration's token", source: "gitlab", proj: glProject, body: gitlabIssueEvent, apiToken: "glpat-b", tokenKey: "token", wantToken: "glpat-b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, syncer := newPMWebhookEnv(operator)
			_, err := svc.HandleEvent(other, tc.source, tc.proj, tc.apiToken, []byte(tc.body))
			if tc.wantErr {
				if !errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), "api_token") {
					t.Fatalf("HandleEvent = %v, want a validation error naming api_token", err)
				}
				syncer.assertNoSync(t)
				return
			}
			if err != nil {
				t.Fatalf("HandleEvent: %v", err)
			}
			got, tenant := syncer.waitCall(t)
			if got.ProviderConfig[tc.tokenKey] != tc.wantToken || tenant != otherTenantID {
				t.Fatalf("sync config %+v in tenant %s, want %s = %q in %s", got.ProviderConfig, tenant, tc.tokenKey, tc.wantToken, otherTenantID)
			}
			if strings.Contains(strings.Join(mapValues(got.ProviderConfig), " "), "operator-plane-token") {
				t.Fatalf("another tenant's sync got the operator's token: %+v", got.ProviderConfig)
			}
		})
	}
}

func mapValues(m map[string]string) []string {
	values := make([]string, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

// KI-85 (D5): a GitLab PM integration's token reaches the GitLab API when
// the webhook's sync runs, so private GitLab projects sync.
func TestPMWebhook_GitLabTokenReachesGitLab(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	gitlabAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.EscapedPath()+" token="+r.Header.Get("PRIVATE-TOKEN"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer gitlabAPI.Close()

	setGitLabOutboundPolicy(t, "127.0.0.1") // the fake GitLab listens on loopback
	hub := &internalMockBroadcaster{}
	svc := NewPMWebhookService(hub, NewSyncService(&mockStore{}), nil)
	proj := &project.Project{ID: "gl-private", RepoURL: gitlabAPI.URL + "/group/private-app.git"}
	body := `{"object_attributes":{"iid":3,"action":"open"},"project":{"path_with_namespace":"group/private-app","web_url":"` + gitlabAPI.URL + `/group/private-app"}}`
	if _, err := svc.HandleEvent(tenantctx.WithTenant(context.Background(), otherTenantID), "gitlab", proj, "glpat-private", []byte(body)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		got := slices.Clone(seen)
		mu.Unlock()
		if len(got) > 0 {
			if got[0] != "/api/v4/projects/group%2Fprivate-app/issues token=glpat-private" {
				t.Fatalf("GitLab API got %v, want the issues of group/private-app with the integration's token", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the sync did not call the GitLab API")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// setGitLabOutboundPolicy lets the GitLab PM provider reach the private
// and loopback addresses of allowed (pm.allowed_private_hosts) until the
// test ends; then none again, the default.
func setGitLabOutboundPolicy(t *testing.T, allowed ...string) {
	t.Helper()
	policy, err := netutil.NewOutboundPolicy(allowed)
	if err != nil {
		t.Fatal(err)
	}
	gitlab.SetOutboundPolicy(policy)
	t.Cleanup(func() {
		none, _ := netutil.NewOutboundPolicy(nil)
		gitlab.SetOutboundPolicy(none)
	})
}

// TestPMWebhook_GitLabSyncReachesNoPrivateHost (KI-85 review, security
// finding 2): a tenant chooses its project's repo_url, whose host the GitLab
// sync calls. A loopback or private host is refused before anything
// connects, unless the operator allowlists it, and the pm.sync event says
// why without the token.
func TestPMWebhook_GitLabSyncReachesNoPrivateHost(t *testing.T) {
	setGitLabOutboundPolicy(t)
	var mu sync.Mutex
	var hits int
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		_, _ = w.Write([]byte(`[]`))
	}))
	defer internal.Close()

	hub := &internalMockBroadcaster{}
	svc := NewPMWebhookService(hub, NewSyncService(&mockStore{}), nil)
	proj := &project.Project{ID: "gl-internal", RepoURL: internal.URL + "/group/app.git"}
	body := `{"object_attributes":{"iid":3,"action":"open"},"project":{"path_with_namespace":"group/app","web_url":"` + internal.URL + `/group/app"}}`
	if _, err := svc.HandleEvent(tenantctx.WithTenant(context.Background(), otherTenantID), "gitlab", proj, "glpat-tenant-b", []byte(body)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		hub.mu.Lock()
		events := slices.Clone(hub.events)
		hub.mu.Unlock()
		if len(events) > 0 {
			ev, ok := events[0].data.(event.PMSyncEvent)
			if !ok || ev.Status != "failed" || !strings.Contains(ev.Error, "loopback address") || !strings.Contains(ev.Error, "pm.allowed_private_hosts") {
				t.Fatalf("pm.sync = %+v, want a failure naming the refused loopback address and pm.allowed_private_hosts", events[0])
			}
			if strings.Contains(ev.Error, "glpat-tenant-b") {
				t.Fatalf("pm.sync error %q carries the token", ev.Error)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the refused sync was not announced")
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 0 {
		t.Fatalf("the loopback server got %d requests, want none", hits)
	}
}

func TestPMWebhook_AFailedSyncIsAnnounced(t *testing.T) {
	svc, syncer := newPMWebhookEnv(nil)
	hub := &internalMockBroadcaster{}
	svc.hub = hub
	syncer.err = errors.New("github api: 502")

	if _, err := svc.HandleEvent(defaultTenantCtx(), "github", ghProject, "", []byte(githubIssueEvent)); err != nil {
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
	ctx := defaultTenantCtx()
	tests := []struct {
		name   string
		source string
		proj   *project.Project
		body   string
		want   error
	}{
		{name: "plane without api_token", source: "plane", proj: planeProject, body: planeIssueEvent, want: domain.ErrValidation},
		{name: "invalid github payload", source: "github", proj: ghProject, body: `{"repository":`, want: domain.ErrValidation},
		{name: "invalid gitlab payload", source: "gitlab", proj: glProject, body: `[]`, want: domain.ErrValidation},
		{name: "unknown source", source: "jira", proj: ghProject, body: `{}`, want: domain.ErrValidation},
		{name: "gitlab project without a repository URL", source: "gitlab", proj: &project.Project{ID: "x", RepoURL: "/srv/repo"}, body: gitlabIssueEvent, want: webhook.ErrRepositoryMismatch},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, syncer := newPMWebhookEnv(nil)
			if _, err := svc.HandleEvent(ctx, tc.source, tc.proj, "", []byte(tc.body)); !errors.Is(err, tc.want) {
				t.Fatalf("HandleEvent = %v, want %v", err, tc.want)
			}
			syncer.assertNoSync(t)
		})
	}
}
