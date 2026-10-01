package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
)

// TestStore_UpdateQuarantineStatus_ResolvesOnce (S2-G fix, 9): an admin's
// approval and an A2A caller's withdrawal of the same held message raced;
// the later one overwrote the earlier and an approved message could be
// marked withdrawn after it was replayed. Only a pending message is
// resolved: the first resolution wins.
func TestStore_UpdateQuarantineStatus_ResolvesOnce(t *testing.T) {
	f := newStatusFixture(t)
	other := newStatusFixture(t)
	now := time.Now().UTC()
	msg := &quarantine.Message{
		Subject: "a2a.task.created", Payload: []byte(`{"task_id":"a2a-1"}`), Status: quarantine.StatusPending, RiskFactors: []string{"test"},
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if err := f.store.QuarantineMessage(f.ctx, msg); err != nil {
		t.Fatalf("QuarantineMessage: %v", err)
	}

	if err := other.store.UpdateQuarantineStatus(other.ctx, msg.ID, quarantine.StatusApproved, &quarantine.Review{ReviewerName: "admin"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("UpdateQuarantineStatus(other tenant) = %v, want ErrNotFound", err)
	}
	if err := f.store.UpdateQuarantineStatus(f.ctx, msg.ID, quarantine.StatusRejected, &quarantine.Review{ReviewerName: "sender", Note: "cancelled"}); err != nil {
		t.Fatalf("UpdateQuarantineStatus: %v", err)
	}
	if err := f.store.UpdateQuarantineStatus(f.ctx, msg.ID, quarantine.StatusApproved, &quarantine.Review{ReviewerName: "admin"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("UpdateQuarantineStatus(resolved) = %v, want ErrConflict", err)
	}
	got, err := f.store.GetQuarantinedMessage(f.ctx, msg.ID)
	if err != nil {
		t.Fatalf("GetQuarantinedMessage: %v", err)
	}
	if got.Status != quarantine.StatusRejected || got.ReviewedBy != "sender" {
		t.Fatalf("message = %s by %q, want rejected by the sender", got.Status, got.ReviewedBy)
	}
	if err := f.store.UpdateQuarantineStatus(f.ctx, uuid.New().String(), quarantine.StatusApproved, &quarantine.Review{ReviewerName: "admin"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("UpdateQuarantineStatus(unknown) = %v, want ErrNotFound", err)
	}
}
