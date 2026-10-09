package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/crypto"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/webhook"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-85: inbound webhooks are registered per project. The webhook's random
// ID names its tenant and project, its own secret authenticates the sender,
// and nothing in the request (header, payload) picks another tenant.

const tenantA, tenantB = "aaaaaaaa-0000-4000-8000-00000000000a", "bbbbbbbb-0000-4000-8000-00000000000b"

var testWebhookKey = []byte("0123456789abcdef0123456789abcdef")

// webhookFakeStore keeps projects, webhooks and deliveries in memory with the
// postgres store's tenant rules.
type webhookFakeStore struct {
	mu            sync.Mutex
	projects      map[string]project.Project
	projectTenant map[string]string
	endpoints     map[string]*webhook.Endpoint
	deliveries    map[string]time.Time
}

func newWebhookFakeStore() *webhookFakeStore {
	return &webhookFakeStore{
		projects: map[string]project.Project{}, projectTenant: map[string]string{},
		endpoints: map[string]*webhook.Endpoint{}, deliveries: map[string]time.Time{},
	}
}

func (s *webhookFakeStore) addProject(tenantID string, p *project.Project) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[p.ID] = *p
	s.projectTenant[p.ID] = tenantID
}

func (s *webhookFakeStore) GetProject(ctx context.Context, id string) (*project.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[id]
	if !ok || s.projectTenant[id] != tenantctx.FromContext(ctx) {
		return nil, domain.ErrNotFound
	}
	return &p, nil
}

func (s *webhookFakeStore) endpointOf(ctx context.Context, projectID, id string) (*webhook.Endpoint, error) {
	e, ok := s.endpoints[id]
	if !ok || e.ProjectID != projectID || e.TenantID != tenantctx.FromContext(ctx) {
		return nil, domain.ErrNotFound
	}
	return e, nil
}

func (s *webhookFakeStore) CreateWebhookEndpoint(ctx context.Context, e *webhook.Endpoint) (*webhook.Endpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tid := tenantctx.FromContext(ctx)
	if _, ok := s.projects[e.ProjectID]; !ok || s.projectTenant[e.ProjectID] != tid {
		return nil, domain.ErrNotFound
	}
	for _, other := range s.endpoints {
		if other.ProjectID == e.ProjectID && other.Kind == e.Kind && other.Provider == e.Provider {
			return nil, domain.ErrConflict
		}
	}
	created := *e
	created.ID, created.TenantID = uuid.NewString(), tid
	created.CreatedAt, created.SecretRotatedAt = time.Now(), time.Now()
	s.endpoints[created.ID] = &created
	out := created
	return &out, nil
}

func (s *webhookFakeStore) ListWebhookEndpoints(ctx context.Context, projectID string) ([]webhook.Endpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []webhook.Endpoint
	for _, e := range s.endpoints {
		if e.ProjectID == projectID && e.TenantID == tenantctx.FromContext(ctx) {
			out = append(out, *e)
		}
	}
	return out, nil
}

func (s *webhookFakeStore) GetWebhookEndpoint(ctx context.Context, projectID, id string) (*webhook.Endpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.endpointOf(ctx, projectID, id)
	if err != nil {
		return nil, err
	}
	out := *e
	return &out, nil
}

