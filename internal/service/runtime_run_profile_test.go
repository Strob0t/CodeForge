package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// KI-69 (b): a run takes the policy profile of its request, else the one its
// project selects (policy_profile, then config["policy_preset"], as for
// conversations), else the service default. Runs ignored the project's
// profile before.
func TestStartRun_PolicyProfileResolution(t *testing.T) {
	tests := []struct {
		name           string
		projectProfile string
		projectPreset  string
		request        string
		want           string
	}{
		{"service default", "", "", "", "headless-safe-sandbox"},
		{"project policy_profile", "plan-readonly", "", "", "plan-readonly"},
		{"project config policy_preset", "", "trusted-mount-autonomous", "", "trusted-mount-autonomous"},
		{"policy_profile before policy_preset", "plan-readonly", "trusted-mount-autonomous", "", "plan-readonly"},
		{"request before project", "plan-readonly", "", "supervised-ask-all", "supervised-ask-all"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, _, _ := newRuntimeTestEnv()
			store.mu.Lock()
			store.projects[0].PolicyProfile = tt.projectProfile
			if tt.projectPreset != "" {
				store.projects[0].Config = map[string]string{"policy_preset": tt.projectPreset}
			}
			store.mu.Unlock()

			r, err := svc.StartRun(context.Background(), &run.StartRequest{
				TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1", PolicyProfile: tt.request,
			})
			if err != nil {
				t.Fatalf("StartRun: %v", err)
			}
			if r.PolicyProfile != tt.want {
				t.Fatalf("run policy profile = %q, want %q", r.PolicyProfile, tt.want)
			}
		})
	}
}
