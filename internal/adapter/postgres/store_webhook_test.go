package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/webhook"
)

func createWebhookProject(ctx context.Context, t *testing.T, store *postgres.Store) *project.Project {
	t.Helper()
	p, err := store.CreateProject(ctx, &project.CreateRequest{Name: "wh-" + uuid.NewString()[:8], RepoURL: "https://github.com/acme/app.git", Provider: "github"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteProject(ctx, p.ID) })
	return p
}

// KI-85: webhooks are tenant-scoped. Only the lookup by ID that an inbound
// delivery needs crosses tenants - it returns the webhook's tenant, in which
// the delivery is then handled.
func TestStore_WebhookEndpoints_TenantScoped(t *testing.T) {
	store := setupStore(t)
	tenantA, tenantB := createTestTenant(t, store), createTestTenant(t, store)
	ctxA, ctxB := ctxWithTenant(t, tenantA), ctxWithTenant(t, tenantB)
	projA := createWebhookProject(ctxA, t, store)

	created, err := store.CreateWebhookEndpoint(ctxA, &webhook.Endpoint{
		ProjectID: projA.ID, Kind: webhook.KindPM, Provider: "gitlab",
		EncryptedSecret: []byte("sealed-secret"), EncryptedAPIToken: []byte("sealed-token"),
	})
	if err != nil {
		t.Fatalf("CreateWebhookEndpoint: %v", err)
	}
	if created.ID == "" || created.TenantID != tenantA || created.CreatedAt.IsZero() || created.SecretRotatedAt.IsZero() {
		t.Fatalf("created %+v", created)
	}

	// Another tenant cannot register a webhook for A's project.
	if _, err := store.CreateWebhookEndpoint(ctxB, &webhook.Endpoint{ProjectID: projA.ID, Kind: webhook.KindVCS, Provider: "github", EncryptedSecret: []byte("s")}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("create for another tenant's project = %v, want ErrNotFound", err)
	}
	// One webhook per project, kind and provider.
	if _, err := store.CreateWebhookEndpoint(ctxA, &webhook.Endpoint{ProjectID: projA.ID, Kind: webhook.KindPM, Provider: "gitlab", EncryptedSecret: []byte("s")}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second webhook of the same kind and provider = %v, want ErrConflict", err)
	}

	if list, err := store.ListWebhookEndpoints(ctxA, projA.ID); err != nil || len(list) != 1 || string(list[0].EncryptedAPIToken) != "sealed-token" {
		t.Fatalf("tenant A lists %+v, %v", list, err)
	}
	if list, err := store.ListWebhookEndpoints(ctxB, projA.ID); err != nil || len(list) != 0 {
		t.Fatalf("tenant B lists %+v, %v; want nothing", list, err)
	}
	if _, err := store.GetWebhookEndpoint(ctxB, projA.ID, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get in tenant B = %v, want ErrNotFound", err)
	}
	if _, err := store.GetWebhookEndpoint(ctxA, uuid.NewString(), created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get under another project = %v, want ErrNotFound", err)
	}
	got, err := store.LookupWebhookEndpoint(ctxB, created.ID)
	if err != nil || got.TenantID != tenantA || got.ProjectID != projA.ID || string(got.EncryptedSecret) != "sealed-secret" {
		t.Fatalf("LookupWebhookEndpoint = %+v, %v; want A's webhook with its tenant", got, err)
	}
	if _, err := store.LookupWebhookEndpoint(ctxA, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("lookup of an unknown ID = %v, want ErrNotFound", err)
	}

	// Rotation, token change and deletion only in the webhook's tenant.
	if _, err := store.RotateWebhookSecret(ctxB, projA.ID, created.ID, []byte("x")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("rotate in tenant B = %v, want ErrNotFound", err)
	}
	if err := store.SetWebhookAPIToken(ctxB, projA.ID, created.ID, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("token change in tenant B = %v, want ErrNotFound", err)
	}
	if err := store.DeleteWebhookEndpoint(ctxB, projA.ID, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete in tenant B = %v, want ErrNotFound", err)
	}
	time.Sleep(10 * time.Millisecond)
	rotated, err := store.RotateWebhookSecret(ctxA, projA.ID, created.ID, []byte("new-secret"))
	if err != nil || string(rotated.EncryptedSecret) != "new-secret" || !rotated.SecretRotatedAt.After(created.SecretRotatedAt) {
		t.Fatalf("RotateWebhookSecret = %+v, %v", rotated, err)
	}
	if err := store.SetWebhookAPIToken(ctxA, projA.ID, created.ID, nil); err != nil {
		t.Fatalf("SetWebhookAPIToken: %v", err)
	}
	if got, _ := store.GetWebhookEndpoint(ctxA, projA.ID, created.ID); got == nil || got.EncryptedAPIToken != nil {
		t.Fatalf("token not cleared: %+v", got)
	}
	if err := store.DeleteWebhookEndpoint(ctxA, projA.ID, created.ID); err != nil {
		t.Fatalf("DeleteWebhookEndpoint: %v", err)
	}
	if _, err := store.LookupWebhookEndpoint(ctxA, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("lookup after delete = %v, want ErrNotFound", err)
	}
}

