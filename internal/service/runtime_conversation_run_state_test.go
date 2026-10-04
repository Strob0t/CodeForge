package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// runStateStore serves one conversation of one tenant.
type runStateStore struct {
	database.Store // only GetConversation is called
	conv           conversation.Conversation
}

func (s *runStateStore) GetConversation(ctx context.Context, id string) (*conversation.Conversation, error) {
	if id != s.conv.ID || tenantctx.FromContext(ctx) != s.conv.TenantID {
		return nil, domain.ErrNotFound
	}
	c := s.conv
	return &c, nil
}

func runOutput(t *testing.T, svc *RuntimeService, tenantID, line, stream string) {
	t.Helper()
	data, err := json.Marshal(messagequeue.RunOutputPayload{TaskID: "conv-1", TenantID: tenantID, Line: line, Stream: stream})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.handleRunOutput(context.Background(), data); err != nil {
		t.Fatalf("handleRunOutput: %v", err)
	}
}

// KI-148: after a reload the chat asks the Core for the conversation's
// running turn - the text streamed so far and every approval still pending,
// with its deadline - so the user can still allow or deny.
func TestConversationRunState_RestoresRunningTurn(t *testing.T) {
	svc, bc := newHITLTestService(30)
	svc.store = &runStateStore{conv: conversation.Conversation{ID: "conv-1", TenantID: "tenant-a"}}
	ctxA := tenantctx.WithTenant(context.Background(), "tenant-a")

	idle, err := svc.ConversationRunState(ctxA, "conv-1")
	if err != nil {
		t.Fatalf("ConversationRunState: %v", err)
	}
	if idle.Active || idle.StreamedText != "" || len(idle.PendingApprovals) != 0 {
		t.Fatalf("idle conversation: %+v", idle)
	}

	if err := svc.BeginConversationRun(ctxA, "conv-1", "turn-1"); err != nil {
		t.Fatal(err)
	}
	svc.ConversationRunDispatched("conv-1", "turn-1")
	runOutput(t, svc, "tenant-a", "Hello ", "stdout")
	runOutput(t, svc, "tenant-a", "ignored", "stderr")
	// Output that names another tenant is not the turn's (KI-148 review).
	runOutput(t, svc, "tenant-b", "foreign", "stdout")
	runOutput(t, svc, "tenant-a", "world", "stdout")

	before := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.waitForApproval(ctxA, &event.AGUIPermissionRequestEvent{RunID: "conv-1", CallID: "call-1", Tool: "bash", Command: "make"})
	}()

	var got *ConversationRunState
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, err = svc.ConversationRunState(ctxA, "conv-1")
		if err != nil {
			t.Fatalf("ConversationRunState: %v", err)
		}
		if len(got.PendingApprovals) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("approval never listed: %+v", got)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !got.Active || got.TurnID != "turn-1" || got.StreamedText != "Hello world" {
		t.Fatalf("running turn: %+v", got)
	}
	pa := got.PendingApprovals[0]
	if pa.CallID != "call-1" || pa.Tool != "bash" || pa.Command != "make" || pa.TimeoutSeconds != 30 {
		t.Fatalf("pending approval: %+v", pa)
	}
	if pa.ExpiresAt.Before(before.Add(29*time.Second)) || pa.ExpiresAt.After(time.Now().Add(30*time.Second)) {
		t.Fatalf("expires_at %v, want about 30 s after the request", pa.ExpiresAt)
	}
	// The seconds left are counted on the Core's clock: the browser's clock
	// may be off, so the restored card counts down from this value.
	if pa.RemainingSeconds < 29 || pa.RemainingSeconds > 30 {
		t.Fatalf("remaining_seconds %d, want 29..30", pa.RemainingSeconds)
	}

	// The live event carries the same deadline, so the card's countdown
	// matches the Core's timeout instead of a fixed 60 s.
	bc.mu.Lock()
	var live event.AGUIPermissionRequestEvent
	for _, ev := range bc.events {
		if ev.EventType == event.AGUIPermissionRequest {
			live, _ = ev.Data.(event.AGUIPermissionRequestEvent)
		}
	}
	bc.mu.Unlock()
	if live.TimeoutSeconds != 30 || !live.ExpiresAt.Equal(pa.ExpiresAt) {
		t.Fatalf("broadcast permission request %+v, want the listed deadline %v", live, pa.ExpiresAt)
	}

	// Another tenant learns nothing about the conversation.
	if _, err := svc.ConversationRunState(tenantctx.WithTenant(context.Background(), "tenant-b"), "conv-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant: %v, want ErrNotFound", err)
	}
	// Only this conversation's approvals are listed.
	if other := svc.state.PendingApprovalRequestsOfRun("conv-2", "tenant-a"); len(other) != 0 {
		t.Fatalf("approvals of another run: %+v", other)
	}
	if other := svc.state.PendingApprovalRequestsOfRun("conv-1", "tenant-b"); len(other) != 0 {
		t.Fatalf("approvals of another tenant: %+v", other)
	}

	if !svc.ResolveApproval(ctxA, "conv-1", "call-1", "allow") {
		t.Fatal("ResolveApproval found nothing")
	}
	<-done
	svc.EndConversationRun("conv-1", "turn-1")
	ended, err := svc.ConversationRunState(ctxA, "conv-1")
	if err != nil {
		t.Fatal(err)
	}
	if ended.Active || ended.StreamedText != "" || len(ended.PendingApprovals) != 0 {
		t.Fatalf("after the turn: %+v", ended)
	}

	// The next turn starts with nothing streamed.
	if err := svc.BeginConversationRun(ctxA, "conv-1", "turn-2"); err != nil {
		t.Fatal(err)
	}
	if next, _ := svc.ConversationRunState(ctxA, "conv-1"); next.StreamedText != "" || next.TurnID != "turn-2" {
		t.Fatalf("next turn: %+v", next)
	}
}

