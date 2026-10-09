package postgres_test

import (
	"testing"

	"github.com/google/uuid"

	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// TestStore_A2ATask_CallerKey (S2-G fix, V1): an inbound task records the
// A2A key that created it, and the list can be narrowed to one key's
// inbound tasks: two keys of one tenant must not see each other's tasks.
func TestStore_A2ATask_CallerKey(t *testing.T) {
	store := setupStore(t)
	ctx := ctxWithTenant(t, createTestTenant(t, store))

	create := func(direction a2adomain.Direction, caller string) *a2adomain.A2ATask {
		t.Helper()
		task := a2adomain.NewA2ATask(uuid.New().String())
		task.Direction = direction
		task.CallerKeyID = caller
		if err := store.CreateA2ATask(ctx, task); err != nil {
			t.Fatalf("CreateA2ATask: %v", err)
		}
		return task
	}
	ofA := create(a2adomain.DirectionInbound, "key-a")
	ofB := create(a2adomain.DirectionInbound, "key-b")
	outbound := create(a2adomain.DirectionOutbound, "")

	got, err := store.GetA2ATask(ctx, ofA.ID)
	if err != nil || got.CallerKeyID != "key-a" {
		t.Fatalf("GetA2ATask = %+v, %v; want caller key-a", got, err)
	}

	tasks, _, err := store.ListA2ATasks(ctx, &database.A2ATaskFilter{Direction: string(a2adomain.DirectionInbound), CallerKeyID: "key-a"})
	if err != nil {
		t.Fatalf("ListA2ATasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != ofA.ID {
		t.Fatalf("key-a's inbound tasks = %+v, want only %s (not %s or outbound %s)", tasks, ofA.ID, ofB.ID, outbound.ID)
	}

	// Without a caller filter (the admin API) every task of the tenant is listed.
	all, _, err := store.ListA2ATasks(ctx, &database.A2ATaskFilter{})
	if err != nil || len(all) != 3 {
		t.Fatalf("ListA2ATasks(all) = %d tasks, %v; want 3", len(all), err)
	}
}