func (s *webhookFakeStore) LookupWebhookEndpoint(_ context.Context, id string) (*webhook.Endpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.endpoints[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	out := *e
	return &out, nil
}

func (s *webhookFakeStore) RotateWebhookSecret(ctx context.Context, projectID, id string, encryptedSecret []byte) (*webhook.Endpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.endpointOf(ctx, projectID, id)
	if err != nil {
		return nil, err
	}
	e.EncryptedSecret, e.SecretRotatedAt = encryptedSecret, time.Now()
	out := *e
	return &out, nil
}

func (s *webhookFakeStore) SetWebhookAPIToken(ctx context.Context, projectID, id string, encryptedToken []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.endpointOf(ctx, projectID, id)
	if err != nil {
		return err
	}
	e.EncryptedAPIToken = encryptedToken
	return nil
}

func (s *webhookFakeStore) DeleteWebhookEndpoint(ctx context.Context, projectID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.endpointOf(ctx, projectID, id); err != nil {
		return err
	}
	delete(s.endpoints, id)
	return nil
}

func (s *webhookFakeStore) ClaimWebhookDelivery(_ context.Context, webhookID string, keys []string, retention time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, at := range s.deliveries {
		if strings.HasPrefix(key, webhookID+"|") && time.Since(at) > retention {
			delete(s.deliveries, key)
		}
	}
	for _, k := range keys {
		if _, ok := s.deliveries[webhookID+"|"+k]; ok {
			return false, nil
		}
	}
	for _, k := range keys {
		s.deliveries[webhookID+"|"+k] = time.Now()
	}
	return true, nil
}

func (s *webhookFakeStore) ReleaseWebhookDelivery(_ context.Context, webhookID string, keys []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range keys {
		delete(s.deliveries, webhookID+"|"+k)
	}
	return nil
}

func githubSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func planeSignature(secret string, body []byte) string {
	return strings.TrimPrefix(githubSignature(secret, body), "sha256=")
}

type webhookEnv struct {
	store  *webhookFakeStore
	svc    *WebhookService
	syncer *recordingSyncer
	hub    *tenantBroadcaster
}

// newWebhookEnv has one project in tenant A and one in tenant B, both for
// github.com/acme/app, plus a GitLab and a Plane project in tenant A.
func newWebhookEnv(t *testing.T) *webhookEnv {
	t.Helper()
	store := newWebhookFakeStore()
	store.addProject(tenantA, &project.Project{ID: "proj-a", RepoURL: "https://github.com/acme/app.git"})
	store.addProject(tenantB, &project.Project{ID: "proj-b", RepoURL: "https://github.com/acme/app.git"})
	store.addProject(tenantA, &project.Project{ID: "proj-gl", RepoURL: "https://gitlab.example.com/group/app.git"})
	store.addProject(tenantA, &project.Project{ID: "proj-pl", Config: map[string]string{"plane_workspace": "acme", "plane_project_id": "p-1"}})
	store.addProject(tenantA, &project.Project{ID: "proj-local", WorkspacePath: "/srv/x"})
	hub := &tenantBroadcaster{}
	syncer := &recordingSyncer{done: make(chan struct{}, 8)}
	svc := NewWebhookService(store, testWebhookKey, NewVCSWebhookService(hub), NewPMWebhookService(hub, syncer, nil), time.Hour)
	return &webhookEnv{store: store, svc: svc, syncer: syncer, hub: hub}
}

func inTenant(tenantID string) context.Context {
	return tenantctx.WithTenant(context.Background(), tenantID)
}

func (env *webhookEnv) register(t *testing.T, tenantID, projectID string, req webhook.CreateRequest) *webhook.Registered {
	t.Helper()
	reg, err := env.svc.Register(inTenant(tenantID), projectID, &req)
	if err != nil {
		t.Fatalf("Register %s %s/%s: %v", projectID, req.Kind, req.Provider, err)
	}
	return reg
}

const githubPush = `{"ref":"refs/heads/main","after":"bbb","repository":{"full_name":"acme/app","html_url":"https://github.com/acme/app"},"commits":[]}`

func signedGitHub(secret, eventType, deliveryID, body string) *webhook.Delivery {
	return &webhook.Delivery{Event: eventType, DeliveryID: deliveryID, Signature: githubSignature(secret, []byte(body)), Body: []byte(body)}
}

func TestWebhooks_RegisterShowsTheSecretOnce(t *testing.T) {
	env := newWebhookEnv(t)
	reg := env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"})

	if len(reg.Secret) != 64 || strings.Trim(reg.Secret, "0123456789abcdef") != "" {
		t.Fatalf("secret %q, want 32 random bytes in hex", reg.Secret)
	}
	if reg.URL != "/api/v1/webhooks/vcs/github/"+reg.ID || reg.ProjectID != "proj-a" {
		t.Fatalf("registered %+v", reg.Endpoint)
	}
	// Stored encrypted with the webhook key, never in clear text.
	stored := env.store.endpoints[reg.ID]
	if strings.Contains(string(stored.EncryptedSecret), reg.Secret) {
		t.Fatal("the secret is stored in clear text")
	}
	if plain, err := crypto.Decrypt(stored.EncryptedSecret, testWebhookKey); err != nil || string(plain) != reg.Secret {
		t.Fatalf("stored secret decrypts to %q, %v", plain, err)
	}
	// Another registration gets another secret and ID.
	other := env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindPM, Provider: "github"})
	if other.Secret == reg.Secret || other.ID == reg.ID {
		t.Fatal("two webhooks share a secret or an ID")
	}

	list, err := env.svc.List(inTenant(tenantA), "proj-a")
	if err != nil || len(list) != 2 {
		t.Fatalf("List = %+v, %v", list, err)
	}
	data, _ := json.Marshal(list)
	for _, leaked := range []string{reg.Secret, other.Secret, `"secret"`} {
		if strings.Contains(string(data), leaked) {
			t.Fatalf("the list shows a secret: %s", data)
		}
	}
	for i := range list {
		if list[i].URL == "" {
			t.Fatalf("listed webhook without its URL: %+v", list[i])
		}
	}
}

