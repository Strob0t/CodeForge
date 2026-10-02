package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-96: every payload that starts tool processes carries the tenant's tool
// UID (tool_uid) with workspace.tool_acls: required, computed in Go from the
// payload's tenant, and omits it with off.

const toolUIDTenant = "cccccccc-0000-0000-0000-000000000003"

// toolUIDStoreFake allocates UIDs from 20000 per tenant, or fails with err.
type toolUIDStoreFake struct {
	byTenant map[string]int
	err      error
}

func (f *toolUIDStoreFake) AllocateToolUID(_ context.Context, tenantID string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	if f.byTenant == nil {
		f.byTenant = map[string]int{}
	}
	if uid, ok := f.byTenant[tenantID]; ok {
		return uid, nil
	}
	f.byTenant[tenantID] = tenant.ToolUIDMin + len(f.byTenant)
	return f.byTenant[tenantID], nil
}

func (f *toolUIDStoreFake) AdvanceToolUIDSequence(context.Context, int) (bool, error) {
	return false, nil
}

// toolUIDOfJSON returns the tool_uid of a payload and whether it is present.
func toolUIDOfJSON(t *testing.T, data []byte) (int, bool) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	raw, ok := fields["tool_uid"]
	if !ok {
		return 0, false
	}
	var uid int
	if err := json.Unmarshal(raw, &uid); err != nil {
		t.Fatalf("tool_uid %s: %v", raw, err)
	}
	return uid, true
}

func toolUIDModes() []struct {
	name     string
	required bool
} {
	return []struct {
		name     string
		required bool
	}{{"required", true}, {"off", false}}
}

func TestRunStart_CarriesTheToolUID(t *testing.T) {
	for _, mode := range toolUIDModes() {
		t.Run(mode.name, func(t *testing.T) {
			svc, _, queue, _ := newRuntimeTestEnv()
			svc.SetToolUIDs(service.NewToolUIDService(&toolUIDStoreFake{}, mode.required))
			ctx := tenantctx.WithTenant(context.Background(), toolUIDTenant)
			if _, err := svc.StartRun(ctx, &run.StartRequest{TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1"}); err != nil {
				t.Fatalf("StartRun: %v", err)
			}
			msg, ok := queue.lastMessage(messagequeue.SubjectRunStart)
			if !ok {
				t.Fatal("no runs.start published")
			}
			uid, present := toolUIDOfJSON(t, msg.Data)
			if present != mode.required || (mode.required && uid != tenant.ToolUIDMin) {
				t.Fatalf("tool_uid = %d (present %v), want 20000 only when required", uid, present)
			}
		})
	}
}

// TestRunStart_ExhaustedToolUIDsCreateNoRun: a full UID range refuses the
// run before it exists (the API answers 503).
func TestRunStart_ExhaustedToolUIDsCreateNoRun(t *testing.T) {
	svc, store, queue, _ := newRuntimeTestEnv()
	svc.SetToolUIDs(service.NewToolUIDService(&toolUIDStoreFake{err: tenant.ErrToolUIDRangeExhausted}, true))
	ctx := tenantctx.WithTenant(context.Background(), toolUIDTenant)
	_, err := svc.StartRun(ctx, &run.StartRequest{TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1"})
	if !errors.Is(err, tenant.ErrToolUIDRangeExhausted) {
		t.Fatalf("StartRun = %v, want ErrToolUIDRangeExhausted", err)
	}
	if _, ok := queue.lastMessage(messagequeue.SubjectRunStart); ok {
		t.Fatal("a run start was published")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.runs) != 0 {
		t.Fatalf("runs created: %+v", store.runs)
	}
}

func TestConversationRunStart_CarriesTheToolUID(t *testing.T) {
	for _, starter := range conversationRunStarters {
		for _, mode := range toolUIDModes() {
			t.Run(starter.name+"/"+mode.name, func(t *testing.T) {
				env := newConvStopEnv(t, nil, nil)
				env.conv.SetToolUIDs(service.NewToolUIDService(&toolUIDStoreFake{}, mode.required))
				ctx := tenantctx.WithTenant(context.Background(), toolUIDTenant)
				if err := starter.start(ctx, env.conv, env.convID); err != nil {
					t.Fatalf("start: %v", err)
				}
				msg, ok := env.starts.lastMessage(messagequeue.SubjectConversationRunStart)
				if !ok {
					t.Fatal("no conversation.run.start published")
				}
				uid, present := toolUIDOfJSON(t, msg.Data)
				if present != mode.required || (mode.required && uid != tenant.ToolUIDMin) {
					t.Fatalf("tool_uid = %d (present %v), want 20000 only when required", uid, present)
				}
			})
		}
	}
}

func TestQualityGateRequest_CarriesTheToolUID(t *testing.T) {
	for _, mode := range toolUIDModes() {
		t.Run(mode.name, func(t *testing.T) {
			env := newGateCommandEnv(t, []string{"go.mod"}, nil, project.GateCommands{Test: "go test ./...", Lint: "go vet ./..."})
			env.svc.SetToolUIDs(service.NewToolUIDService(&toolUIDStoreFake{}, mode.required))
			env.addRun("run-uid", "headless-safe-sandbox", run.StatusRunning, run.DeliverModeNone)
			completeRun(t, env, "run-uid")
			msg, ok := env.queue.lastMessage(messagequeue.SubjectQualityGateRequest)
			if !ok {
				t.Fatal("no quality gate request")
			}
			uid, present := toolUIDOfJSON(t, msg.Data)
			if present != mode.required || (mode.required && uid < tenant.ToolUIDMin) {
				t.Fatalf("tool_uid = %d (present %v)", uid, present)
			}
		})
	}
}

func TestBenchmarkRunRequest_CarriesTheToolUID(t *testing.T) {
	for _, mode := range toolUIDModes() {
		t.Run(mode.name, func(t *testing.T) {
			store := newBenchMockStore()
			suiteSvc := service.NewBenchmarkSuiteService(store, "/tmp")
			runMgr := service.NewBenchmarkRunManager(store, suiteSvc)
			runMgr.SetToolUIDs(service.NewToolUIDService(&toolUIDStoreFake{}, mode.required))
			q := &benchMockQueue{}
			runMgr.SetQueue(q)
			store.suites["suite-1"] = &benchmark.Suite{ID: "suite-1", Name: "s", Type: benchmark.TypeSimple, ProviderName: "p"}
			ctx := tenantctx.WithTenant(context.Background(), toolUIDTenant)
			if _, err := runMgr.StartRun(ctx, &benchmark.CreateRunRequest{Dataset: "d", Model: "m", Metrics: []string{"correctness"}, SuiteID: "suite-1"}); err != nil {
				t.Fatalf("StartRun: %v", err)
			}
			if len(q.published) != 1 || q.published[0].Subject != messagequeue.SubjectBenchmarkRunRequest {
				t.Fatalf("published = %+v", q.published)
			}
			uid, present := toolUIDOfJSON(t, q.published[0].Data)
			if present != mode.required || (mode.required && uid != tenant.ToolUIDMin) {
				t.Fatalf("tool_uid = %d (present %v), want 20000 only when required", uid, present)
			}
		})
	}
}
