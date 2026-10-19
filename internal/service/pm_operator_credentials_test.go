package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-85 review: the operator's PM credentials (github.token,
// plane.api_token) serve only the default tenant on every path - webhook
// syncs, PM imports and manual syncs - and the operator's settings never
// reach another tenant in an error.

// listingPMProvider records the project refs it lists.
type listingPMProvider struct {
	name  string
	mu    sync.Mutex
	lists []string
}

func (p *listingPMProvider) Name() string { return p.name }
func (p *listingPMProvider) Capabilities() pmprovider.Capabilities {
	return pmprovider.Capabilities{ListItems: true}
}

func (p *listingPMProvider) ListItems(_ context.Context, projectRef string) ([]pmprovider.Item, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lists = append(p.lists, projectRef)
	return []pmprovider.Item{{ID: "1", ExternalID: "1", Title: "issue"}}, nil
}

func (p *listingPMProvider) GetItem(context.Context, string, string) (*pmprovider.Item, error) {
	return nil, pmprovider.ErrNotSupported
}

func (p *listingPMProvider) CreateItem(context.Context, string, *pmprovider.Item) (*pmprovider.Item, error) {
	return nil, pmprovider.ErrNotSupported
}

func (p *listingPMProvider) UpdateItem(context.Context, string, *pmprovider.Item) (*pmprovider.Item, error) {
	return nil, pmprovider.ErrNotSupported
}

func (p *listingPMProvider) listed() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.lists...)
}

// POST /projects/{id}/roadmap/import/pm uses the providers built at startup
// with the operator's credentials: github-issues uses github.token, plane
// carries plane.api_token. Another tenant's import from them is
// refused before the provider is asked; GitLab (no operator credential) is
// imported.
func TestImportPMItems_OperatorCredentialsServeOnlyTheDefaultTenant(t *testing.T) {
	store := &mockStore{projects: []project.Project{{ID: "p1", Name: "p1"}}}
	other := tenantctx.WithTenant(context.Background(), otherTenantID)
	tests := []struct {
		provider string
		ctx      context.Context
		refused  bool
	}{
		{"github-issues", other, true},
		{"plane", other, true},
		{"gitlab", other, false},
		{"github-issues", defaultTenantCtx(), false},
		{"plane", defaultTenantCtx(), false},
	}
	for _, tc := range tests {
		t.Run(tc.provider+"/"+tenantctx.FromContext(tc.ctx), func(t *testing.T) {
			prov := &listingPMProvider{name: tc.provider}
			svc := NewRoadmapService(store, nil, nil, []pmprovider.Provider{prov})
			res, err := svc.ImportPMItems(tc.ctx, "p1", tc.provider, "operator-org/private-repo")
			if tc.refused {
				if !errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), "default tenant") {
					t.Fatalf("ImportPMItems = %+v, %v; want a validation error naming the default tenant", res, err)
				}
				if got := prov.listed(); len(got) != 0 {
					t.Fatalf("the provider listed %v for another tenant", got)
				}
				return
			}
			if err != nil || res.FeaturesCreated != 1 {
				t.Fatalf("ImportPMItems = %+v, %v", res, err)
			}
		})
	}
}

// POST /projects/{id}/roadmap/sync takes the provider config from the
// request; github-issues without a token would run gh with the Go Core's
// login. Another tenant must bring its own token.
func TestSync_GitHubLoginServesOnlyTheDefaultTenant(t *testing.T) {
	svc := NewSyncService(&mockStore{})
	other := tenantctx.WithTenant(context.Background(), otherTenantID)
	cfg := &roadmap.SyncConfig{ProjectID: "p1", ProjectRef: "operator-org/private-repo", Provider: "github-issues", Direction: roadmap.SyncDirectionPull}
	if _, err := svc.Sync(other, cfg); !errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), "token") {
		t.Fatalf("Sync without a token in another tenant = %v, want a validation error naming the token", err)
	}

	// With its own token (or in the default tenant) the sync gets past the
	// check; an unknown direction then stops it before gh runs.
	for name, tc := range map[string]struct {
		ctx   context.Context
		token string
	}{
		"own token":      {other, "ghp_own"},
		"default tenant": {defaultTenantCtx(), ""},
	} {
		c := *cfg
		c.Direction, c.ProviderConfig = "nowhere", map[string]string{"token": tc.token}
		if _, err := svc.Sync(tc.ctx, &c); err == nil || !strings.Contains(err.Error(), "unknown sync direction") {
			t.Fatalf("%s: Sync = %v, want it past the credential check", name, err)
		}
	}
}

// The refusal of a Plane webhook sync for a project whose plane_base_url
// is not the operator's Plane reaches the delivery's answer and the
// tenant's pm.sync event: it names the project's setting, never the
// operator's plane.base_url (it may be an internal host).
func TestPMWebhook_PlaneRefusalDoesNotNameTheOperatorsPlane(t *testing.T) {
	const operatorPlane = "https://plane.corp.internal"
	svc, syncer := newPMWebhookEnv(map[string]map[string]string{"plane": {"api_token": "operator-plane-token", "base_url": operatorPlane}})
	hub := &internalMockBroadcaster{}
	svc.hub = hub
	proj := &project.Project{ID: "pl-b", Config: map[string]string{"plane_workspace": "b", "plane_project_id": "P", "plane_base_url": "https://x.invalid"}}

	_, err := svc.HandleEvent(tenantctx.WithTenant(context.Background(), otherTenantID), "plane", proj, "plane_b",
		[]byte(`{"event":"issue.updated","data":{"id":"i","workspace":"w","project":"P"}}`))
	if !errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), "plane_base_url") {
		t.Fatalf("HandleEvent = %v, want a validation error naming plane_base_url", err)
	}
	if strings.Contains(err.Error(), "plane.corp.internal") {
		t.Fatalf("the error names the operator's Plane: %v", err)
	}
	syncer.assertNoSync(t)
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if len(hub.events) != 1 {
		t.Fatalf("events %+v, want one pm.sync", hub.events)
	}
	if ev, ok := hub.events[0].data.(event.PMSyncEvent); !ok || ev.Status != "failed" || strings.Contains(ev.Error, "plane.corp.internal") {
		t.Fatalf("pm.sync event %+v names the operator's Plane or is no failure", hub.events[0])
	}
}