func TestWebhooks_RegisterChecksProjectAndRequest(t *testing.T) {
	env := newWebhookEnv(t)
	tests := []struct {
		name      string
		tenantID  string
		projectID string
		req       webhook.CreateRequest
		want      error
	}{
		{"project of another tenant", tenantB, "proj-a", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"}, domain.ErrNotFound},
		{"unknown project", tenantA, "nope", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"}, domain.ErrNotFound},
		{"invalid provider", tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "plane"}, domain.ErrValidation},
		{"token on a vcs webhook", tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github", APIToken: "x"}, domain.ErrValidation},
		{"repository webhook for a project without a repository", tenantA, "proj-local", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"}, domain.ErrValidation},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := env.svc.Register(inTenant(tc.tenantID), tc.projectID, &tc.req); !errors.Is(err, tc.want) {
				t.Fatalf("Register = %v, want %v", err, tc.want)
			}
		})
	}
	env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"})
	if _, err := env.svc.Register(inTenant(tenantA), "proj-a", &webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("a second webhook of the same kind and provider = %v, want ErrConflict", err)
	}
}

// Two tenants with a project of the same repository, each with its own
// webhook: an event signed for A acts only in A, never in B.
func TestWebhooks_EventSignedForATenantActsOnlyInThatTenant(t *testing.T) {
	env := newWebhookEnv(t)
	pmA := env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindPM, Provider: "github", APIToken: "ghp_a"})
	pmB := env.register(t, tenantB, "proj-b", webhook.CreateRequest{Kind: webhook.KindPM, Provider: "github", APIToken: "ghp_b"})
	vcsA := env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"})
	vcsB := env.register(t, tenantB, "proj-b", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"})

	// The webhook's request carries another tenant (as an HTTP layer that
	// honoured a header would set): the webhook's own tenant wins.
	ctx := inTenant(tenantB)
	res, err := env.svc.Receive(ctx, webhook.KindPM, "github", pmA.ID, signedGitHub(pmA.Secret, "issues", "d-1", githubIssueEvent))
	if err != nil || res.Status != webhook.InboundAccepted {
		t.Fatalf("Receive = %+v, %v", res, err)
	}
	got, tenant := env.syncer.waitCall(t)
	if got.ProjectID != "proj-a" || tenant != tenantA || got.ProviderConfig["token"] != "ghp_a" {
		t.Fatalf("sync of project %s in tenant %s with %v, want proj-a in tenant A with A's token", got.ProjectID, tenant, got.ProviderConfig)
	}

	// A's signature on B's webhook: refused, nothing happens in B.
	if _, err := env.svc.Receive(ctx, webhook.KindPM, "github", pmB.ID, signedGitHub(pmA.Secret, "issues", "d-2", githubIssueEvent)); !errors.Is(err, ErrWebhookUnauthorized) {
		t.Fatalf("A-signed event on B's webhook = %v, want ErrWebhookUnauthorized", err)
	}
	env.syncer.assertNoSync(t)

	if _, err := env.svc.Receive(ctx, webhook.KindVCS, "github", vcsA.ID, signedGitHub(vcsA.Secret, "push", "d-3", githubPush)); err != nil {
		t.Fatalf("push on A: %v", err)
	}
	if _, err := env.svc.Receive(ctx, webhook.KindVCS, "github", vcsB.ID, signedGitHub(vcsA.Secret, "push", "d-4", githubPush)); !errors.Is(err, ErrWebhookUnauthorized) {
		t.Fatalf("A-signed push on B's webhook = %v, want ErrWebhookUnauthorized", err)
	}
	if pushes := env.hub.tenantsOf(event.EventVCSPush); len(pushes) != 1 || pushes[0] != tenantA {
		t.Fatalf("push events in tenants %v, want one in tenant A", pushes)
	}
}

