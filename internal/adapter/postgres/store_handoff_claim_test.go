package postgres_test

import (
	"testing"

	"github.com/google/uuid"
)

// TestStore_HandoffClaims (S2-G fix, 3): handoff.request and
// handoff.approved are delivered at least once; a handoff's stage is
// claimed once per tenant before its run starts, so a redelivery does
// nothing, and a released claim (a transient failure) can be claimed again.
func TestStore_HandoffClaims(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	id := uuid.New().String()
	claim := func(f *statusFixture, stage string) bool {
		t.Helper()
		claimed, err := f.store.ClaimHandoff(f.ctx, id, stage)
		if err != nil {
			t.Fatalf("ClaimHandoff(%s): %v", stage, err)
		}
		return claimed
	}

	if !claim(a, "request") {
		t.Fatal("the first claim was refused")
	}
	if claim(a, "request") {
		t.Fatal("a second claim of the same stage succeeded")
	}
	if !claim(a, "approved") {
		t.Fatal("the approval stage was taken for the request's")
	}
	if !claim(b, "request") {
		t.Fatal("another tenant's handoff with the same ID was refused")
	}
	if err := a.store.ReleaseHandoff(a.ctx, id, "request"); err != nil {
		t.Fatalf("ReleaseHandoff: %v", err)
	}
	if !claim(a, "request") {
		t.Fatal("a released claim could not be claimed again")
	}
	if _, err := a.store.ClaimHandoff(a.ctx, id, "elsewhere"); err == nil {
		t.Fatal("an unknown stage was claimed")
	}
}
