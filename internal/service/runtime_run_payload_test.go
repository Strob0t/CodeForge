package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// startRunPayload starts a run in the default test environment and returns
// the runs.start payload the worker receives.
func startRunPayload(t *testing.T) messagequeue.RunStartPayload {
	t.Helper()
	svc, _, queue, _ := newRuntimeTestEnv()
	if _, err := svc.StartRun(context.Background(), &run.StartRequest{
		TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
	}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	msg, ok := queue.lastMessage(messagequeue.SubjectRunStart)
	if !ok {
		t.Fatal("no runs.start message published")
	}
	var payload messagequeue.RunStartPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("unmarshal runs.start: %v", err)
	}
	return payload
}

// TestStartRun_PayloadCarriesWorkspaceAndBackend: the worker runs the tools in
// the project workspace, so the run start names it (KI-23); before, the worker
// had no workspace for runs at all.
func TestStartRun_PayloadCarriesWorkspaceAndBackend(t *testing.T) {
	payload := startRunPayload(t)

	if payload.WorkspacePath != "/tmp/test-workspace" {
		t.Errorf("workspace_path = %q, want the project workspace %q", payload.WorkspacePath, "/tmp/test-workspace")
	}
	if payload.Backend != "aider" {
		t.Errorf("backend = %q, want the agent's backend %q", payload.Backend, "aider")
	}
}

// TestRunStart_CarriesTheApprovalTimeout: the worker waits for the policy
// decision at least as long as the Go Core waits for a HITL approval; before,
// it gave up after a fixed 30 s while Go waited 60 s, so later approvals were
// lost (KI-21).
func TestRunStart_CarriesTheApprovalTimeout(t *testing.T) {
	tests := []struct {
		name       string
		configured int
		want       int
	}{
		{name: "default", configured: 0, want: 60},
		{name: "configured", configured: 300, want: 300},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &runtimeMockStore{
				projects: []project.Project{{ID: "proj-1", WorkspacePath: "/tmp/ws"}},
				agents:   []agent.Agent{{ID: "agent-1", ProjectID: "proj-1", Backend: "aider", Status: agent.StatusIdle}},
				tasks:    []task.Task{{ID: "task-1", ProjectID: "proj-1", Prompt: "p", Status: task.StatusPending}},
			}
			queue := &runtimeMockQueue{}
			svc := service.NewRuntimeService(store, queue, &runtimeMockBroadcaster{}, &runtimeMockEventStore{},
				service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{ApprovalTimeoutSeconds: tt.configured})
			if _, err := svc.StartRun(context.Background(), &run.StartRequest{
				TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
			}); err != nil {
				t.Fatalf("StartRun: %v", err)
			}
			msg, ok := queue.lastMessage(messagequeue.SubjectRunStart)
			if !ok {
				t.Fatal("no runs.start message published")
			}
			var payload messagequeue.RunStartPayload
			if err := json.Unmarshal(msg.Data, &payload); err != nil {
				t.Fatalf("unmarshal runs.start: %v", err)
			}
			if payload.ApprovalTimeoutSeconds != tt.want {
				t.Fatalf("approval_timeout_seconds = %d, want %d", payload.ApprovalTimeoutSeconds, tt.want)
			}
		})
	}
}

// TestConversationRunStart_CarriesTheApprovalTimeout: agentic conversation
// runs ask for approvals through the same Go wait (KI-21).
func TestConversationRunStart_CarriesTheApprovalTimeout(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Runtime
		want int
	}{
		{name: "no runtime config", cfg: nil, want: 60},
		{name: "configured", cfg: &config.Runtime{ApprovalTimeoutSeconds: 90}, want: 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &convMockStore{}
			store.projects = []project.Project{{ID: "proj-1", Name: "p", WorkspacePath: t.TempDir()}}
			queue := &runtimeMockQueue{}
			conv := service.NewConversationService(store, &runtimeMockBroadcaster{}, "gpt-4o", service.NewModeService())
			conv.SetQueue(queue)
			conv.SetAgentConfig(&config.Agent{MaxLoopIterations: 10})
			if tt.cfg != nil {
				conv.SetRuntimeConfig(tt.cfg)
			}
			ctx := context.Background()
			c, err := conv.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "t"})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if err := conv.SendMessageAgentic(ctx, c.ID, &conversation.SendMessageRequest{Content: "do it"}); err != nil {
				t.Fatalf("SendMessageAgentic: %v", err)
			}
			msg, ok := queue.lastMessage(messagequeue.SubjectConversationRunStart)
			if !ok {
				t.Fatal("no conversation.run.start published")
			}
			var payload messagequeue.ConversationRunStartPayload
			if err := json.Unmarshal(msg.Data, &payload); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if payload.ApprovalTimeoutSeconds != tt.want {
				t.Fatalf("approval_timeout_seconds = %d, want %d", payload.ApprovalTimeoutSeconds, tt.want)
			}
		})
	}
}