// One answer for every failure, so a caller learns nothing about which
// webhook IDs exist (no ID oracle).
func TestWebhooks_UnauthorizedIsUniform(t *testing.T) {
	env := newWebhookEnv(t)
	gh := env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindPM, Provider: "github"})
	other := env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"})
	body := []byte(githubIssueEvent)
	tests := []struct {
		name     string
		kind     webhook.Kind
		provider string
		id       string
		sig      string
	}{
		{"unknown ID", webhook.KindPM, "github", uuid.NewString(), githubSignature(gh.Secret, body)},
		{"ID that is no UUID", webhook.KindPM, "github", "1 OR 1=1", githubSignature(gh.Secret, body)},
		{"empty ID", webhook.KindPM, "github", "", githubSignature(gh.Secret, body)},
		{"upper-case ID", webhook.KindPM, "github", strings.ToUpper(gh.ID), githubSignature(gh.Secret, body)},
		{"wrong signature", webhook.KindPM, "github", gh.ID, githubSignature("wrong", body)},
		{"no signature", webhook.KindPM, "github", gh.ID, ""},
		{"signature without the sha256= prefix", webhook.KindPM, "github", gh.ID, strings.TrimPrefix(githubSignature(gh.Secret, body), "sha256=")},
		{"signature that is no hex", webhook.KindPM, "github", gh.ID, "sha256=zz"},
		{"another webhook's secret", webhook.KindPM, "github", gh.ID, githubSignature(other.Secret, body)},
		{"right signature on the wrong provider path", webhook.KindPM, "gitlab", gh.ID, gh.Secret},
		{"right signature on the wrong kind path", webhook.KindVCS, "github", gh.ID, githubSignature(gh.Secret, body)},
		{"unknown provider", webhook.KindPM, "bitbucket", gh.ID, githubSignature(gh.Secret, body)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := env.svc.Receive(context.Background(), tc.kind, tc.provider, tc.id,
				&webhook.Delivery{Event: "issues", DeliveryID: "d", Signature: tc.sig, Body: body})
			if !errors.Is(err, ErrWebhookUnauthorized) || res != nil {
				t.Fatalf("Receive = %+v, %v; want ErrWebhookUnauthorized", res, err)
			}
			if err.Error() != ErrWebhookUnauthorized.Error() {
				t.Fatalf("error %q names more than the uniform answer", err)
			}
		})
	}
	env.syncer.assertNoSync(t)
	if len(env.store.deliveries) != 0 {
		t.Fatalf("a refused delivery was recorded: %v", env.store.deliveries)
	}
}

// GitLab sends the secret as X-Gitlab-Token, Plane an HMAC in hex.
func TestWebhooks_GitLabTokenAndPlaneSignature(t *testing.T) {
	env := newWebhookEnv(t)
	gl := env.register(t, tenantA, "proj-gl", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "gitlab"})
	push := []byte(`{"ref":"refs/heads/main","project":{"path_with_namespace":"group/app","web_url":"https://gitlab.example.com/group/app"}}`)
	if _, err := env.svc.Receive(context.Background(), webhook.KindVCS, "gitlab", gl.ID, &webhook.Delivery{Event: "Push Hook", Signature: gl.Secret, Body: push}); err != nil {
		t.Fatalf("gitlab push: %v", err)
	}
	for _, bad := range []string{"", gl.Secret[:63], gl.Secret + "0", strings.ToUpper(gl.Secret)} {
		if _, err := env.svc.Receive(context.Background(), webhook.KindVCS, "gitlab", gl.ID, &webhook.Delivery{Event: "Push Hook", Signature: bad, Body: push}); !errors.Is(err, ErrWebhookUnauthorized) {
			t.Fatalf("gitlab token %q = %v, want ErrWebhookUnauthorized", bad, err)
		}
	}

	// Plane generates the signing secret of its webhooks; the admin gives
	// it to CodeForge.
	const planeSecret = "plane_wh_secret_0123456789abcdef"
	pl := env.register(t, tenantA, "proj-pl", webhook.CreateRequest{Kind: webhook.KindPM, Provider: "plane", APIToken: "plane_a", Secret: planeSecret})
	if pl.Secret != planeSecret {
		t.Fatalf("registered plane secret %q, want Plane's", pl.Secret)
	}
	body := []byte(planeIssueEvent)
	res, err := env.svc.Receive(context.Background(), webhook.KindPM, "plane", pl.ID, &webhook.Delivery{Signature: planeSignature(planeSecret, body), Body: body})
	if err != nil || res.Status != webhook.InboundAccepted {
		t.Fatalf("plane event = %+v, %v", res, err)
	}
	if got, tenant := env.syncer.waitCall(t); got.ProjectID != "proj-pl" || got.ProviderConfig["api_token"] != "plane_a" || tenant != tenantA {
		t.Fatalf("plane sync %+v in %s", got, tenant)
	}
	if _, err := env.svc.Receive(context.Background(), webhook.KindPM, "plane", pl.ID, &webhook.Delivery{Signature: planeSignature("x", body), Body: body}); !errors.Is(err, ErrWebhookUnauthorized) {
		t.Fatalf("wrong plane signature = %v", err)
	}
}

