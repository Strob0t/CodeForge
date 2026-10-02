package http_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/domain/webhook"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// emptyWebhookStore has no projects and no webhooks: every lookup is
// not found.
type emptyWebhookStore struct{}

func (emptyWebhookStore) GetProject(context.Context, string) (*project.Project, error) {
	return nil, domain.ErrNotFound
}

func (emptyWebhookStore) CreateWebhookEndpoint(context.Context, *webhook.Endpoint) (*webhook.Endpoint, error) {
	return nil, domain.ErrNotFound
}

func (emptyWebhookStore) ListWebhookEndpoints(context.Context, string) ([]webhook.Endpoint, error) {
	return nil, nil
}

func (emptyWebhookStore) GetWebhookEndpoint(context.Context, string, string) (*webhook.Endpoint, error) {
	return nil, domain.ErrNotFound
}

func (emptyWebhookStore) LookupWebhookEndpoint(context.Context, string) (*webhook.Endpoint, error) {
	return nil, domain.ErrNotFound
}

func (emptyWebhookStore) RotateWebhookSecret(context.Context, string, string, []byte) (*webhook.Endpoint, error) {
	return nil, domain.ErrNotFound
}

func (emptyWebhookStore) SetWebhookAPIToken(context.Context, string, string, []byte) error {
	return domain.ErrNotFound
}

func (emptyWebhookStore) DeleteWebhookEndpoint(context.Context, string, string) error {
	return domain.ErrNotFound
}

func (emptyWebhookStore) ClaimWebhookDelivery(context.Context, string, []string, time.Duration) (bool, error) {
	return true, nil
}

func (emptyWebhookStore) ReleaseWebhookDelivery(context.Context, string, []string) error { return nil }

// countingSyncer counts the roadmap syncs webhooks start.
type countingSyncer struct {
	mu    sync.Mutex
	calls int
}

func (c *countingSyncer) Sync(_ context.Context, _ *roadmap.SyncConfig) (*roadmap.SyncResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return &roadmap.SyncResult{}, nil
}

func webhookRouter() http.Handler {
	webhooks := service.NewWebhookService(emptyWebhookStore{}, []byte("0123456789abcdef0123456789abcdef"),
		service.NewVCSWebhookService(nil), service.NewPMWebhookService(nil, &countingSyncer{}, nil), time.Hour)
	return newTestRouterWithLLM(&mockStore{}, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000",
		func(h *cfhttp.Handlers) { h.Webhooks = webhooks })
}

// KI-85 (D4): registering, rotating, changing the token of and deleting a
// webhook need a tenant admin; editors may list (IDs and URLs, never
// secrets); viewers get nothing.
func TestWebhookRoutes_Roles(t *testing.T) {
	admin := &user.User{ID: "ad", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	editor := &user.User{ID: "ed", Role: user.RoleEditor, TenantID: tenantctx.DefaultTenantID}
	viewer := &user.User{ID: "vi", Role: user.RoleViewer, TenantID: tenantctx.DefaultTenantID}
	const base = "/api/v1/projects/p1/webhooks"
	routes := []struct {
		method, path, body string
		editorMay          bool
	}{
		{http.MethodPost, base, `{"kind":"vcs","provider":"github"}`, false},
		{http.MethodGet, base, "", true},
		{http.MethodPost, base + "/w1/rotate", "", false},
		{http.MethodPut, base + "/w1/api-token", `{"api_token":"t"}`, false},
		{http.MethodDelete, base + "/w1", "", false},
	}
	r := webhookRouter()
	for _, rt := range routes {
		for _, u := range []*user.User{viewer, editor, admin} {
			t.Run(rt.method+" "+rt.path+"/"+string(u.Role), func(t *testing.T) {
				req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
				req.Header.Set("Content-Type", "application/json")
				req = req.WithContext(middleware.ContextWithTestUser(req.Context(), u))
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				allowed := u == admin || (u == editor && rt.editorMay)
				if forbidden := w.Code == http.StatusForbidden; forbidden == allowed {
					t.Fatalf("status %d (%s), allowed = %v", w.Code, w.Body.String(), allowed)
				}
			})
		}
	}
}

// KI-85: an unknown webhook, an ID that is no UUID and a bad signature get
// the same 401, so the answer reveals no webhook ID; the removed global
// routes answer 410 and name the migration.
func TestWebhookInbound_Answers(t *testing.T) {
	r := webhookRouter()
	post := func(path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	signed := map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": "sha256=00", "X-Tenant-ID": "not-a-uuid"}
	first := post("/api/v1/webhooks/vcs/github/"+uuid.NewString(), []byte(`{}`), signed)
	if first.Code != http.StatusUnauthorized {
		t.Fatalf("unknown webhook: status %d %s", first.Code, first.Body.String())
	}
	for _, path := range []string{
		"/api/v1/webhooks/vcs/github/not-a-uuid",
		"/api/v1/webhooks/pm/plane/" + uuid.NewString(),
		"/api/v1/webhooks/pm/github/" + uuid.NewString(),
		"/api/v1/webhooks/vcs/bitbucket/" + uuid.NewString(),
	} {
		if w := post(path, []byte(`{}`), signed); w.Code != first.Code || w.Body.String() != first.Body.String() {
			t.Fatalf("%s: status %d %s, want the same answer as for an unknown webhook", path, w.Code, w.Body.String())
		}
	}
	if w := post("/api/v1/webhooks/vcs/github/"+uuid.NewString(), nil, nil); w.Code != first.Code || w.Body.String() != first.Body.String() {
		t.Fatalf("no signature: status %d %s", w.Code, w.Body.String())
	}

	for _, old := range []string{"/vcs/github", "/vcs/gitlab", "/pm/github", "/pm/gitlab", "/pm/plane"} {
		w := post("/api/v1/webhooks"+old, []byte(`{}`), signed)
		if w.Code != http.StatusGone || !strings.Contains(w.Body.String(), "/api/v1/projects/{id}/webhooks") {
			t.Fatalf("old route %s: status %d %s, want 410 naming the per-project registration", old, w.Code, w.Body.String())
		}
	}

	big := bytes.Repeat([]byte("a"), 10<<20+1)
	if w := post("/api/v1/webhooks/vcs/github/"+uuid.NewString(), big, signed); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body over 10 MB: status %d", w.Code)
	}
}
