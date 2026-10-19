package service_test

import (
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/domain/trust"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// trustedReadProfile allows Read only for callers with at least partial
// trust; everything else falls to the plan-mode default (deny).
func trustedReadProfile() policy.PolicyProfile {
	return policy.PolicyProfile{
		Name: "trusted-read",
		Mode: policy.ModePlan,
		Rules: []policy.PermissionRule{
			{Specifier: policy.ToolSpecifier{Tool: "LLM"}, Decision: policy.DecisionAllow},
			{Specifier: policy.ToolSpecifier{Tool: "Read"}, Decision: policy.DecisionAllow, TrustMinimum: trust.LevelPartial},
		},
	}
}

var trustCases = []struct {
	name  string
	trust *trust.Annotation
	want  string
}{
	{"no annotation", nil, "deny"},
	{"untrusted", &trust.Annotation{TrustLevel: trust.LevelUntrusted}, "deny"},
	{"partial", &trust.Annotation{TrustLevel: trust.LevelPartial}, "allow"},
	{"full", &trust.Annotation{TrustLevel: trust.LevelFull}, "allow"},
}

// KI-7 review finding 10: the trust annotation of the request reaches the
// policy evaluation on the conversation path.
func TestConversationToolCall_TrustAnnotation(t *testing.T) {
	policySvc := service.NewPolicyService("headless-safe-sandbox", []policy.PolicyProfile{trustedReadProfile()})
	svc, queue := newConversationPolicyEnvWith(&project.Project{PolicyProfile: "trusted-read"}, "coder", policySvc)
	for _, tc := range trustCases {
		t.Run(tc.name, func(t *testing.T) {
			decision, reason := toolCallDecision(t, svc, queue, &messagequeue.ToolCallRequestPayload{
				RunID: "conv-pol", CallID: "c-" + tc.name, Tool: "read_file", Path: "a.go", Trust: tc.trust,
			})
			if decision != tc.want {
				t.Errorf("read_file with %s trust -> %s (%s), want %s", tc.name, decision, reason, tc.want)
			}
		})
	}
}

// The same on the run path.
func TestRunToolCall_TrustAnnotation(t *testing.T) {
	store := &runtimeMockStore{
		projects: []project.Project{{ID: "proj-1", Name: "p", WorkspacePath: "/tmp/test-workspace"}},
		agents:   []agent.Agent{{ID: "agent-1", ProjectID: "proj-1", Name: "a", Backend: "aider", Status: agent.StatusRunning, Config: map[string]string{}}},
		tasks:    []task.Task{{ID: "task-1", ProjectID: "proj-1", Title: "t", Prompt: "p", Status: task.StatusRunning}},
		runs: []run.Run{{ID: "run-trust", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
			PolicyProfile: "trusted-read", Status: run.StatusRunning, StartedAt: time.Now()}},
	}
	queue := &runtimeMockQueue{}
	policySvc := service.NewPolicyService("headless-safe-sandbox", []policy.PolicyProfile{trustedReadProfile()})
	svc := service.NewRuntimeService(store, queue, &runtimeMockBroadcaster{}, &runtimeMockEventStore{}, policySvc, &config.Runtime{})
	for _, tc := range trustCases {
		t.Run(tc.name, func(t *testing.T) {
			decision, reason := toolCallDecision(t, svc, queue, &messagequeue.ToolCallRequestPayload{
				RunID: "run-trust", CallID: "c-" + tc.name, Tool: "read_file", Path: "a.go", Trust: tc.trust,
			})
			if decision != tc.want {
				t.Errorf("read_file with %s trust -> %s (%s), want %s", tc.name, decision, reason, tc.want)
			}
		})
	}
}