// A project's webhooks go with the project.
func TestStore_WebhookEndpoints_DeletedWithTheProject(t *testing.T) {
	store := setupStore(t)
	ctx := ctxWithTenant(t, createTestTenant(t, store))
	p, err := store.CreateProject(ctx, &project.CreateRequest{Name: "wh-del", RepoURL: "https://github.com/acme/x.git", Provider: "github"})
	if err != nil {
		t.Fatal(err)
	}
	e, err := store.CreateWebhookEndpoint(ctx, &webhook.Endpoint{ProjectID: p.ID, Kind: webhook.KindVCS, Provider: "github", EncryptedSecret: []byte("s")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimWebhookDelivery(ctx, e.ID, []string{"id:d-1"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LookupWebhookEndpoint(ctx, e.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("webhook outlived its project: %v", err)
	}
}

// KI-85 (D6): a delivery claims its keys (body hash, delivery ID) once per
// webhook, all or none: a claim that meets one claimed key records nothing.
// A released claim (failed handling) can be claimed again; claims older than
// the retention are pruned.
func TestStore_WebhookDeliveries(t *testing.T) {
	store := setupStore(t)
	ctx := ctxWithTenant(t, createTestTenant(t, store))
	p := createWebhookProject(ctx, t, store)
	newEndpoint := func(provider string) string {
		t.Helper()
		e, err := store.CreateWebhookEndpoint(ctx, &webhook.Endpoint{ProjectID: p.ID, Kind: webhook.KindVCS, Provider: provider, EncryptedSecret: []byte("s")})
		if err != nil {
			t.Fatal(err)
		}
		return e.ID
	}
	gh, gl := newEndpoint("github"), newEndpoint("gitlab")

	claim := func(webhookID string, retention time.Duration, keys ...string) bool {
		t.Helper()
		ok, err := store.ClaimWebhookDelivery(ctx, webhookID, keys, retention)
		if err != nil {
			t.Fatalf("ClaimWebhookDelivery(%v): %v", keys, err)
		}
		return ok
	}
	if !claim(gh, time.Hour, "body:1", "id:d-1") {
		t.Fatal("first claim refused")
	}
	if claim(gh, time.Hour, "body:1", "id:d-1") {
		t.Fatal("second claim of the same delivery succeeded")
	}
	if claim(gh, time.Hour, "body:1", "id:d-9") {
		t.Fatal("a claim with a claimed body succeeded")
	}
	if claim(gh, time.Hour, "body:9", "id:d-1") {
		t.Fatal("a claim with a claimed delivery ID succeeded")
	}
	// The refused claims recorded none of their keys.
	if !claim(gh, time.Hour, "id:d-9", "body:9", "body:9") {
		t.Fatal("a key of a refused claim was recorded")
	}
	if !claim(gl, time.Hour, "body:1", "id:d-1") {
		t.Fatal("the same delivery on another webhook was refused")
	}
	if !claim(gl, time.Hour) {
		t.Fatal("a claim without keys was refused")
	}
	if err := store.ReleaseWebhookDelivery(ctx, gh, []string{"body:1", "id:d-1"}); err != nil {
		t.Fatalf("ReleaseWebhookDelivery: %v", err)
	}
	if !claim(gh, time.Hour, "body:1", "id:d-1") {
		t.Fatal("a released delivery could not be claimed again")
	}
	// Pruning: a claim older than the retention is gone after the next
	// claim on that webhook.
	time.Sleep(20 * time.Millisecond)
	if !claim(gh, 10*time.Millisecond, "id:d-2") {
		t.Fatal("claim d-2 refused")
	}
	if !claim(gh, time.Hour, "body:1", "id:d-1") {
		t.Fatal("an expired delivery was not pruned")
	}
	// Other tenants cannot release a webhook's claims.
	other := ctxWithTenant(t, createTestTenant(t, store))
	if err := store.ReleaseWebhookDelivery(other, gh, []string{"body:1", "id:d-1"}); err != nil {
		t.Fatalf("ReleaseWebhookDelivery in another tenant: %v", err)
	}
	if claim(gh, time.Hour, "id:d-1") {
		t.Fatal("another tenant released the webhook's claim")
	}
}