// An event for another repository than the webhook's project is ignored
// (and logged); a substring of the repository name is another repository.
func TestWebhooks_EventForAnotherRepositoryIsIgnored(t *testing.T) {
	env := newWebhookEnv(t)
	vcs := env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"})
	pm := env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindPM, Provider: "github", APIToken: "t"})
	for i, name := range []string{"acme/ap", "acme/app-private", "evil/acme/app", "acme"} {
		push := `{"ref":"refs/heads/main","repository":{"full_name":"` + name + `"}}`
		res, err := env.svc.Receive(context.Background(), webhook.KindVCS, "github", vcs.ID, signedGitHub(vcs.Secret, "push", "p"+string(rune('0'+i)), push))
		if err != nil || res.Status != webhook.InboundIgnored || res.Reason == "" {
			t.Fatalf("push for %s = %+v, %v; want ignored with a reason", name, res, err)
		}
		issue := `{"action":"opened","issue":{"number":1},"repository":{"full_name":"` + name + `"}}`
		res, err = env.svc.Receive(context.Background(), webhook.KindPM, "github", pm.ID, signedGitHub(pm.Secret, "issues", "i"+string(rune('0'+i)), issue))
		if err != nil || res.Status != webhook.InboundIgnored {
			t.Fatalf("issue event for %s = %+v, %v; want ignored", name, res, err)
		}
	}
	if env.hub.count() != 0 {
		t.Fatalf("events broadcast for other repositories: %v", env.hub.types)
	}
	env.syncer.assertNoSync(t)
}

func TestWebhooks_EventsAWebhookDoesNotHandleAreIgnored(t *testing.T) {
	env := newWebhookEnv(t)
	vcs := env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"})
	for _, ev := range []string{"ping", "issues", ""} {
		body := `{"zen":"` + ev + `"}` // one body per event: a body is one delivery
		res, err := env.svc.Receive(context.Background(), webhook.KindVCS, "github", vcs.ID, signedGitHub(vcs.Secret, ev, "", body))
		if err != nil || res.Status != webhook.InboundIgnored || res.Event != ev {
			t.Fatalf("event %q = %+v, %v; want ignored", ev, res, err)
		}
	}
}

