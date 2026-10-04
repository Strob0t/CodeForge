package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-65: runs are acked on accept (ADR-016), so a run whose worker died
// waits forever for its completion - the in-memory timeout covers only
// policies with TimeoutSeconds and dies with a Go Core restart. The worker
// sends a heartbeat every 30 s while it executes a run; the Go Core records
// it in the store, and the stuck-work watchdog stops the running runs whose
// heartbeats stopped. A run without heartbeat waits in NATS for a free worker
// and is left alone, and so is every run that still sends heartbeats.

const lostWorkerTenant = "cccccccc-0000-0000-0000-000000000003"

func newLostWorkerEnv(heartbeatTimeout time.Duration) (*service.RuntimeService, *runtimeMockStore, *runtimeMockQueue, *runtimeMockBroadcaster) {
	_, store, queue, _ := newRuntimeTestEnv()
	bc := &runtimeMockBroadcaster{}
	svc := service.NewRuntimeService(store, queue, bc, &runtimeMockEventStore{},
		service.NewPolicyService("headless-safe-sandbox", []policy.PolicyProfile{gateNoRollback}), &config.Runtime{
			StallThreshold:    5,
			HeartbeatInterval: 30 * time.Second,
			HeartbeatTimeout:  heartbeatTimeout,
		})
	return svc, store, queue, bc
}

// setRunHeartbeat stores the time of a run's last heartbeat.
func setRunHeartbeat(store *runtimeMockStore, id string, at time.Time) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.runBeats == nil {
		store.runBeats = map[string]runBeat{}
	}
	store.runBeats[id] = runBeat{at: at}
}

func TestHandleHeartbeat_RecordsTheRunsHeartbeat(t *testing.T) {
	svc, store, _, _ := newLostWorkerEnv(2 * time.Minute)
	setStoredRun(store, &run.Run{ID: "run-1", TenantID: lostWorkerTenant, TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1", Status: run.StatusRunning})

	if err := svc.HandleHeartbeat(context.Background(), &messagequeue.RunHeartbeatPayload{RunID: "run-1", TenantID: lostWorkerTenant}); err != nil {
		t.Fatalf("HandleHeartbeat: %v", err)
	}

	store.mu.Lock()
	beat, ok := store.runBeats["run-1"]
	store.mu.Unlock()
	if !ok || time.Since(beat.at) > time.Minute {
		t.Fatalf("run heartbeat not recorded: %+v %v", beat, ok)
	}
	if beat.tenant != lostWorkerTenant {
		t.Fatalf("heartbeat recorded in tenant %q, want the payload's %q", beat.tenant, lostWorkerTenant)
	}
	if _, ok := svc.LastHeartbeat("run-1"); !ok {
		t.Fatal("run heartbeat not kept for the tool-call termination check")
	}
}

// TestHandleHeartbeat_ConversationTurn: a conversation run's heartbeat names
// its turn and is recorded for the conversation's active turn only; it is not
// kept in memory, where conversation IDs were never removed (KI-67).
func TestHandleHeartbeat_ConversationTurn(t *testing.T) {
	svc, store, _, _ := newLostWorkerEnv(2 * time.Minute)

	if err := svc.HandleHeartbeat(context.Background(), &messagequeue.RunHeartbeatPayload{RunID: "conv-1", TurnID: "turn-1", TenantID: lostWorkerTenant}); err != nil {
		t.Fatalf("HandleHeartbeat: %v", err)
	}

	store.mu.Lock()
	beats := append([]turnBeat(nil), store.turnBeats...)
	_, runBeat := store.runBeats["conv-1"]
	store.mu.Unlock()
	if len(beats) != 1 || beats[0] != (turnBeat{conversation: "conv-1", turn: "turn-1", tenant: lostWorkerTenant}) {
		t.Fatalf("turn heartbeats = %+v, want conv-1/turn-1 in the payload's tenant", beats)
	}
	if runBeat {
		t.Fatal("a conversation heartbeat was recorded as a run heartbeat")
	}
	if _, ok := svc.LastHeartbeat("conv-1"); ok {
		t.Fatal("a conversation heartbeat was kept in memory")
	}
}

func TestEndRunsWithLostWorker(t *testing.T) {
	svc, store, queue, bc := newLostWorkerEnv(2 * time.Minute) // lost after 2m + 2 x 30s
	long := time.Now().Add(-time.Hour)
	for _, r := range []struct {
		id     string
		status run.Status
		beat   time.Time // zero: no heartbeat yet
	}{
		{"run-lost", run.StatusRunning, long},
		{"run-healthy", run.StatusRunning, time.Now()},
		{"run-slow-beat", run.StatusRunning, time.Now().Add(-2 * time.Minute)}, // within timeout + margin
		{"run-queued", run.StatusRunning, time.Time{}},
		{"run-gate", run.StatusQualityGate, long},
		{"run-done", run.StatusCompleted, long},
	} {
		setStoredRun(store, &run.Run{
			ID: r.id, TenantID: lostWorkerTenant, TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
			PolicyProfile: "headless-safe-sandbox", Status: r.status, DeliverMode: run.DeliverModeNone,
		})
		if !r.beat.IsZero() {
			setRunHeartbeat(store, r.id, r.beat)
		}
	}

	ended, err := svc.EndRunsWithLostWorker(context.Background())
	if err != nil {
		t.Fatalf("EndRunsWithLostWorker: %v", err)
	}
	if ended != 1 {
		t.Fatalf("ended %d runs, want 1", ended)
	}

	lost := storedRun(t, store, "run-lost")
	if lost.Status != run.StatusTimeout || !strings.Contains(lost.Error, "heartbeat") {
		t.Fatalf("lost run = %s %q, want timeout for a lost heartbeat", lost.Status, lost.Error)
	}
	msg, ok := queue.lastMessage(messagequeue.SubjectRunCancel)
	if !ok {
		t.Fatal("the worker was not told to stop the run")
	}
	var cancel struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(msg.Data, &cancel); err != nil || cancel.RunID != "run-lost" {
		t.Fatalf("runs.cancel = %s, want run-lost", msg.Data)
	}
	for id, want := range map[string]run.Status{
		"run-healthy": run.StatusRunning, "run-slow-beat": run.StatusRunning, "run-queued": run.StatusRunning,
		"run-gate": run.StatusQualityGate, "run-done": run.StatusCompleted,
	} {
		if got := storedRun(t, store, id).Status; got != want {
			t.Errorf("%s status = %s, want %s", id, got, want)
		}
	}
	events := bc.snapshot()
	if len(events) == 0 {
		t.Fatal("ending the lost run broadcast nothing")
	}
	for _, ev := range events {
		if ev.Tenant != lostWorkerTenant {
			t.Errorf("%s broadcast in tenant %q, want the run's %q", ev.EventType, ev.Tenant, lostWorkerTenant)
		}
	}
}

func TestEndRunsWithLostWorker_DisabledWithoutHeartbeatTimeout(t *testing.T) {
	svc, store, _, _ := newLostWorkerEnv(0)
	setStoredRun(store, &run.Run{ID: "run-lost", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1", Status: run.StatusRunning})
	setRunHeartbeat(store, "run-lost", time.Now().Add(-24*time.Hour))

	ended, err := svc.EndRunsWithLostWorker(tenantctx.WithTenant(context.Background(), lostWorkerTenant))
	if err != nil || ended != 0 {
		t.Fatalf("EndRunsWithLostWorker = %d, %v; want 0, nil", ended, err)
	}
	if got := storedRun(t, store, "run-lost").Status; got != run.StatusRunning {
		t.Fatalf("run status = %s, want running", got)
	}
}
