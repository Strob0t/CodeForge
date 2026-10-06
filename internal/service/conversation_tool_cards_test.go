package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-129, KI-161: a conversation turn showed its tool cards only when it
// ended. The Core broadcasts agui.tool_call when it decides a call of the
// turn and agui.tool_result when the worker reports the call, as for runs,
// in the conversation's tenant.

func newToolCardsEnv(projectProfile string) (*service.RuntimeService, *runtimeMockBroadcaster) {
	store := &extRuntimeMockStore{
		runtimeMockStore: runtimeMockStore{projects: []project.Project{
			{ID: "proj-tc", Name: "tc", WorkspacePath: "/tmp/tc", PolicyProfile: projectProfile, TenantID: "tenant-a"},
		}},
		conversations: []conversation.Conversation{
			{ID: "conv-tc", ProjectID: "proj-tc", Title: "tool cards", TenantID: "tenant-a", ActiveTurnID: "turn-2"},
		},
	}
	hub := &runtimeMockBroadcaster{}
	svc := service.NewRuntimeService(store, &runtimeMockQueue{}, hub, &runtimeMockEventStore{},
		service.NewPolicyService(projectProfile, nil), &config.Runtime{})
	return svc, hub
}

func aguiToolCalls(hub *runtimeMockBroadcaster) []broadcastedEvent {
	var out []broadcastedEvent
	for _, ev := range hub.snapshot() {
		if ev.EventType == event.AGUIToolCall {
			out = append(out, ev)
		}
	}
	return out
}

func TestConversationToolCall_BroadcastsAGUIToolCall(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		tool    string
		preview string
	}{
		{"allowed call", "headless-safe-sandbox", "Read", `{"path": "main.go"}`},
		{"denied call", "plan-readonly", "Edit", `{"path": "main.go", "old": "a", "new": "b"}`},
		{"no arguments", "headless-safe-sandbox", "Read", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, hub := newToolCardsEnv(tt.profile)
			req := messagequeue.ToolCallRequestPayload{
				RunID: "conv-tc", CallID: "call-1", Tool: tt.tool, Path: "main.go", ArgumentsPreview: tt.preview,
			}
			if err := svc.HandleToolCallRequest(context.Background(), &req); err != nil {
				t.Fatalf("HandleToolCallRequest: %v", err)
			}
			calls := aguiToolCalls(hub)
			if len(calls) != 1 {
				t.Fatalf("got %d agui.tool_call events, want 1", len(calls))
			}
			if calls[0].Tenant != "tenant-a" {
				t.Errorf("broadcast in tenant %q, want tenant-a", calls[0].Tenant)
			}
			got, ok := calls[0].Data.(event.AGUIToolCallEvent)
			if !ok {
				t.Fatalf("payload is %T", calls[0].Data)
			}
			want := event.AGUIToolCallEvent{RunID: "conv-tc", CallID: "call-1", Name: tt.tool, Args: tt.preview}
			if got != want {
				t.Errorf("event = %+v, want %+v", got, want)
			}
		})
	}
}

// A call of another (stopped) turn is denied before any card is shown.
func TestConversationToolCall_NoCardForACallOfAnEndedTurn(t *testing.T) {
	svc, hub := newToolCardsEnv("headless-safe-sandbox")
	req := messagequeue.ToolCallRequestPayload{RunID: "conv-tc", CallID: "call-old", Tool: "Read", Path: "main.go", TurnID: "turn-1"}
	if err := svc.HandleToolCallRequest(context.Background(), &req); err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}
	if calls := aguiToolCalls(hub); len(calls) != 0 {
		t.Fatalf("got %d agui.tool_call events for a call of an ended turn, want 0", len(calls))
	}
}

func TestHandleToolCallResult_ConversationBroadcastsAGUIToolResult(t *testing.T) {
	tests := []struct {
		name      string
		result    messagequeue.ToolCallResultPayload
		wantEvent event.AGUIToolResultEvent
	}{
		{
			name:      "success",
			result:    messagequeue.ToolCallResultPayload{Success: true, Output: "package main", Diff: []byte(`{"path":"main.go"}`)},
			wantEvent: event.AGUIToolResultEvent{Result: "package main", Diff: []byte(`{"path":"main.go"}`)},
		},
		{
			// The worker reports a denied call with its reason as error.
			name:      "denied",
			result:    messagequeue.ToolCallResultPayload{Success: false, Error: "Permission denied: not allowed"},
			wantEvent: event.AGUIToolResultEvent{Error: "Permission denied: not allowed"},
		},
		{
			name:      "failed with output only",
			result:    messagequeue.ToolCallResultPayload{Success: false, Output: "command not found"},
			wantEvent: event.AGUIToolResultEvent{Result: "command not found", Error: "command not found"},
		},
		{
			name:      "failed without any text",
			result:    messagequeue.ToolCallResultPayload{Success: false},
			wantEvent: event.AGUIToolResultEvent{Error: "tool call failed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, hub := newToolCardsEnv("headless-safe-sandbox")
			res := tt.result
			res.RunID, res.CallID, res.Tool, res.TenantID = "conv-tc", "call-1", "Read", "tenant-a"
			if err := svc.HandleToolCallResult(context.Background(), &res); err != nil {
				t.Fatalf("HandleToolCallResult: %v", err)
			}
			var results []broadcastedEvent
			for _, ev := range hub.snapshot() {
				if ev.EventType == event.AGUIToolResult {
					results = append(results, ev)
				}
			}
			if len(results) != 1 {
				t.Fatalf("got %d agui.tool_result events, want 1", len(results))
			}
			if results[0].Tenant != "tenant-a" {
				t.Errorf("broadcast in tenant %q, want tenant-a", results[0].Tenant)
			}
			got, ok := results[0].Data.(event.AGUIToolResultEvent)
			if !ok {
				t.Fatalf("payload is %T", results[0].Data)
			}
			want := tt.wantEvent
			want.RunID, want.CallID = "conv-tc", "call-1"
			if got.RunID != want.RunID || got.CallID != want.CallID || got.Result != want.Result ||
				got.Error != want.Error || string(got.Diff) != string(want.Diff) {
				t.Errorf("event = %+v, want %+v", got, want)
			}
		})
	}
}

// Without a tenant the result names no audience: nothing is broadcast.
func TestHandleToolCallResult_ConversationWithoutTenantIsNotBroadcast(t *testing.T) {
	svc, hub := newToolCardsEnv("headless-safe-sandbox")
	res := messagequeue.ToolCallResultPayload{RunID: "conv-tc", CallID: "call-1", Tool: "Read", Success: true, Output: "x"}
	if err := svc.HandleToolCallResult(context.Background(), &res); err != nil {
		t.Fatalf("HandleToolCallResult: %v", err)
	}
	for _, ev := range hub.snapshot() {
		if ev.EventType == event.AGUIToolResult {
			t.Fatalf("broadcast %+v without a tenant", ev)
		}
	}
}
