package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-129, KI-161: a conversation turn showed its tool cards only when it
// ended. The Core broadcasts agui.tool_call when it decides a call of the
// turn and agui.tool_result when the worker reports the call, as for runs,
// in the conversation's tenant.

// tenantScopedConvStore finds a conversation only in its own tenant, as the
// Postgres store does.
type tenantScopedConvStore struct {
	*extRuntimeMockStore
}

func (s tenantScopedConvStore) GetConversation(ctx context.Context, id string) (*conversation.Conversation, error) {
	c, err := s.extRuntimeMockStore.GetConversation(ctx, id)
	if err != nil {
		return nil, err
	}
	if tenantID, _ := tenantctx.Lookup(ctx); tenantID != c.TenantID {
		return nil, errMockNotFound
	}
	return c, nil
}

func newToolCardsEnv(projectProfile string) (*service.RuntimeService, *runtimeMockBroadcaster) {
	svc, hub, _ := newToolCardsEnvWithStore(projectProfile)
	return svc, hub
}

func newToolCardsEnvWithStore(projectProfile string) (*service.RuntimeService, *runtimeMockBroadcaster, *extRuntimeMockStore) {
	store := &extRuntimeMockStore{
		runtimeMockStore: runtimeMockStore{projects: []project.Project{
			{ID: "proj-tc", Name: "tc", WorkspacePath: "/tmp/tc", PolicyProfile: projectProfile, TenantID: "tenant-a"},
		}},
		conversations: []conversation.Conversation{
			{ID: "conv-tc", ProjectID: "proj-tc", Title: "tool cards", TenantID: "tenant-a", ActiveTurnID: "turn-2"},
		},
	}
	hub := &runtimeMockBroadcaster{}
	svc := service.NewRuntimeService(tenantScopedConvStore{store}, &runtimeMockQueue{}, hub, &runtimeMockEventStore{},
		service.NewPolicyService(projectProfile, nil), &config.Runtime{})
	return svc, hub, store
}