// Rotation replaces the secret: the old one stops working at once. Only the
// webhook's own tenant can rotate it.
func TestWebhooks_RotateSecret(t *testing.T) {
	env := newWebhookEnv(t)
	reg := env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"})
	if _, err := env.svc.RotateSecret(inTenant(tenantB), "proj-a", reg.ID, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("rotation by another tenant = %v, want ErrNotFound", err)
	}
	if _, err := env.svc.RotateSecret(inTenant(tenantA), "proj-gl", reg.ID, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("rotation under another project = %v, want ErrNotFound", err)
	}
	rotated, err := env.svc.RotateSecret(inTenant(tenantA), "proj-a", reg.ID, "")
	if err != nil || rotated.Secret == reg.Secret || rotated.ID != reg.ID || len(rotated.Secret) != 64 {
		t.Fatalf("RotateSecret = %+v, %v", rotated, err)
	}
	if _, err := env.svc.Receive(context.Background(), webhook.KindVCS, "github", reg.ID, signedGitHub(reg.Secret, "push", "r1", githubPush)); !errors.Is(err, ErrWebhookUnauthorized) {
		t.Fatalf("old secret after rotation = %v, want ErrWebhookUnauthorized", err)
	}
	if _, err := env.svc.Receive(context.Background(), webhook.KindVCS, "github", reg.ID, signedGitHub(rotated.Secret, "push", "r2", githubPush)); err != nil {
		t.Fatalf("new secret: %v", err)
	}
	// CodeForge generates GitHub's and GitLab's secrets: none is taken.
	if _, err := env.svc.RotateSecret(inTenant(tenantA), "proj-a", reg.ID, "chosen-by-the-caller-0123"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("rotation to a given secret = %v, want ErrValidation", err)
	}
}

// Plane regenerates its webhook's secret itself: the rotation takes the new
// one and needs it.
func TestWebhooks_RotatePlaneSecret(t *testing.T) {
	env := newWebhookEnv(t)
	pl := env.register(t, tenantA, "proj-pl", webhook.CreateRequest{Kind: webhook.KindPM, Provider: "plane", APIToken: "t", Secret: "plane_wh_old_0123456789"})
	for _, bad := range []string{"", "short", "plane secret with spaces"} {
		if _, err := env.svc.RotateSecret(inTenant(tenantA), "proj-pl", pl.ID, bad); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("plane rotation to %q = %v, want ErrValidation", bad, err)
		}
	}
	rotated, err := env.svc.RotateSecret(inTenant(tenantA), "proj-pl", pl.ID, "plane_wh_new_0123456789")
	if err != nil || rotated.Secret != "plane_wh_new_0123456789" {
		t.Fatalf("RotateSecret = %+v, %v", rotated, err)
	}
	body := []byte(planeIssueEvent)
	if _, err := env.svc.Receive(context.Background(), webhook.KindPM, "plane", pl.ID, &webhook.Delivery{Signature: planeSignature("plane_wh_old_0123456789", body), Body: body}); !errors.Is(err, ErrWebhookUnauthorized) {
		t.Fatalf("old plane secret = %v, want ErrWebhookUnauthorized", err)
	}
	if res, err := env.svc.Receive(context.Background(), webhook.KindPM, "plane", pl.ID, &webhook.Delivery{Signature: planeSignature("plane_wh_new_0123456789", body), Body: body}); err != nil || res.Status != webhook.InboundAccepted {
		t.Fatalf("new plane secret = %+v, %v", res, err)
	}
	env.syncer.waitCall(t)
}

