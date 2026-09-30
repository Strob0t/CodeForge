package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// Worker heartbeats feed the stuck-work watchdog (KI-65): work acked on
// accept whose worker stopped sending heartbeats is ended. The list queries
// span all tenants; work without any heartbeat (not accepted yet) is never
// listed. Test heartbeats are dated a century back and the queries look for
// heartbeats older than 99 years, so rows of other tests are never listed.

const (
	ancientBeat  = "100 years"
	staleForTest = 99 * 365 * 24 * time.Hour
)

func TestHeartbeatQueries_IntentionallyCrossTenant(t *testing.T) {
	src := readStoreSource(t, "store_heartbeat.go")
	for _, name := range []string{"ListRunsWithStaleHeartbeat", "ListConversationTurnsWithStaleHeartbeat", "ListTasksWithStaleHeartbeat"} {
		if doc := methodDocComment(t, src, "store_heartbeat.go", name); !strings.Contains(doc, "INTENTIONALLY CROSS-TENANT") {
			t.Errorf("%s must document why it is intentionally cross-tenant", name)
		}
	}
}

func TestStore_RunHeartbeat(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)

	lost := a.runIn(t, run.StatusRunning)
	healthy := a.runIn(t, run.StatusRunning)
	queued := a.runIn(t, run.StatusRunning) // never accepted: no heartbeat
	ended := a.runIn(t, run.StatusCompleted)
	otherTenant := b.runIn(t, run.StatusRunning)

	for _, r := range []*run.Run{lost, healthy, otherTenant} {
		if err := a.store.TouchRunHeartbeat(ctxOf(a, b, r), r.ID); err != nil {
			t.Fatalf("TouchRunHeartbeat(%s): %v", r.ID, err)
		}
	}
	// A heartbeat of an ended run is not recorded.
	if err := a.store.TouchRunHeartbeat(a.ctx, ended.ID); err != nil {
		t.Fatalf("TouchRunHeartbeat(ended): %v", err)
	}
	// Another tenant's heartbeat for a run is not recorded.
	if err := b.store.TouchRunHeartbeat(b.ctx, queued.ID); err != nil {
		t.Fatalf("TouchRunHeartbeat(foreign): %v", err)
	}
	for _, id := range []string{lost.ID, otherTenant.ID} {
		if _, err := pool.Exec(context.Background(), `UPDATE runs SET last_heartbeat_at = NOW() - interval '`+ancientBeat+`' WHERE id = $1`, id); err != nil {
			t.Fatalf("age heartbeat: %v", err)
		}
	}
	if _, err := pool.Exec(context.Background(), `UPDATE runs SET last_heartbeat_at = NOW() - interval '`+ancientBeat+`' WHERE id = $1 AND last_heartbeat_at IS NOT NULL`, ended.ID); err != nil {
		t.Fatalf("age ended heartbeat: %v", err)
	}

	stale, err := a.store.ListRunsWithStaleHeartbeat(context.Background(), staleForTest, 1000)
	if err != nil {
		t.Fatalf("ListRunsWithStaleHeartbeat: %v", err)
	}
	found := map[string]run.Run{}
	for i := range stale {
		found[stale[i].ID] = stale[i]
	}
	for _, want := range []*run.Run{lost, otherTenant} {
		got, ok := found[want.ID]
		if !ok {
			t.Fatalf("run %s with a lost heartbeat not listed", want.ID)
		}
		if got.TenantID == "" || got.Status != run.StatusRunning {
			t.Fatalf("listed run = %+v, want its tenant and status", got)
		}
	}
	for name, r := range map[string]*run.Run{"healthy": healthy, "never accepted": queued, "ended": ended} {
		if _, listed := found[r.ID]; listed {
			t.Errorf("%s run %s listed", name, r.ID)
		}
	}

	limited, err := a.store.ListRunsWithStaleHeartbeat(context.Background(), staleForTest, 1)
	if err != nil {
		t.Fatalf("ListRunsWithStaleHeartbeat with limit: %v", err)
	}
	if len(limited) > 1 {
		t.Fatalf("limit 1 returned %d runs", len(limited))
	}
}

// ctxOf returns the tenant context of the fixture that owns r.
func ctxOf(a, b *statusFixture, r *run.Run) context.Context {
	if r.ProjectID == b.project.ID {
		return b.ctx
	}
	return a.ctx
}

