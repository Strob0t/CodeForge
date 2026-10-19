package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
)

// TestStore_QuarantineRelease (KI-71 review): the replay of an approved
// quarantine message is found by its exact payload and subject in its own
// tenant only, and consumed once.
func TestStore_QuarantineRelease(t *testing.T) {
	store := setupStore(t)
	ctx := ctxWithTenant(t, createTestTenant(t, store))
	other := ctxWithTenant(t, createTestTenant(t, store))
	const subject = "handoff.approved"
	payload := []byte(`{"handoff_id":"` + uuid.NewString() + `"}`)

	release := func(ctx context.Context, subject string, payload []byte) (string, error) {
		t.Helper()
		return store.UnconsumedQuarantineRelease(ctx, subject, payload)
	}
	wantNone := func(ctx context.Context, subject string, payload []byte) {
		t.Helper()
		if id, err := release(ctx, subject, payload); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("UnconsumedQuarantineRelease = %q, %v; want domain.ErrNotFound", id, err)
		}
	}

	pending := holdQuarantined(ctx, t, store, subject, payload, quarantine.StatusPending)
	wantNone(ctx, subject, payload)
	holdQuarantined(other, t, store, subject, payload, quarantine.StatusApproved)
	wantNone(ctx, subject, payload)

	approved := holdQuarantined(ctx, t, store, subject, payload, quarantine.StatusApproved)
	if id, err := release(ctx, subject, payload); err != nil || id != approved {
		t.Fatalf("UnconsumedQuarantineRelease = %q, %v; want %q", id, err, approved)
	}
	wantNone(ctx, subject, append(payload[:len(payload):len(payload)], ' '))
	wantNone(ctx, "runs.start", payload)

	if err := store.ConsumeQuarantineRelease(other, approved); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ConsumeQuarantineRelease in another tenant = %v, want domain.ErrNotFound", err)
	}
	if err := store.ConsumeQuarantineRelease(ctx, approved); err != nil {
		t.Fatalf("ConsumeQuarantineRelease: %v", err)
	}
	if err := store.ConsumeQuarantineRelease(ctx, approved); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second ConsumeQuarantineRelease = %v, want domain.ErrConflict", err)
	}
	wantNone(ctx, subject, payload)
	if err := store.ConsumeQuarantineRelease(ctx, pending); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("ConsumeQuarantineRelease of a pending message = %v, want domain.ErrConflict", err)
	}
}

// holdQuarantined stores a quarantined message of subject with payload and,
// unless it stays pending, records its review.
func holdQuarantined(ctx context.Context, t *testing.T, store *postgres.Store, subject string, payload []byte, status quarantine.Status) string {
	t.Helper()
	now := time.Now().UTC()
	msg := &quarantine.Message{
		ProjectID: "proj-release", Subject: subject, Payload: payload,
		RiskFactors: []string{"prompt_injection"}, Status: quarantine.StatusPending,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if err := store.QuarantineMessage(ctx, msg); err != nil {
		t.Fatalf("QuarantineMessage: %v", err)
	}
	if status != quarantine.StatusPending {
		if err := store.UpdateQuarantineStatus(ctx, msg.ID, status, &quarantine.Review{ReviewerName: "Admin"}); err != nil {
			t.Fatalf("UpdateQuarantineStatus: %v", err)
		}
	}
	return msg.ID
}