// A delivery is handled once within the retention: a provider's redelivery
// (same delivery ID) and a replay of a signed delivery (same body, whatever
// delivery-ID header it carries - the signature covers only the body) are
// duplicates. A delivery that failed can be redelivered.
func TestWebhooks_ReplayIsDeduplicated(t *testing.T) {
	env := newWebhookEnv(t)
	pm := env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindPM, Provider: "github", APIToken: "t"})
	issue := func(number int) string {
		return `{"action":"opened","issue":{"number":` + strconv.Itoa(number) + `},"repository":{"full_name":"acme/app"}}`
	}
	receive := func(d *webhook.Delivery) (*webhook.InboundResult, error) {
		return env.svc.Receive(context.Background(), webhook.KindPM, "github", pm.ID, d)
	}

	first := signedGitHub(pm.Secret, "issues", "delivery-1", issue(1))
	if res, err := receive(first); err != nil || res.Status != webhook.InboundAccepted {
		t.Fatalf("first delivery = %+v, %v", res, err)
	}
	env.syncer.waitCall(t)
	for name, replay := range map[string]*webhook.Delivery{
		"redelivery (same delivery ID)":             first,
		"same delivery ID, changed body":            signedGitHub(pm.Secret, "issues", "delivery-1", issue(2)),
		"replay with a fresh delivery ID":           signedGitHub(pm.Secret, "issues", uuid.NewString(), issue(1)),
		"replay without a delivery ID":              signedGitHub(pm.Secret, "issues", "", issue(1)),
		"replay under another event type":           signedGitHub(pm.Secret, "push", "", issue(1)),
		"replay with an over-long delivery ID":      signedGitHub(pm.Secret, "issues", strings.Repeat("x", 500), issue(1)),
		"replay with a delivery ID of a body claim": signedGitHub(pm.Secret, "issues", "body:"+strings.Repeat("0", 64), issue(1)),
	} {
		res, err := receive(replay)
		if err != nil || res.Status != webhook.InboundDuplicate {
			t.Errorf("%s = %+v, %v; want duplicate", name, res, err)
		}
	}
	env.syncer.assertNoSync(t)

	// The same delivery ID and body on another webhook is another delivery.
	vcs := env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"})
	if res, err := env.svc.Receive(context.Background(), webhook.KindVCS, "github", vcs.ID, signedGitHub(vcs.Secret, "push", "delivery-1", githubPush)); err != nil || res.Status != webhook.InboundProcessed {
		t.Fatalf("same delivery ID on another webhook = %+v, %v", res, err)
	}

	// A delivery that fails is released: its redelivery runs.
	broken := signedGitHub(pm.Secret, "issues", "delivery-2", `{"repository":`)
	if _, err := receive(broken); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("broken payload = %v, want ErrValidation", err)
	}
	if res, err := receive(broken); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("redelivered broken payload = %+v, %v, want ErrValidation again", res, err)
	}
	if res, err := receive(signedGitHub(pm.Secret, "issues", "delivery-2", issue(3))); err != nil || res.Status != webhook.InboundAccepted {
		t.Fatalf("redelivery after a failure = %+v, %v", res, err)
	}
	env.syncer.waitCall(t)

	// Without a delivery ID a delivery is still handled once.
	if res, err := receive(signedGitHub(pm.Secret, "issues", "", issue(4))); err != nil || res.Status != webhook.InboundAccepted {
		t.Fatalf("delivery without an ID = %+v, %v", res, err)
	}
	env.syncer.waitCall(t)
	if res, err := receive(signedGitHub(pm.Secret, "issues", "", issue(4))); err != nil || res.Status != webhook.InboundDuplicate {
		t.Fatalf("its replay = %+v, %v; want duplicate", res, err)
	}
	env.syncer.assertNoSync(t)
}

// An event that names another repository is ignored and not remembered:
// once the project's repository URL is corrected, the provider's
// redelivery of that event is handled.
func TestWebhooks_IgnoredMismatchIsHandledAfterTheRepositoryIsCorrected(t *testing.T) {
	env := newWebhookEnv(t)
	env.store.addProject(tenantA, &project.Project{ID: "proj-old", RepoURL: "https://github.com/acme/app-old.git"})
	vcs := env.register(t, tenantA, "proj-old", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"})
	push := signedGitHub(vcs.Secret, "push", "delivery-1", githubPush)
	if res, err := env.svc.Receive(context.Background(), webhook.KindVCS, "github", vcs.ID, push); err != nil || res.Status != webhook.InboundIgnored {
		t.Fatalf("push for another repository = %+v, %v; want ignored", res, err)
	}

	env.store.addProject(tenantA, &project.Project{ID: "proj-old", RepoURL: "https://github.com/acme/app.git"})
	res, err := env.svc.Receive(context.Background(), webhook.KindVCS, "github", vcs.ID, push)
	if err != nil || res.Status != webhook.InboundProcessed {
		t.Fatalf("redelivery after the repository URL was corrected = %+v, %v; want processed", res, err)
	}
	if pushes := env.hub.tenantsOf(event.EventVCSPush); len(pushes) != 1 {
		t.Fatalf("push events %v, want one", pushes)
	}
}

func TestWebhooks_APITokenOnlyOnPMWebhooks(t *testing.T) {
	env := newWebhookEnv(t)
	pm := env.register(t, tenantA, "proj-gl", webhook.CreateRequest{Kind: webhook.KindPM, Provider: "gitlab"})
	vcs := env.register(t, tenantA, "proj-gl", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "gitlab"})
	ctx := inTenant(tenantA)

	if err := env.svc.SetAPIToken(ctx, "proj-gl", pm.ID, "glpat-new"); err != nil {
		t.Fatalf("SetAPIToken: %v", err)
	}
	stored := env.store.endpoints[pm.ID].EncryptedAPIToken
	if plain, err := crypto.Decrypt(stored, testWebhookKey); err != nil || string(plain) != "glpat-new" {
		t.Fatalf("stored token decrypts to %q, %v", plain, err)
	}
	list, _ := env.svc.List(ctx, "proj-gl")
	for i := range list {
		if list[i].ID == pm.ID && !list[i].HasAPIToken {
			t.Fatalf("listed webhook %+v has no has_api_token", list[i])
		}
	}
	if err := env.svc.SetAPIToken(ctx, "proj-gl", pm.ID, ""); err != nil || env.store.endpoints[pm.ID].EncryptedAPIToken != nil {
		t.Fatalf("clearing the token: %v, stored %v", err, env.store.endpoints[pm.ID].EncryptedAPIToken)
	}
	for name, call := range map[string]func() error{
		"vcs webhook":    func() error { return env.svc.SetAPIToken(ctx, "proj-gl", vcs.ID, "t") },
		"invalid token":  func() error { return env.svc.SetAPIToken(ctx, "proj-gl", pm.ID, "a b") },
		"another tenant": func() error { return env.svc.SetAPIToken(inTenant(tenantB), "proj-gl", pm.ID, "t") },
	} {
		if err := call(); err == nil {
			t.Fatalf("%s: SetAPIToken succeeded", name)
		}
	}
}

