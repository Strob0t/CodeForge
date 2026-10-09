package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/feedback"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// recordingProvider is a feedback provider that answers allow and records
// the request it got.
type recordingProvider struct {
	mu   sync.Mutex
	reqs []feedback.FeedbackRequest
}

func (p *recordingProvider) Name() string { return "recording" }

//nolint:gocritic // hugeParam: signature of the feedback.Provider port
func (p *recordingProvider) RequestFeedback(_ context.Context, req feedback.FeedbackRequest) (feedback.FeedbackResult, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()
	return feedback.FeedbackResult{Decision: feedback.DecisionAllow, Provider: "recording"}, nil
}

// KI-69 (c): Slack and email approvers see what the web approver sees: the
// deciding profile and the arguments preview.
func TestApproval_FeedbackProvidersGetProfileAndArgumentsPreview(t *testing.T) {
	svc, store, queue, _ := newRuntimeTestEnvWithPolicy(service.NewPolicyService("supervised-ask-all", nil))
	provider := &recordingProvider{}
	svc.RegisterFeedbackProvider(provider)
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{ID: "run-fb", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "supervised-ask-all", Status: run.StatusRunning, StartedAt: time.Now()})
	store.mu.Unlock()

	decision, reason := toolCallDecision(t, svc, queue, &messagequeue.ToolCallRequestPayload{
		RunID: "run-fb", CallID: "c-fb", Tool: "write_file", Path: "notes.md",
		ArgumentsPreview: `{"content": "hello", "file_path": "notes.md"}`,
	})
	if decision != "allow" {
		t.Fatalf("decision = %s (%s), want allow from the provider", decision, reason)
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.reqs) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(provider.reqs))
	}
	got := provider.reqs[0]
	if got.Profile != "supervised-ask-all" {
		t.Errorf("profile = %q, want supervised-ask-all", got.Profile)
	}
	if got.ArgumentsPreview != `{"content": "hello", "file_path": "notes.md"}` {
		t.Errorf("arguments preview = %q", got.ArgumentsPreview)
	}
	if got.Tool != "write_file" || got.Path != "notes.md" || got.CallID != "c-fb" {
		t.Errorf("request = %+v", got)
	}
}

// S3-F review C5: feedback providers learn the tenant of the run asking, so
// the operator's email provider can answer only its own tenant.
func TestApproval_FeedbackRequestCarriesTheRunsTenant(t *testing.T) {
	const tenant = "22222222-2222-2222-2222-222222222222"
	svc, store, queue, _ := newRuntimeTestEnvWithPolicy(service.NewPolicyService("supervised-ask-all", nil))
	provider := &recordingProvider{}
	svc.RegisterFeedbackProvider(provider)
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{ID: "run-tn", TenantID: tenant, TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "supervised-ask-all", Status: run.StatusRunning, StartedAt: time.Now()})
	store.mu.Unlock()

	if decision, reason := toolCallDecision(t, svc, queue, &messagequeue.ToolCallRequestPayload{
		RunID: "run-tn", CallID: "c-tn", Tool: "write_file", Path: "notes.md", TenantID: tenant,
	}); decision != "allow" {
		t.Fatalf("decision = %s (%s), want allow from the provider", decision, reason)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.reqs) != 1 || provider.reqs[0].TenantID != tenant {
		t.Fatalf("provider requests = %+v, want one with tenant %s", provider.reqs, tenant)
	}
}
