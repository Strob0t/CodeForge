package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-76: a run whose start was dead-lettered (the worker rejected it, or
// could not accept it within its deliveries) is executed by no worker and
// sends no heartbeat, so neither its completion nor the stuck-work watchdog
// ended it: a conversation refused every message with 409 until a stop or a
// Go Core restart, and a run stayed running. The Go Core reads the
// dead-lettered starts and ends these runs as failed.

func TestConversationRun_DeadLetteredStartEndsTheRun(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	ctx := context.Background()
	start := conversationRunStarters[0].start
	if err := start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("start: %v", err)
	}
	msg, ok := env.starts.lastMessage(messagequeue.SubjectConversationRunStart)
	if !ok {
		t.Fatal("no run start published")
	}

	if err := env.conv.HandleDeadLetteredRunStart(ctx, messagequeue.SubjectConversationRunStart+".dlq", msg.Data); err != nil {
		t.Fatalf("HandleDeadLetteredRunStart: %v", err)
	}

	var finished *event.AGUIRunFinishedEvent
	for _, ev := range env.hub.snapshot() {
		if f, ok := ev.Data.(event.AGUIRunFinishedEvent); ok && f.RunID == env.convID {
			finished = &f
		}
	}
	if finished == nil || finished.Status != "failed" || !strings.Contains(finished.Error, "dead-lettered") {
		t.Fatalf("run finished event = %+v, want failed for a dead-lettered start", finished)
	}
	if err := start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("next message after the dead-lettered start: %v", err)
	}
}

// TestConversationRun_DeadLetteredStartOfAnEndedRunIsIgnored: the start of a
// run that is no longer the conversation's active run (it was stopped, a
// newer run runs) ends nothing.
func TestConversationRun_DeadLetteredStartOfAnEndedRunIsIgnored(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	ctx := context.Background()
	start := conversationRunStarters[0].start
	if err := start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("first run: %v", err)
	}
	old, _ := env.starts.lastMessage(messagequeue.SubjectConversationRunStart)
	if err := env.conv.StopConversation(ctx, env.convID); err != nil {
		t.Fatalf("StopConversation: %v", err)
	}
	if err := start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("next run: %v", err)
	}
	events := len(env.hub.snapshot())

	for _, data := range [][]byte{old.Data, []byte("not json"), []byte(`{"conversation_id":""}`)} {
		if err := env.conv.HandleDeadLetteredRunStart(ctx, messagequeue.SubjectConversationRunStart+".dlq", data); err != nil {
			t.Fatalf("HandleDeadLetteredRunStart(%s): %v", data, err)
		}
	}

	if got := len(env.hub.snapshot()); got != events {
		t.Errorf("events = %d, want %d: nothing ended", got, events)
	}
	if err := start(ctx, env.conv, env.convID); !errors.Is(err, service.ErrConversationRunInProgress) {
		t.Errorf("message while the newer run runs = %v, want ErrConversationRunInProgress", err)
	}
}

func TestRun_DeadLetteredStartFailsTheRun(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	const runID = "11111111-2222-3333-4444-555555555555"
	setStoredRun(store, &run.Run{
		ID: runID, TenantID: lostWorkerTenant, TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning,
	})
	data, err := json.Marshal(messagequeue.RunStartPayload{RunID: runID, TenantID: lostWorkerTenant, TaskID: "task-1", ProjectID: "proj-1"})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.HandleDeadLetteredRunStart(context.Background(), data); err != nil {
		t.Fatalf("HandleDeadLetteredRunStart: %v", err)
	}

	r := storedRun(t, store, runID)
	if r.Status != run.StatusFailed || !strings.Contains(r.Error, "dead-lettered") {
		t.Fatalf("run = %s %q, want failed for a dead-lettered start", r.Status, r.Error)
	}

	// Unreadable starts, unknown runs and runs that ended are ignored.
	for _, bad := range [][]byte{[]byte("not json"), []byte(`{"run_id":"handoff-run-1-agent-2"}`), []byte(`{"run_id":"99999999-2222-3333-4444-555555555555"}`), data} {
		if err := svc.HandleDeadLetteredRunStart(context.Background(), bad); err != nil {
			t.Errorf("HandleDeadLetteredRunStart(%s): %v", bad, err)
		}
	}
	if r := storedRun(t, store, runID); r.Status != run.StatusFailed {
		t.Errorf("ended run changed to %s", r.Status)
	}
}