func TestWebhooks_DeleteStopsDeliveries(t *testing.T) {
	env := newWebhookEnv(t)
	reg := env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"})
	if err := env.svc.Delete(inTenant(tenantB), "proj-a", reg.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete by another tenant = %v, want ErrNotFound", err)
	}
	if err := env.svc.Delete(inTenant(tenantA), "proj-a", reg.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := env.svc.Receive(context.Background(), webhook.KindVCS, "github", reg.ID, signedGitHub(reg.Secret, "push", "x", githubPush)); !errors.Is(err, ErrWebhookUnauthorized) {
		t.Fatalf("delivery to a deleted webhook = %v, want ErrWebhookUnauthorized", err)
	}
}

// A secret that cannot be decrypted (auth.jwt_secret changed) refuses the
// delivery like a wrong signature.
func TestWebhooks_UndecryptableSecretRefuses(t *testing.T) {
	env := newWebhookEnv(t)
	reg := env.register(t, tenantA, "proj-a", webhook.CreateRequest{Kind: webhook.KindVCS, Provider: "github"})
	svc := NewWebhookService(env.store, []byte("another-key-another-key-another!"), env.svc.vcs, env.svc.pm, time.Hour)
	if _, err := svc.Receive(context.Background(), webhook.KindVCS, "github", reg.ID, signedGitHub(reg.Secret, "push", "x", githubPush)); !errors.Is(err, ErrWebhookUnauthorized) {
		t.Fatalf("Receive = %v, want ErrWebhookUnauthorized", err)
	}
}

// A delivery ID is stored as sent when it is a short printable ID (the
// providers send UUIDs), otherwise as its SHA-256: storable whatever the
// header holds, and still one key per delivery.
func TestStoredDeliveryID(t *testing.T) {
	const guid = "72d3162e-cc78-11e3-81ab-4c9367dc0958"
	long := strings.Repeat("a", maxDeliveryIDLength+1)
	tests := []struct {
		id     string
		hashed bool
	}{
		{"", false},
		{guid, false},
		{strings.Repeat("a", maxDeliveryIDLength), false},
		{long, true},
		{"a\xffb", true},
		{"a\x00b", true},
		{"with space", true},
		{"zeile\nzwei", true},
	}
	for _, tc := range tests {
		got := storedDeliveryID(tc.id)
		if tc.hashed {
			if !strings.HasPrefix(got, "sha256:") || len(got) != len("sha256:")+64 || got != storedDeliveryID(tc.id) {
				t.Fatalf("storedDeliveryID(%q) = %q, want a stable sha256 key", tc.id, got)
			}
			continue
		}
		if got != tc.id {
			t.Fatalf("storedDeliveryID(%q) = %q, want it unchanged", tc.id, got)
		}
	}
	if storedDeliveryID(long) == storedDeliveryID(long+"b") {
		t.Fatal("two long IDs share a key")
	}
}
