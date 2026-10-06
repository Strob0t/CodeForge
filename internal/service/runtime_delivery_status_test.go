package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-188 (R2-5): a failed push or pull request was announced as a completed
// delivery ("completed (branch: codeforge/x, PR: )") although the branch
// existed only locally. A failed push is a failed delivery; a pushed branch
// without its pull request is a partial one. Both say why.

// fixedDeliverer returns one delivery result.
type fixedDeliverer struct{ result service.DeliveryResult }

func (d *fixedDeliverer) Deliver(_ context.Context, r *run.Run, _ string) (*service.DeliveryResult, error) {
	result := d.result
	result.Mode = r.DeliverMode
	if d.result.Mode != "" {
		result.Mode = d.result.Mode
	}
	return &result, nil
}

func TestTriggerDelivery_ReportsFailedPushesAndPullRequests(t *testing.T) {
	tests := []struct {
		name        string
		deliver     run.DeliverMode
		result      service.DeliveryResult
		wantStatus  string
		wantEvent   event.Type
		wantAudit   string
		wantInError string
	}{
		{
			name:       "pushed branch",
			deliver:    run.DeliverModeBranch,
			result:     service.DeliveryResult{BranchName: "codeforge/x", CommitHash: "abc"},
			wantStatus: "completed", wantEvent: event.TypeDeliveryCompleted, wantAudit: "delivery.completed",
		},
		{
			name:       "opened pull request",
			deliver:    run.DeliverModePR,
			result:     service.DeliveryResult{BranchName: "codeforge/x", CommitHash: "abc", PRURL: "https://github.com/a/b/pull/1"},
			wantStatus: "completed", wantEvent: event.TypeDeliveryCompleted, wantAudit: "delivery.completed",
		},
		{
			name:       "branch push failed",
			deliver:    run.DeliverModeBranch,
			result:     service.DeliveryResult{BranchName: "codeforge/x", CommitHash: "abc", PushError: "git push: rejected"},
			wantStatus: "failed", wantEvent: event.TypeDeliveryFailed, wantAudit: "delivery.failed",
			wantInError: "git push: rejected",
		},
		{
			name:       "PR delivery whose push failed",
			deliver:    run.DeliverModePR,
			result:     service.DeliveryResult{Mode: run.DeliverModeBranch, BranchName: "codeforge/x", CommitHash: "abc", PushError: "git push: rejected"},
			wantStatus: "failed", wantEvent: event.TypeDeliveryFailed, wantAudit: "delivery.failed",
			wantInError: "git push: rejected",
		},
		{
			name:       "pull request not opened",
			deliver:    run.DeliverModePR,
			result:     service.DeliveryResult{Mode: run.DeliverModeBranch, BranchName: "codeforge/x", CommitHash: "abc", PRError: "no GitHub token"},
			wantStatus: "partial", wantEvent: event.TypeDeliveryPartial, wantAudit: "delivery.partial",
			wantInError: "no GitHub token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, store, queue, bc := newRuntimeTestEnv()
			events := &recordingEventStore{}
			svc := service.NewRuntimeService(store, queue, bc, events, service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{
				StallThreshold: 5, DefaultTestCommand: "go test ./...", DefaultLintCommand: "golangci-lint run ./...",
			})
			svc.SetDeliverService(&fixedDeliverer{result: tt.result})
			svc.SetCheckpointService(&recordingCheckpointer{})
			setStoredRun(store, &run.Run{
				ID: "run-d", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
				PolicyProfile: "plan-readonly", Status: run.StatusRunning, DeliverMode: tt.deliver,
			})

			if err := svc.HandleRunComplete(context.Background(), &messagequeue.RunCompletePayload{RunID: "run-d", Status: "completed", Output: "done"}); err != nil {
				t.Fatalf("HandleRunComplete: %v", err)
			}

			var final *event.DeliveryEvent
			for _, ev := range bc.snapshot() {
				if d, ok := ev.Data.(event.DeliveryEvent); ok && ev.EventType == event.EventDelivery && d.Status != "started" {
					final = &d
				}
			}
			if final == nil {
				t.Fatal("no final delivery broadcast")
			}
			if final.Status != tt.wantStatus || final.BranchName != tt.result.BranchName || final.CommitHash != tt.result.CommitHash {
				t.Fatalf("broadcast %+v, want status %q with the branch and commit", *final, tt.wantStatus)
			}
			if !strings.Contains(final.Error, tt.wantInError) || (tt.wantInError == "") != (final.Error == "") {
				t.Fatalf("broadcast error %q, want it to name %q", final.Error, tt.wantInError)
			}

			var types []event.Type
			for _, ev := range events.events {
				if strings.HasPrefix(string(ev.Type), "run.delivery.") && ev.Type != event.TypeDeliveryStarted {
					types = append(types, ev.Type)
				}
			}
			if len(types) != 1 || types[0] != tt.wantEvent {
				t.Fatalf("run events %v, want one %s", types, tt.wantEvent)
			}
			var actions []string
			for _, a := range events.audits {
				if strings.HasPrefix(a.Action, "delivery.") {
					actions = append(actions, a.Action)
					if tt.wantInError != "" && !strings.Contains(a.Details, tt.wantInError) {
						t.Errorf("audit %s %q does not name %q", a.Action, a.Details, tt.wantInError)
					}
				}
			}
			if len(actions) != 1 || actions[0] != tt.wantAudit {
				t.Fatalf("audit actions %v, want one %s", actions, tt.wantAudit)
			}
		})
	}
}