func broadcastsOf(hub *runtimeMockBroadcaster, eventType string) []broadcastedEvent {
	var out []broadcastedEvent
	for _, ev := range hub.snapshot() {
		if ev.EventType == eventType {
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
				RunID: "conv-tc", CallID: "call-1", TenantID: "tenant-a", Tool: tt.tool, Path: "main.go",
				ArgumentsPreview: tt.preview, ReportsResult: true,
			}
			if err := svc.HandleToolCallRequest(context.Background(), &req); err != nil {
				t.Fatalf("HandleToolCallRequest: %v", err)
			}
			calls := broadcastsOf(hub, event.AGUIToolCall)
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

// No card for a call whose result never follows: the agent loop's "LLM"
// permission before each completion is no tool call, and a Claude Code turn
// cannot match the result of a call the CLI gave no tool_use_id (the card
// would stay running).
func TestConversationToolCall_NoCardWithoutAToolResult(t *testing.T) {
	tests := []struct {
		name string
		req  messagequeue.ToolCallRequestPayload
	}{
		{"LLM permission", messagequeue.ToolCallRequestPayload{Tool: "LLM", Command: "chat_completion", ReportsResult: true}},
		{"llm in lower case", messagequeue.ToolCallRequestPayload{Tool: "llm", ReportsResult: true}},
		{"result not reported", messagequeue.ToolCallRequestPayload{Tool: "Read", Path: "main.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, hub := newToolCardsEnv("headless-safe-sandbox")
			req := tt.req
			req.RunID, req.CallID, req.TenantID = "conv-tc", "call-1", "tenant-a"
			if err := svc.HandleToolCallRequest(context.Background(), &req); err != nil {
				t.Fatalf("HandleToolCallRequest: %v", err)
			}
			if calls := broadcastsOf(hub, event.AGUIToolCall); len(calls) != 0 {
				t.Fatalf("got %d agui.tool_call events, want 0", len(calls))
			}
		})
	}
}

// A call of another (stopped) turn is denied before any card is shown.
func TestConversationToolCall_NoCardForACallOfAnEndedTurn(t *testing.T) {
	svc, hub := newToolCardsEnv("headless-safe-sandbox")
	req := messagequeue.ToolCallRequestPayload{
		RunID: "conv-tc", CallID: "call-old", TenantID: "tenant-a", Tool: "Read", Path: "main.go", TurnID: "turn-1", ReportsResult: true,
	}
	if err := svc.HandleToolCallRequest(context.Background(), &req); err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}
	if calls := broadcastsOf(hub, event.AGUIToolCall); len(calls) != 0 {
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
			results := broadcastsOf(hub, event.AGUIToolResult)
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

// A result is shown only in the tenant that owns the conversation (defence
// in depth: the tenant comes from the worker), and the "LLM" permission's
// result is no card.
func TestHandleToolCallResult_ConversationResultWithoutACard(t *testing.T) {
	tests := []struct {
		name   string
		result messagequeue.ToolCallResultPayload
	}{
		{"no tenant", messagequeue.ToolCallResultPayload{RunID: "conv-tc", Tool: "Read"}},
		{"another tenant", messagequeue.ToolCallResultPayload{RunID: "conv-tc", Tool: "Read", TenantID: "tenant-b"}},
		{"unknown conversation", messagequeue.ToolCallResultPayload{RunID: "conv-gone", Tool: "Read", TenantID: "tenant-a"}},
		{"LLM permission", messagequeue.ToolCallResultPayload{RunID: "conv-tc", Tool: "LLM", TenantID: "tenant-a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, hub := newToolCardsEnv("headless-safe-sandbox")
			res := tt.result
			res.CallID, res.Success, res.Output = "call-1", true, "x"
			if err := svc.HandleToolCallResult(context.Background(), &res); err != nil {
				t.Fatalf("HandleToolCallResult: %v", err)
			}
			if results := broadcastsOf(hub, event.AGUIToolResult); len(results) != 0 {
				t.Fatalf("broadcast %+v, want none", results)
			}
		})
	}
}

// A write_file diff carries the whole old and new file; the broadcast keeps
// a few KB of it and says that it was cut, on the conversation and the run
// path.
func TestToolResult_BroadcastDiffIsCapped(t *testing.T) {
	big := strings.Repeat("SECRET=1\n", 120_000) // about 1 MB
	diff, err := json.Marshal(map[string]any{
		"path": ".env",
		"hunks": []map[string]any{
			{"old_start": 1, "old_lines": 120_001, "new_start": 1, "new_lines": 2, "old_content": big, "new_content": "A=1\n"},
			{"old_start": 9, "old_lines": 1, "new_start": 9, "new_lines": 1, "old_content": big, "new_content": big},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	broadcastDiff := func(t *testing.T, conversationPath bool) json.RawMessage {
		t.Helper()
		svc, hub, store := newToolCardsEnvWithStore("headless-safe-sandbox")
		runID := "conv-tc"
		if !conversationPath {
			runID = "run-diff"
			store.mu.Lock()
			store.runs = append(store.runs, run.Run{
				ID: runID, TaskID: "task-1", ProjectID: "proj-tc", TenantID: "tenant-a",
				PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning, StartedAt: time.Now(),
			})
			store.mu.Unlock()
		}
		res := messagequeue.ToolCallResultPayload{
			RunID: runID, CallID: "call-1", Tool: "write_file", TenantID: "tenant-a", Success: true, Output: "wrote", Diff: diff,
		}
		if err := svc.HandleToolCallResult(context.Background(), &res); err != nil {
			t.Fatalf("HandleToolCallResult: %v", err)
		}
		results := broadcastsOf(hub, event.AGUIToolResult)
		if len(results) != 1 {
			t.Fatalf("got %d agui.tool_result events, want 1", len(results))
		}
		return results[0].Data.(event.AGUIToolResultEvent).Diff
	}

	for _, path := range []struct {
		name         string
		conversation bool
	}{{"conversation", true}, {"run", false}} {
		t.Run(path.name, func(t *testing.T) {
			got := broadcastDiff(t, path.conversation)
			if len(got) > 16<<10 {
				t.Fatalf("broadcast diff is %d bytes, want at most 16 KiB", len(got))
			}
			var d struct {
				Path      string `json:"path"`
				Truncated bool   `json:"truncated"`
				Hunks     []struct {
					OldStart   int    `json:"old_start"`
					NewLines   int    `json:"new_lines"`
					OldContent string `json:"old_content"`
					NewContent string `json:"new_content"`
				} `json:"hunks"`
			}
			if err := json.Unmarshal(got, &d); err != nil {
				t.Fatalf("broadcast diff is no diff: %v", err)
			}
			if d.Path != ".env" || !d.Truncated || len(d.Hunks) == 0 {
				t.Fatalf("diff = path %q, truncated %t, %d hunks; want .env, truncated, hunks", d.Path, d.Truncated, len(d.Hunks))
			}
			if d.Hunks[0].OldStart != 1 || d.Hunks[0].NewLines != 2 || !strings.HasPrefix(big, d.Hunks[0].OldContent) {
				t.Errorf("first hunk = %+v, want its position and the start of its content", d.Hunks[0].OldStart)
			}
		})
	}
}

// A small diff is broadcast as the worker sent it.
func TestToolResult_SmallDiffIsBroadcastUnchanged(t *testing.T) {
	svc, hub := newToolCardsEnv("headless-safe-sandbox")
	diff := json.RawMessage(`{"path":"a.go","hunks":[{"old_start":1,"old_lines":1,"new_start":1,"new_lines":1,"old_content":"a","new_content":"b"}]}`)
	res := messagequeue.ToolCallResultPayload{
		RunID: "conv-tc", CallID: "call-1", Tool: "edit_file", TenantID: "tenant-a", Success: true, Diff: diff,
	}
	if err := svc.HandleToolCallResult(context.Background(), &res); err != nil {
		t.Fatalf("HandleToolCallResult: %v", err)
	}
	results := broadcastsOf(hub, event.AGUIToolResult)
	if len(results) != 1 || string(results[0].Data.(event.AGUIToolResultEvent).Diff) != string(diff) {
		t.Fatalf("broadcast %+v, want the diff unchanged", results)
	}
}

// The agent loop requests an "LLM" permission before each completion and
// reports its usage as that call's result; runs use the same loop, so the
// run path shows no card for it either (neither agui.tool_call nor
// agui.tool_result), while its native run.toolcall status events stay.
func TestRunToolCall_LLMPermissionGetsNoCard(t *testing.T) {
	svc, hub, store := newToolCardsEnvWithStore("headless-safe-sandbox")
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID: "run-llm", TaskID: "task-1", ProjectID: "proj-tc", TenantID: "tenant-a",
		PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning, StartedAt: time.Now(),
	})
	store.mu.Unlock()

	req := messagequeue.ToolCallRequestPayload{
		RunID: "run-llm", CallID: "call-llm", TenantID: "tenant-a", Tool: "LLM", Command: "chat_completion", ReportsResult: true,
	}
	if err := svc.HandleToolCallRequest(context.Background(), &req); err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}
	res := messagequeue.ToolCallResultPayload{
		RunID: "run-llm", CallID: "call-llm", TenantID: "tenant-a", Tool: "LLM", Success: true, Output: "(tool_calls)",
		TokensIn: 10, TokensOut: 5, CostUSD: 0.001,
	}
	if err := svc.HandleToolCallResult(context.Background(), &res); err != nil {
		t.Fatalf("HandleToolCallResult: %v", err)
	}

	if calls := broadcastsOf(hub, event.AGUIToolCall); len(calls) != 0 {
		t.Errorf("got %d agui.tool_call events for the LLM permission, want 0", len(calls))
	}
	if results := broadcastsOf(hub, event.AGUIToolResult); len(results) != 0 {
		t.Errorf("got %d agui.tool_result events for the LLM permission, want 0", len(results))
	}
	if statuses := broadcastsOf(hub, event.EventToolCallStatus); len(statuses) != 2 {
		t.Errorf("got %d run.toolcall status events, want 2 (decision and result)", len(statuses))
	}
}