// The stored active turn counts when this process did not dispatch it (a
// restarted Core): the chat still shows the turn as running.
func TestConversationRunState_StoredActiveTurn(t *testing.T) {
	svc, _ := newHITLTestService(30)
	svc.store = &runStateStore{conv: conversation.Conversation{ID: "conv-1", TenantID: "tenant-a", ActiveTurnID: "turn-9"}}
	got, err := svc.ConversationRunState(tenantctx.WithTenant(context.Background(), "tenant-a"), "conv-1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Active || got.TurnID != "turn-9" {
		t.Fatalf("stored active turn: %+v", got)
	}
}

// Output of a task run (no conversation run of that ID) is not kept, and the
// streamed text of a turn is bounded: it keeps the newest text, cut at a
// character boundary.
func TestConversationStream_OnlyActiveRunsAndBounded(t *testing.T) {
	m := NewRunStateManager()
	m.AppendConversationStream("task-1", "tenant-a", "not a conversation")
	if got := m.ConversationStream("task-1", ""); got != "" {
		t.Fatalf("task output kept: %q", got)
	}

	m.BeginConversationRun("conv-1", "turn-1", "tenant-a")
	chunk := strings.Repeat("ä", 1000) // 2 bytes per rune
	for range (maxConversationStreamBytes / len(chunk)) + 3 {
		m.AppendConversationStream("conv-1", "tenant-a", chunk)
	}
	m.AppendConversationStream("conv-1", "tenant-a", "END")
	m.AppendConversationStream("conv-1", "", "no tenant")
	m.AppendConversationStream("conv-1", "tenant-b", "other tenant")
	got := m.ConversationStream("conv-1", "turn-1")
	if len(got) > maxConversationStreamBytes || !utf8.ValidString(got) || !strings.HasSuffix(got, "END") {
		t.Fatalf("bounded stream: %d bytes, valid %v, suffix %q", len(got), utf8.ValidString(got), got[max(0, len(got)-3):])
	}
	if got := m.ConversationStream("conv-1", "turn-0"); got != "" {
		t.Fatalf("stream of another turn: %d bytes", len(got))
	}
}
