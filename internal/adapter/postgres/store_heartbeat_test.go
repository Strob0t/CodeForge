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
	for _, name := range []string{"ListRunsWithStaleHeartbeat", "ListConversationTurnsWithStaleHeartbeat", "ListTasksWithStaleHeartbeat", "ListTasksNeverAccepted"} {
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
	// The conversation carries its stored active turn.
	if c, err := a.store.GetConversation(a.ctx, lost.ID); err != nil || c.ActiveTurnID != lostTurn {
		t.Fatalf("GetConversation active turn = %+v, %v; want %q", c, err, lostTurn)
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
	if endedIt, err := a.store.EndConversationTurn(a.ctx, ended.ID, turn()); err != nil || endedIt {
		t.Fatalf("EndConversationTurn(other turn) = %v, %v; want false, nil", endedIt, err)
	}
	// Another tenant cannot end a's turn.
	if endedIt, err := b.store.EndConversationTurn(b.ctx, lost.ID, lostTurn); err != nil || endedIt {
		t.Fatalf("EndConversationTurn(cross-tenant) = %v, %v; want false, nil", endedIt, err)
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

	if endedIt, err := a.store.EndConversationTurn(a.ctx, ended.ID, endedTurn); err != nil || !endedIt {
		t.Fatalf("EndConversationTurn = %v, %v; want true, nil", endedIt, err)
	}
	// A turn ends once: its second end reports that it had ended already.
	if endedIt, err := a.store.EndConversationTurn(a.ctx, ended.ID, endedTurn); err != nil || endedIt {
		t.Fatalf("EndConversationTurn(again) = %v, %v; want false, nil", endedIt, err)
	}
	// A stop ends whatever turn is active.
	if endedIt, err := b.store.EndConversationTurn(b.ctx, foreign.ID, ""); err != nil || !endedIt {
		t.Fatalf("EndConversationTurn(any) = %v, %v; want true, nil", endedIt, err)
	}
	got = list()
	for _, conv := range []string{ended.ID, foreign.ID} {
		if _, listed := got[conv]; listed {
			t.Errorf("ended conversation turn %s still listed", conv)
		}
	}
	if c, err := a.store.GetConversation(a.ctx, ended.ID); err != nil || c.ActiveTurnID != "" {
		t.Errorf("GetConversation active turn after its end = %+v, %v; want none", c, err)
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

// TestStore_TaskHeartbeat: a task heartbeat names the dispatch it was sent
// for and counts only for the task's current dispatch (S2-F review, F7). A
// late heartbeat of an earlier dispatch used to be recorded for the task's
// current version: it counted for a re-dispatch no worker had accepted, and
// the watchdog failed that dispatch.
func TestStore_TaskHeartbeat(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)
	dispatched := func(f *statusFixture) (*task.Task, string) {
		t.Helper()
		tk, err := f.store.CreateTask(f.ctx, task.CreateRequest{ProjectID: f.project.ID, Title: "heartbeat", Prompt: "p"})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		dispatch := uuid.New().String()
		if err := f.store.QueueTask(f.ctx, tk.ID, f.agent.ID, dispatch); err != nil {
			t.Fatalf("QueueTask: %v", err)
		}
		return tk, dispatch
	}
	beat := func(f *statusFixture, id, dispatch string) {
		t.Helper()
		if err := f.store.TouchTaskHeartbeat(f.ctx, id, dispatch); err != nil {
			t.Fatalf("TouchTaskHeartbeat: %v", err)
		}
	}
	ageBeat := func(id string) {
		t.Helper()
		if _, err := pool.Exec(context.Background(), `UPDATE task_heartbeats SET beat_at = NOW() - interval '`+ancientBeat+`' WHERE task_id = $1`, id); err != nil {
			t.Fatalf("age task heartbeat: %v", err)
		}
	}

	lost, lostDispatch := dispatched(a)
	lostRunning, lostRunningDispatch := dispatched(a)
	healthy, healthyDispatch := dispatched(a)
	queued, queuedDispatch := dispatched(a) // never accepted
	done, doneDispatch := dispatched(a)
	foreign, foreignDispatch := dispatched(b)
	// A backend task running in its dispatch. UpdateTaskStatus(running) is
	// a run taking the task, which ends its dispatch (S2-G fix, 1), so the
	// status is set directly.
	if _, err := pool.Exec(context.Background(), `UPDATE tasks SET status = 'running' WHERE id = $1`, lostRunning.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}

	beat(a, lost.ID, lostDispatch)
	beat(a, lostRunning.ID, lostRunningDispatch)
	beat(a, healthy.ID, healthyDispatch)
	beat(a, done.ID, doneDispatch)
	beat(b, foreign.ID, foreignDispatch)
	// Another tenant's heartbeat for a task is not recorded, nor one that
	// names another dispatch.
	beat(b, queued.ID, queuedDispatch)
	beat(a, queued.ID, uuid.New().String())
	if err := a.store.UpdateTaskStatus(a.ctx, done.ID, task.StatusCompleted); err != nil {
		t.Fatalf("UpdateTaskStatus(done): %v", err)
	}
	for _, id := range []string{lost.ID, lostRunning.ID, done.ID, foreign.ID, queued.ID} {
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
	for want, dispatch := range map[*task.Task]string{lost: lostDispatch, lostRunning: lostRunningDispatch, foreign: foreignDispatch} {
		tk, ok := got[want.ID]
		if !ok {
			t.Fatalf("task %s with a lost heartbeat not listed", want.ID)
		}
		if tk.TenantID == "" || tk.ProjectID == "" || tk.DispatchID != dispatch {
			t.Fatalf("listed task = %+v, want its tenant, project and dispatch %s", tk, dispatch)
		}
	}
	for name, tk := range map[string]*task.Task{"healthy": healthy, "never accepted": queued, "ended": done} {
		if _, listed := got[tk.ID]; listed {
			t.Errorf("%s task %s listed", name, tk.ID)
		}
	}

	// A re-dispatch starts without heartbeat: neither the old dispatch's
	// heartbeat nor a late one of it counts for the new dispatch.
	if err := a.store.UpdateTaskStatus(a.ctx, lost.ID, task.StatusFailed); err != nil {
		t.Fatalf("UpdateTaskStatus(failed): %v", err)
	}
	redispatch := uuid.New().String()
	if err := a.store.QueueTask(a.ctx, lost.ID, a.agent.ID, redispatch); err != nil {
		t.Fatalf("QueueTask(re-dispatch): %v", err)
	}
	if _, listed := list()[lost.ID]; listed {
		t.Error("re-dispatched task listed with the previous dispatch's heartbeat")
	}
	beat(a, lost.ID, lostDispatch)
	ageBeat(lost.ID)
	if _, listed := list()[lost.ID]; listed {
		t.Error("a late heartbeat of the previous dispatch counted for the re-dispatch")
	}
	// The new dispatch's own heartbeat counts.
	beat(a, lost.ID, redispatch)
	ageBeat(lost.ID)
	if tk, listed := list()[lost.ID]; !listed || tk.DispatchID != redispatch {
		t.Errorf("re-dispatch with a lost heartbeat: listed %v (%+v), want it with dispatch %s", listed, tk, redispatch)
	}
}