func TestStore_ConversationTurnHeartbeat(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)
	ageTurn := func(id string) {
		t.Helper()
		if _, err := pool.Exec(context.Background(), `UPDATE conversations SET active_turn_heartbeat_at = NOW() - interval '`+ancientBeat+`' WHERE id = $1 AND active_turn_heartbeat_at IS NOT NULL`, id); err != nil {
			t.Fatalf("age turn heartbeat: %v", err)
		}
	}

	lost, healthy, queued, ended := a.conversation(t), a.conversation(t), a.conversation(t), a.conversation(t)
	foreign := b.conversation(t)
	turn := func() string { return uuid.New().String() }
	lostTurn, healthyTurn, queuedTurn, endedTurn, foreignTurn := turn(), turn(), turn(), turn(), turn()

	for conv, tr := range map[string]string{lost.ID: lostTurn, healthy.ID: healthyTurn, queued.ID: queuedTurn, ended.ID: endedTurn} {
		if err := a.store.BeginConversationTurn(a.ctx, conv, tr); err != nil {
			t.Fatalf("BeginConversationTurn: %v", err)
		}
	}
	if err := b.store.BeginConversationTurn(b.ctx, foreign.ID, foreignTurn); err != nil {
		t.Fatalf("BeginConversationTurn(foreign): %v", err)
	}
	// Another tenant cannot begin a turn on a's conversation.
	if err := b.store.BeginConversationTurn(b.ctx, lost.ID, turn()); err != nil {
		t.Fatalf("BeginConversationTurn(cross-tenant): %v", err)
	}

	for conv, tr := range map[string]string{lost.ID: lostTurn, healthy.ID: healthyTurn, ended.ID: endedTurn} {
		if err := a.store.TouchConversationTurnHeartbeat(a.ctx, conv, tr); err != nil {
			t.Fatalf("TouchConversationTurnHeartbeat: %v", err)
		}
	}
	if err := b.store.TouchConversationTurnHeartbeat(b.ctx, foreign.ID, foreignTurn); err != nil {
		t.Fatalf("TouchConversationTurnHeartbeat(foreign): %v", err)
	}
	// A heartbeat of another turn (a stopped run) does not count for the active one.
	if err := a.store.TouchConversationTurnHeartbeat(a.ctx, queued.ID, turn()); err != nil {
		t.Fatalf("TouchConversationTurnHeartbeat(other turn): %v", err)
	}
	// Ending another turn keeps the active one; ending the active one clears it.
	if err := a.store.EndConversationTurn(a.ctx, ended.ID, turn()); err != nil {
		t.Fatalf("EndConversationTurn(other turn): %v", err)
	}
	for _, id := range []string{lost.ID, ended.ID, foreign.ID} {
		ageTurn(id)
	}

	list := func() map[string]string {
		t.Helper()
		turns, err := a.store.ListConversationTurnsWithStaleHeartbeat(context.Background(), staleForTest, 1000)
		if err != nil {
			t.Fatalf("ListConversationTurnsWithStaleHeartbeat: %v", err)
		}
		got := map[string]string{}
		for _, at := range turns {
			if at.TenantID == "" {
				t.Fatalf("listed turn without tenant: %+v", at)
			}
			got[at.ConversationID] = at.TurnID
		}
		return got
	}

	got := list()
	for conv, tr := range map[string]string{lost.ID: lostTurn, ended.ID: endedTurn, foreign.ID: foreignTurn} {
		if got[conv] != tr {
			t.Errorf("conversation %s: listed turn %q, want %q", conv, got[conv], tr)
		}
	}
	for name, conv := range map[string]string{"healthy": healthy.ID, "never accepted": queued.ID} {
		if _, listed := got[conv]; listed {
			t.Errorf("%s conversation %s listed", name, conv)
		}
	}

	if err := a.store.EndConversationTurn(a.ctx, ended.ID, endedTurn); err != nil {
		t.Fatalf("EndConversationTurn: %v", err)
	}
	// A stop ends whatever turn is active.
	if err := b.store.EndConversationTurn(b.ctx, foreign.ID, ""); err != nil {
		t.Fatalf("EndConversationTurn(any): %v", err)
	}
	got = list()
	for _, conv := range []string{ended.ID, foreign.ID} {
		if _, listed := got[conv]; listed {
			t.Errorf("ended conversation turn %s still listed", conv)
		}
	}
	if got[lost.ID] != lostTurn {
		t.Errorf("lost turn no longer listed after other turns ended")
	}

	// A new turn starts without heartbeat, so it is not taken for lost.
	if err := a.store.BeginConversationTurn(a.ctx, lost.ID, turn()); err != nil {
		t.Fatalf("BeginConversationTurn(next): %v", err)
	}
	if _, listed := list()[lost.ID]; listed {
		t.Error("next turn listed before any heartbeat")
	}
}

