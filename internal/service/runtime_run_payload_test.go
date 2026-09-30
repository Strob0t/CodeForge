package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
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
