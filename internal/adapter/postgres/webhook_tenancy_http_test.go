package postgres_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	// The GitHub issues provider the PM webhook syncs with (main registers it the same way).
	_ "github.com/Strob0t/CodeForge/internal/adapter/githubpm"
	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/domain/webhook"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// tenantSyncer records the project and tenant of every sync a webhook starts.
type tenantSyncer struct {
	mu    sync.Mutex
	syncs []string // "<project>@<tenant>"
}

func (s *tenantSyncer) Sync(ctx context.Context, cfg *roadmap.SyncConfig) (*roadmap.SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncs = append(s.syncs, cfg.ProjectID+"@"+tenantctx.FromContext(ctx))
	return &roadmap.SyncResult{}, nil
}

func (s *tenantSyncer) waitFor(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		got := append([]string(nil), s.syncs...)
		s.mu.Unlock()
		if len(got) >= n || time.Now().After(deadline) {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestWebhooks_PerTenantOnPostgres (KI-85): two tenants with a project of the
// same repository register their own webhooks through the API (admins
// only; the secret is shown once). A delivery acts only in the tenant of the
// webhook it is signed for - whatever X-Tenant-ID says - and is handled once.
func TestWebhooks_PerTenantOnPostgres(t *testing.T) {
	store := setupStore(t)
	tenantA, tenantB := createTestTenant(t, store), createTestTenant(t, store)
	ctxA, ctxB := ctxWithTenant(t, tenantA), ctxWithTenant(t, tenantB)
	projA, projB := createWebhookProject(ctxA, t, store), createWebhookProject(ctxB, t, store)

	syncer := &tenantSyncer{}
	webhooks := service.NewWebhookService(store, []byte("0123456789abcdef0123456789abcdef"),
		service.NewVCSWebhookService(nil), service.NewPMWebhookService(nil, syncer, nil), time.Hour)
	r := chi.NewRouter()
	r.Use(middleware.TenantID)
	cfhttp.MountRoutes(r, &cfhttp.Handlers{Webhooks: webhooks, Limits: &config.Limits{MaxRequestBodySize: 1 << 20}})

	call := func(ctx context.Context, u *user.User, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(middleware.ContextWithTestUser(ctx, u))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	register := func(ctx context.Context, tenantID, projectID string) webhook.Registered {
		t.Helper()
		admin := &user.User{ID: uuid.NewString(), Role: user.RoleAdmin, TenantID: tenantID}
		w := call(ctx, admin, http.MethodPost, "/api/v1/projects/"+projectID+"/webhooks", `{"kind":"pm","provider":"github","api_token":"ghp_x"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("register: status %d: %s", w.Code, w.Body.String())
		}
		var reg webhook.Registered
		if err := json.Unmarshal(w.Body.Bytes(), &reg); err != nil || reg.Secret == "" {
			t.Fatalf("register response %s: %v", w.Body.String(), err)
		}
		return reg
	}
	regA, regB := register(ctxA, tenantA, projA.ID), register(ctxB, tenantB, projB.ID)

	// An editor cannot register; the list shows no secret.
	editorA := &user.User{ID: uuid.NewString(), Role: user.RoleEditor, TenantID: tenantA}
	if w := call(ctxA, editorA, http.MethodPost, "/api/v1/projects/"+projA.ID+"/webhooks", `{"kind":"vcs","provider":"github"}`); w.Code != http.StatusForbidden {
		t.Fatalf("editor registers: status %d", w.Code)
	}
	if w := call(ctxA, editorA, http.MethodGet, "/api/v1/projects/"+projA.ID+"/webhooks", ""); w.Code != http.StatusOK ||
		strings.Contains(w.Body.String(), regA.Secret) || !strings.Contains(w.Body.String(), regA.URL) {
		t.Fatalf("list: status %d body %s", w.Code, w.Body.String())
	}
	// Tenant B's admin cannot see or rotate A's webhook.
	adminB := &user.User{ID: uuid.NewString(), Role: user.RoleAdmin, TenantID: tenantB}
	if w := call(ctxB, adminB, http.MethodPost, "/api/v1/projects/"+projA.ID+"/webhooks/"+regA.ID+"/rotate", ""); w.Code != http.StatusNotFound {
		t.Fatalf("tenant B rotates A's webhook: status %d", w.Code)
	}

	// One body per delivery, as GitHub sends them: a redelivery repeats its
	// delivery's body.
	issueEvent := func(deliveryID string) string {
		return `{"action":"opened","issue":{"number":1,"node_id":"` + deliveryID + `"},"repository":{"full_name":"acme/app","html_url":"https://github.com/acme/app"}}`
	}
	deliverBody := func(url, secret, deliveryID, tenantHeader, body string) *httptest.ResponseRecorder {
		t.Helper()
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(body))
		req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(body))
		req.Header.Set("X-GitHub-Event", "issues")
		req.Header.Set("X-GitHub-Delivery", deliveryID)
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		if tenantHeader != "" {
			req.Header.Set("X-Tenant-ID", tenantHeader)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	deliver := func(url, secret, deliveryID, tenantHeader string) *httptest.ResponseRecorder {
		t.Helper()
		return deliverBody(url, secret, deliveryID, tenantHeader, issueEvent(deliveryID))
	}

	// Signed for A, naming tenant B in the header: acts in A.
	if w := deliver(regA.URL, regA.Secret, "d-1", tenantB); w.Code != http.StatusAccepted {
		t.Fatalf("delivery to A: status %d: %s", w.Code, w.Body.String())
	}
	if got := syncer.waitFor(t, 1); len(got) != 1 || got[0] != projA.ID+"@"+tenantA {
		t.Fatalf("syncs %v, want project A in tenant A", got)
	}
	// Redelivered, or replayed under a fresh delivery ID (the signature
	// covers only the body): handled once.
	if w := deliver(regA.URL, regA.Secret, "d-1", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "duplicate") {
		t.Fatalf("redelivery: status %d: %s", w.Code, w.Body.String())
	}
	if w := deliverBody(regA.URL, regA.Secret, uuid.NewString(), "", issueEvent("d-1")); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "duplicate") {
		t.Fatalf("replay with a fresh delivery ID: status %d: %s", w.Code, w.Body.String())
	}
	// Signed for A, sent to B's webhook: refused like an unknown webhook.
	wrong := deliver(regB.URL, regA.Secret, "d-2", "")
	unknown := deliver("/api/v1/webhooks/pm/github/"+uuid.NewString(), regA.Secret, "d-3", "")
	if wrong.Code != http.StatusUnauthorized || unknown.Code != http.StatusUnauthorized || wrong.Body.String() != unknown.Body.String() {
		t.Fatalf("wrong secret: %d %s; unknown webhook: %d %s; want the same 401", wrong.Code, wrong.Body.String(), unknown.Code, unknown.Body.String())
	}
	// B's own delivery acts in B.
	if w := deliver(regB.URL, regB.Secret, "d-1", tenantA); w.Code != http.StatusAccepted {
		t.Fatalf("delivery to B: status %d: %s", w.Code, w.Body.String())
	}
	if got := syncer.waitFor(t, 2); len(got) != 2 || got[1] != projB.ID+"@"+tenantB {
		t.Fatalf("syncs %v, want project B in tenant B second", got)
	}
	// A payload that cannot be read is a 400; its redelivery is not taken
	// for a duplicate.
	if w := deliverBody(regA.URL, regA.Secret, "d-9", "", `{"repository":`); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid payload: status %d: %s", w.Code, w.Body.String())
	}
	if w := deliver(regA.URL, regA.Secret, "d-9", ""); w.Code != http.StatusAccepted {
		t.Fatalf("redelivery after a failure: status %d: %s", w.Code, w.Body.String())
	}
	// After rotation the old secret is refused.
	adminA := &user.User{ID: uuid.NewString(), Role: user.RoleAdmin, TenantID: tenantA}
	w := call(ctxA, adminA, http.MethodPost, "/api/v1/projects/"+projA.ID+"/webhooks/"+regA.ID+"/rotate", "")
	var rotated webhook.Registered
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &rotated) != nil || rotated.Secret == "" || rotated.Secret == regA.Secret {
		t.Fatalf("rotate: status %d: %s", w.Code, w.Body.String())
	}
	if w := deliver(regA.URL, regA.Secret, "d-10", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("old secret after rotation: status %d", w.Code)
	}
	if w := deliver(regA.URL, rotated.Secret, "d-10", ""); w.Code != http.StatusAccepted {
		t.Fatalf("new secret: status %d: %s", w.Code, w.Body.String())
	}

	// Plane generates its webhooks' secret: it is given at registration and
	// at rotation.
	w = call(ctxA, adminA, http.MethodPost, "/api/v1/projects/"+projA.ID+"/webhooks",
		`{"kind":"pm","provider":"plane","api_token":"plane_x","secret":"plane_wh_0123456789abcdef"}`)
	var plane webhook.Registered
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &plane) != nil || plane.Secret != "plane_wh_0123456789abcdef" {
		t.Fatalf("register plane: status %d: %s", w.Code, w.Body.String())
	}
	if w = call(ctxA, adminA, http.MethodPost, "/api/v1/projects/"+projA.ID+"/webhooks/"+plane.ID+"/rotate", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("plane rotation without the new secret: status %d: %s", w.Code, w.Body.String())
	}
	w = call(ctxA, adminA, http.MethodPost, "/api/v1/projects/"+projA.ID+"/webhooks/"+plane.ID+"/rotate", `{"secret":"plane_wh_new_0123456789"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "plane_wh_new_0123456789") {
		t.Fatalf("plane rotation: status %d: %s", w.Code, w.Body.String())
	}
}