func TestStore_TaskHeartbeat(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)
	newTask := func(f *statusFixture, status task.Status) *task.Task {
		t.Helper()
		tk, err := f.store.CreateTask(f.ctx, task.CreateRequest{ProjectID: f.project.ID, Title: "heartbeat", Prompt: "p"})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if status != task.StatusPending {
			if err := f.store.UpdateTaskStatus(f.ctx, tk.ID, status); err != nil {
				t.Fatalf("UpdateTaskStatus: %v", err)
			}
		}
		return tk
	}
	ageBeat := func(id string) {
		t.Helper()
		if _, err := pool.Exec(context.Background(), `UPDATE task_heartbeats SET beat_at = NOW() - interval '`+ancientBeat+`' WHERE task_id = $1`, id); err != nil {
			t.Fatalf("age task heartbeat: %v", err)
		}
	}

	lost := newTask(a, task.StatusRunning)
	lostQueued := newTask(a, task.StatusQueued)
	healthy := newTask(a, task.StatusRunning)
	queued := newTask(a, task.StatusQueued) // never accepted
	done := newTask(a, task.StatusRunning)
	foreign := newTask(b, task.StatusRunning)

	for _, tk := range []*task.Task{lost, lostQueued, healthy, done} {
		if err := a.store.TouchTaskHeartbeat(a.ctx, tk.ID); err != nil {
			t.Fatalf("TouchTaskHeartbeat: %v", err)
		}
	}
	if err := b.store.TouchTaskHeartbeat(b.ctx, foreign.ID); err != nil {
		t.Fatalf("TouchTaskHeartbeat(foreign): %v", err)
	}
	// Another tenant's heartbeat for a task is not recorded.
	if err := b.store.TouchTaskHeartbeat(b.ctx, queued.ID); err != nil {
		t.Fatalf("TouchTaskHeartbeat(cross-tenant): %v", err)
	}
	if err := a.store.UpdateTaskStatus(a.ctx, done.ID, task.StatusCompleted); err != nil {
		t.Fatalf("UpdateTaskStatus(done): %v", err)
	}
	for _, id := range []string{lost.ID, lostQueued.ID, done.ID, foreign.ID, queued.ID} {
		ageBeat(id)
	}

	list := func() map[string]task.Task {
		t.Helper()
		tasks, err := a.store.ListTasksWithStaleHeartbeat(context.Background(), staleForTest, 1000)
		if err != nil {
			t.Fatalf("ListTasksWithStaleHeartbeat: %v", err)
		}
		got := map[string]task.Task{}
		for i := range tasks {
			got[tasks[i].ID] = tasks[i]
		}
		return got
	}
	got := list()
	for _, want := range []*task.Task{lost, lostQueued, foreign} {
		tk, ok := got[want.ID]
		if !ok {
			t.Fatalf("task %s with a lost heartbeat not listed", want.ID)
		}
		if tk.TenantID == "" || tk.ProjectID == "" {
			t.Fatalf("listed task = %+v, want its tenant and project", tk)
		}
	}
	for name, tk := range map[string]*task.Task{"healthy": healthy, "never accepted": queued, "ended": done} {
		if _, listed := got[tk.ID]; listed {
			t.Errorf("%s task %s listed", name, tk.ID)
		}
	}

	// A re-dispatch (the task row changes after the last heartbeat) starts
	// without heartbeat: the old dispatch's heartbeat does not count.
	if err := a.store.UpdateTaskStatus(a.ctx, lost.ID, task.StatusQueued); err != nil {
		t.Fatalf("UpdateTaskStatus(re-dispatch): %v", err)
	}
	if _, listed := list()[lost.ID]; listed {
		t.Error("re-dispatched task listed with the previous dispatch's heartbeat")
	}
}
