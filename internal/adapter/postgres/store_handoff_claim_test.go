package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestStore_HandoffClaims (S2-G fix 3; S2-G fix 2, 3): handoff.request and
// handoff.approved are delivered at least once; a handoff's stage is claimed
// once per tenant before its run starts, so a redelivery does nothing. A
// claim is done once the stage was carried out; a claim that was never done
// (the process died) can be taken over once its lease ran out; a released
// claim (a transient failure) can be claimed again at once.
func TestStore_HandoffClaims(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)
	const lease = 10 * time.Minute
	id := uuid.New().String()
	claim := func(f *statusFixture, stage string) (claimed, done bool, age time.Duration) {
		t.Helper()
		c, err := f.store.ClaimHandoff(f.ctx, id, stage, lease)
		if err != nil {
			t.Fatalf("ClaimHandoff(%s): %v", stage, err)
		}
		return c.Claimed, c.Done, c.Age
	}

	if claimed, _, _ := claim(a, "request"); !claimed {
		t.Fatal("the first claim was refused")
	}
	if claimed, done, age := claim(a, "request"); claimed || done || age < 0 || age > time.Minute {
		t.Fatalf("second claim = claimed %v, done %v, age %s; want a fresh claim in progress", claimed, done, age)
	}
	if claimed, _, _ := claim(a, "approved"); !claimed {
		t.Fatal("the approval stage was taken for the request's")
	}
	if claimed, _, _ := claim(b, "request"); !claimed {
		t.Fatal("another tenant's handoff with the same ID was refused")
	}

	// The process died after claiming: once the lease ran out, the claim is
	// taken over.
	if _, err := pool.Exec(context.Background(),
		`UPDATE handoff_claims SET claimed_at = now() - interval '11 minutes' WHERE handoff_id = $1 AND stage = 'request'`, id); err != nil {
		t.Fatalf("age claim: %v", err)
	}
	if claimed, _, _ := claim(a, "request"); !claimed {
		t.Fatal("a claim older than its lease was not taken over")
	}

	// Done: never claimed again, however old.
	if err := a.store.FinishHandoff(a.ctx, id, "request"); err != nil {
		t.Fatalf("FinishHandoff: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE handoff_claims SET claimed_at = now() - interval '1 day' WHERE handoff_id = $1 AND stage = 'request'`, id); err != nil {
		t.Fatalf("age claim: %v", err)
	}
	if claimed, done, _ := claim(a, "request"); claimed || !done {
		t.Fatalf("claim of a done stage = claimed %v, done %v; want done", claimed, done)
	}

	// Released: claimed again at once, with the task of the earlier attempt
	// (S2-G fix 2, 1).
	if err := a.store.SetHandoffTask(a.ctx, id, "approved", "task-1"); err != nil {
		t.Fatalf("SetHandoffTask: %v", err)
	}
	if err := a.store.ReleaseHandoff(a.ctx, id, "approved"); err != nil {
		t.Fatalf("ReleaseHandoff: %v", err)
	}
	c, err := a.store.ClaimHandoff(a.ctx, id, "approved", lease)
	if err != nil || !c.Claimed || c.TaskID != "task-1" {
		t.Fatalf("claim after release = %+v, %v; want claimed with task-1", c, err)
	}
	if _, err := a.store.ClaimHandoff(a.ctx, id, "elsewhere", lease); err == nil {
		t.Fatal("an unknown stage was claimed")
	}
}
