package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// A conversation turn is completed once (S2-F review, F3). The stuck-work
// watchdog ends a turn whose worker went silent with a synthetic failed
// completion; the worker's own completion of that turn, arriving later, was
// processed again: it stored the old turn's messages after the next turn's,
// announced the next turn as finished and fed its waiter. A completion is
// processed only while its turn is the conversation's active turn.

// completeTurn reports the worker's completion of the conversation's run
// turn with an assistant answer.
func (e *convStopEnv) completeTurn(t *testing.T, turn, answer string) {
	t.Helper()
	data, err := json.Marshal(messagequeue.ConversationRunCompletePayload{
		RunID: e.convID, ConversationID: e.convID, Status: "completed", AssistantContent: answer, TurnID: turn,
	})
	if err != nil {
		t.Fatalf("marshal completion: %v", err)
	}
	if err := e.conv.HandleConversationRunComplete(context.Background(), messagequeue.SubjectConversationRunComplete, data); err != nil {
		t.Fatalf("HandleConversationRunComplete: %v", err)
	}
}

// hasMessage reports whether the conversation stores a message with content.
func (e *convStopEnv) hasMessage(t *testing.T, content string) bool {
	t.Helper()
	msgs, err := e.conv.ListMessages(context.Background(), e.convID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	for i := range msgs {
		if msgs[i].Content == content {
			return true
		}
	}
	return false
}

// finishedEvents counts the run finished broadcasts of the conversation.
func (e *convStopEnv) finishedEvents() int {
	n := 0
	for _, ev := range e.hub.snapshot() {
		if f, ok := ev.Data.(event.AGUIRunFinishedEvent); ok && ev.EventType == event.AGUIRunFinished && f.RunID == e.convID {
			n++
		}
	}
	return n
}

// endTurnByWatchdog lets the stuck-work watchdog end the conversation's
// active turn as a run whose worker went silent.
func (e *convStopEnv) endTurnByWatchdog(t *testing.T, turn string) {
	t.Helper()
	e.conv.SetRuntimeConfig(&config.Runtime{HeartbeatTimeout: 2 * time.Minute, HeartbeatInterval: 30 * time.Second})
	e.heartbeatOfTurn(t, e.convID, turn)
	e.store.setTurnHeartbeat(e.convID, time.Now().Add(-time.Hour))
	if ended, err := e.conv.EndConversationRunsWithLostWorker(context.Background()); err != nil || ended != 1 {
		t.Fatalf("EndConversationRunsWithLostWorker = %d, %v; want 1, nil", ended, err)
	}
}

func TestConversationRun_CompletionOfAnEndedTurnIsDropped(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	ctx := context.Background()
	start := conversationRunStarters[0].start

	if err := start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first := env.lastTurn(t)
	env.endTurnByWatchdog(t, first)

	if err := start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("next run: %v", err)
	}
	next := env.lastTurn(t)
	waiter, err := env.conv.ExpectCompletion(env.convID)
	if err != nil {
		t.Fatalf("ExpectCompletion: %v", err)
	}
	defer waiter.Close()
	finished := env.finishedEvents()

	// The worker of the ended turn reconnects and reports its end.
	env.completeTurn(t, first, "answer of the ended turn")

	if env.hasMessage(t, "answer of the ended turn") {
		t.Error("the ended turn's answer was stored")
	}
	if got := env.finishedEvents(); got != finished {
		t.Errorf("run finished broadcasts = %d, want %d: the next run was announced as finished", got, finished)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if result, err := waiter.Wait(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the next run's waiter got %+v, %v; want no result", result, err)
	}
	if !env.runtime.IsActiveConversationRun(env.convID, next) || env.store.activeTurnOf(env.convID) != next {
		t.Error("the next run is no longer the conversation's active run")
	}

	// The next turn's own completion is processed.
	env.completeTurn(t, next, "answer of the next turn")
	if !env.hasMessage(t, "answer of the next turn") {
		t.Error("the active turn's answer was not stored")
	}
}

// TestConversationRun_CompletionOfTheStoredActiveTurn: a process that did
// not dispatch the turn (a restart) takes the stored active turn for the
// active one.
func TestConversationRun_CompletionOfTheStoredActiveTurn(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	if err := env.store.BeginConversationTurn(context.Background(), env.convID, "turn-before-restart"); err != nil {
		t.Fatalf("BeginConversationTurn: %v", err)
	}

	env.completeTurn(t, "turn-of-another-run", "answer of another run")
	env.completeTurn(t, "turn-before-restart", "answer after the restart")

	if env.hasMessage(t, "answer of another run") {
		t.Error("the answer of a turn that is not active was stored")
	}
	if !env.hasMessage(t, "answer after the restart") {
		t.Error("the stored active turn's answer was not stored")
	}
	if got := env.store.activeTurnOf(env.convID); got != "" {
		t.Errorf("stored active turn = %q, want none", got)
	}
}

// TestConversationRun_ToolCallsOfAnEndedTurnAreDenied (S2-F review, F4): a
// turn the stuck-work watchdog ended left no mark, so the calls of its
// worker, when it reconnected, were evaluated and allowed. A call of a turn
// that is neither the active run here nor the stored active turn is denied.
func TestConversationRun_ToolCallsOfAnEndedTurnAreDenied(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	ctx := context.Background()
	start := conversationRunStarters[0].start

	if err := start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first := env.lastTurn(t)
	if resp := env.toolCallInTurn(t, "call-while-active", first); resp.Decision != "allow" {
		t.Fatalf("call of the active turn: %q (%s), want allow", resp.Decision, resp.Reason)
	}
	env.endTurnByWatchdog(t, first)

	if resp := env.toolCallInTurn(t, "call-after-the-watchdog", first); resp.Decision != "deny" || resp.Reason != "conversation run ended" {
		t.Fatalf("call of the ended turn: %q (%s), want deny (conversation run ended)", resp.Decision, resp.Reason)
	}

	if err := start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("next run: %v", err)
	}
	next := env.lastTurn(t)
	if resp := env.toolCallInTurn(t, "call-during-the-next-turn", first); resp.Decision != "deny" {
		t.Fatalf("call of the ended turn during the next one: %q (%s), want deny", resp.Decision, resp.Reason)
	}
	if resp := env.toolCallInTurn(t, "call-of-the-next-turn", next); resp.Decision != "allow" {
		t.Fatalf("call of the next turn: %q (%s), want allow", resp.Decision, resp.Reason)
	}
}

