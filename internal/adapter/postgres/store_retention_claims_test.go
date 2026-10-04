package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/domain/webhook"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// KI-90: the bookkeeping rows that make at-least-once deliveries idempotent
// were never purged. handoff_claims has no foreign key, so a row stayed for
// every handoff stage ever carried out; a webhook's delivery claims were
// pruned only on its next delivery. The retention job now deletes claims
// whose stage was done before the cutoff, and delivery claims older than
// their dedup window, across all tenants.

var (
	purgeHandoffClaims     purgeMethod = func(p database.RetentionPurger) purgeFunc { return p.DeleteExpiredHandoffClaims }
	purgeWebhookDeliveries purgeMethod = func(p database.RetentionPurger) purgeFunc { return p.DeleteExpiredWebhookDeliveries }
)

// handoffClaim claims a stage of a new handoff in the fixture's tenant and
// returns the handoff ID.
func (f *statusFixture) handoffClaim(t *testing.T, stage string) string {
	t.Helper()
	id := "retention-" + uuid.NewString()
	claim, err := f.store.ClaimHandoff(f.ctx, id, stage, time.Minute)
	if err != nil || !claim.Claimed {
		t.Fatalf("ClaimHandoff = %+v, %v", claim, err)
	}
	return id
}

// doneHandoffClaim is handoffClaim with the stage done doneAgo back.
func (f *statusFixture) doneHandoffClaim(t *testing.T, pool *pgxpool.Pool, stage, doneAgo string) string {
	t.Helper()
	id := f.handoffClaim(t, stage)
	if err := f.store.FinishHandoff(f.ctx, id, stage); err != nil {
		t.Fatalf("FinishHandoff: %v", err)
	}
	tag, err := pool.Exec(context.Background(),
		`UPDATE handoff_claims SET claimed_at = now() - $2::interval, done_at = now() - $3::interval WHERE handoff_id = $1`,
		id, expired, doneAgo)
	backdated(t, tag, err)
	return id
}

func handoffClaimExists(t *testing.T, pool *pgxpool.Pool, handoffID string) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM handoff_claims WHERE handoff_id = $1)`, handoffID).Scan(&exists); err != nil {
		t.Fatalf("handoff claim %s: %v", handoffID, err)
	}
	return exists
}

func TestStore_DeleteExpiredHandoffClaims(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)

	doneOldA := a.doneHandoffClaim(t, pool, "request", expired)
	doneOldB := b.doneHandoffClaim(t, pool, "approved", expired)
	doneInsideA := a.doneHandoffClaim(t, pool, "request", inside)
	doneFreshB := b.doneHandoffClaim(t, pool, "request", fresh)

	// A claim never done may still be carried out: a redelivery takes it
	// over (and reuses its task) after its lease, a released one at once
	// (claimed_at -infinity). It is not purged, however old.
	undoneOldA := a.handoffClaim(t, "request")
	tag, err := pool.Exec(context.Background(),
		`UPDATE handoff_claims SET claimed_at = now() - $2::interval WHERE handoff_id = $1`, undoneOldA, expired)
	backdated(t, tag, err)
	releasedA := a.handoffClaim(t, "request")
	if err := a.store.ReleaseHandoff(a.ctx, releasedA, "request"); err != nil {
		t.Fatalf("ReleaseHandoff: %v", err)
	}

	purgeAll(t, a.store, "DeleteExpiredHandoffClaims", purgeHandoffClaims)

	for name, tc := range map[string]struct {
		id   string
		kept bool
	}{
		"done, expired":                {doneOldA, false},
		"other tenant, done, expired":  {doneOldB, false},
		"done inside retention":        {doneInsideA, true},
		"other tenant, done, fresh":    {doneFreshB, true},
		"never done, claimed long ago": {undoneOldA, true},
		"released":                     {releasedA, true},
	} {
		if got := handoffClaimExists(t, pool, tc.id); got != tc.kept {
			t.Errorf("%s: kept = %v, want %v", name, got, tc.kept)
		}
	}
}

// webhookWithDeliveries registers a webhook in the fixture's tenant and
// claims one delivery key per entry of receivedAgo, received that long ago;
// it returns the keys in the same order.
func (f *statusFixture) webhookWithDeliveries(t *testing.T, pool *pgxpool.Pool, receivedAgo ...string) (webhookID string, keys []string) {
	t.Helper()
	ep, err := f.store.CreateWebhookEndpoint(f.ctx, &webhook.Endpoint{
		ProjectID: f.project.ID, Kind: webhook.KindVCS, Provider: "github", EncryptedSecret: []byte("s"),
	})
	if err != nil {
		t.Fatalf("CreateWebhookEndpoint: %v", err)
	}
	// All keys are claimed before any is backdated: a claim prunes the
	// webhook's claims older than its window.
	for range receivedAgo {
		key := "id:" + uuid.NewString()
		claimed, err := f.store.ClaimWebhookDelivery(f.ctx, ep.ID, []string{key}, time.Hour)
		if err != nil || !claimed {
			t.Fatalf("ClaimWebhookDelivery = %v, %v", claimed, err)
		}
		keys = append(keys, key)
	}
	for i, ago := range receivedAgo {
		tag, err := pool.Exec(context.Background(),
			`UPDATE webhook_deliveries SET received_at = now() - $3::interval WHERE webhook_id = $1 AND delivery_key = $2`,
			ep.ID, keys[i], ago)
		backdated(t, tag, err)
	}
	return ep.ID, keys
}

func webhookDeliveryExists(t *testing.T, pool *pgxpool.Pool, webhookID, key string) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM webhook_deliveries WHERE webhook_id = $1 AND delivery_key = $2)`,
		webhookID, key).Scan(&exists); err != nil {
		t.Fatalf("webhook delivery %s: %v", key, err)
	}
	return exists
}

// A quiet webhook's delivery claims are purged by the job, not only on the
// webhook's next delivery; the webhook itself stays.
func TestStore_DeleteExpiredWebhookDeliveries(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)

	whA, keysA := a.webhookWithDeliveries(t, pool, expired, inside, fresh)
	whB, keysB := b.webhookWithDeliveries(t, pool, expired, fresh)

	purgeAll(t, a.store, "DeleteExpiredWebhookDeliveries", purgeWebhookDeliveries)

	for name, tc := range map[string]struct {
		webhook, key string
		kept         bool
	}{
		"expired":               {whA, keysA[0], false},
		"inside the window":     {whA, keysA[1], true},
		"fresh":                 {whA, keysA[2], true},
		"other tenant, expired": {whB, keysB[0], false},
		"other tenant, fresh":   {whB, keysB[1], true},
	} {
		if got := webhookDeliveryExists(t, pool, tc.webhook, tc.key); got != tc.kept {
			t.Errorf("%s: kept = %v, want %v", name, got, tc.kept)
		}
	}
	if _, err := a.store.GetWebhookEndpoint(a.ctx, a.project.ID, whA); err != nil {
		t.Errorf("webhook removed with its deliveries: %v", err)
	}
}

func TestRetentionClaimQueries_IntentionallyCrossTenant(t *testing.T) {
	src := readStoreSource(t, "store_retention.go")
	for _, name := range []string{"DeleteExpiredHandoffClaims", "DeleteExpiredWebhookDeliveries"} {
		if doc := methodDocComment(t, src, "store_retention.go", name); !strings.Contains(doc, "INTENTIONALLY CROSS-TENANT") {
			t.Errorf("%s must document why it is intentionally cross-tenant", name)
		}
	}
}