// TestConversationRun_ToolCallsOfTheStoredActiveTurn: a process that did
// not dispatch the turn (a restart) evaluates the calls of the stored active
// turn and denies those of any other turn.
func TestConversationRun_ToolCallsOfTheStoredActiveTurn(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	if err := env.store.BeginConversationTurn(context.Background(), env.convID, "turn-before-restart"); err != nil {
		t.Fatalf("BeginConversationTurn: %v", err)
	}
	if resp := env.toolCallInTurn(t, "call-of-the-stored-turn", "turn-before-restart"); resp.Decision != "allow" {
		t.Fatalf("call of the stored active turn: %q (%s), want allow", resp.Decision, resp.Reason)
	}
	if resp := env.toolCallInTurn(t, "call-of-another-turn", "turn-of-another-run"); resp.Decision != "deny" {
		t.Fatalf("call of another turn: %q (%s), want deny", resp.Decision, resp.Reason)
	}
	// A worker that sends no turn is evaluated as before.
	if resp := env.toolCall(t, "call-without-turn"); resp.Decision != "allow" {
		t.Fatalf("call without turn: %q (%s), want allow", resp.Decision, resp.Reason)
	}
}

// TestStopConversation_EndsOnlyTheStoppedTurn (S2-F review, F10): a stop
// released the run in memory, then cleared whatever turn was stored as
// active; a run that began in between lost its fresh turn, and the
// watchdog and the turn checks no longer knew it. The stop ends the stored
// turn of the run it stopped only.
func TestStopConversation_EndsOnlyTheStoppedTurn(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	ctx := context.Background()
	start := conversationRunStarters[0].start
	if err := start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("first run: %v", err)
	}

	var next string
	env.store.endTurnHook = func(string, string) {
		env.store.endTurnHook = nil
		// The next message arrives while the stop is under way.
		if err := start(ctx, env.conv, env.convID); err != nil {
			t.Errorf("run started during the stop: %v", err)
			return
		}
		next = env.lastTurn(t)
	}
	if err := env.conv.StopConversation(ctx, env.convID); err != nil {
		t.Fatalf("StopConversation: %v", err)
	}
	if next == "" {
		t.Fatal("no run started during the stop")
	}
	if got := env.store.activeTurnOf(env.convID); got != next {
		t.Fatalf("stored active turn = %q, want the run that began during the stop %q", got, next)
	}
}

// TestStopConversation_EndsTheStoredTurnAfterARestart: a process that did
// not dispatch the run (a restart) ends the stored active turn.
func TestStopConversation_EndsTheStoredTurnAfterARestart(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	ctx := context.Background()
	if err := env.store.BeginConversationTurn(ctx, env.convID, "turn-before-restart"); err != nil {
		t.Fatalf("BeginConversationTurn: %v", err)
	}
	if err := env.conv.StopConversation(ctx, env.convID); err != nil {
		t.Fatalf("StopConversation: %v", err)
	}
	if got := env.store.activeTurnOf(env.convID); got != "" {
		t.Fatalf("stored active turn after the stop = %q, want none", got)
	}
}
