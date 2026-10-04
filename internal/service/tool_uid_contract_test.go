package service_test

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-96 / ADR-018 trust invariants: only the Go Core picks the identity a
// tool process runs as. It never republishes a worker-originated payload (a
// dead-letter copy the worker wrote) onto a subject that starts tool
// processes, and a handoff run's tool_uid comes from the claimed tenant.

// toolStartSubjects are the subjects whose payloads start tool processes.
var toolStartSubjects = []string{
	"SubjectRunStart",
	"SubjectConversationRunStart",
	"SubjectTaskAgent",
	"SubjectQualityGateRequest",
	"SubjectConversationTestRequest",
	"SubjectBenchmarkRunRequest",
	"SubjectWorkspaceDeleteRequest",
}

// handoffWorkspaceStore is the handoff store whose project has a workspace,
// so a real RuntimeService starts the handoff's run.
type handoffWorkspaceStore struct {
	*handoffStore
}

func (s *handoffWorkspaceStore) GetProject(context.Context, string) (*project.Project, error) {
	return &project.Project{ID: "proj-1", WorkspacePath: "/data/workspaces/" + handoffTenantA + "/proj-1"}, nil
}

func TestHandoffRequest_ToolUIDComesFromTheClaimedTenant(t *testing.T) {
	env := newHandoffEnv(t, false)
	store := &handoffWorkspaceStore{handoffStore: env.store}
	queue := &runtimeMockQueue{}
	runtimeSvc := service.NewRuntimeService(store, queue, &runtimeMockBroadcaster{}, &runtimeMockEventStore{},
		service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{})
	uids := &toolUIDStoreFake{byTenant: map[string]int{handoffTenantA: 20011, handoffTenantB: 20022}}
	runtimeSvc.SetToolUIDs(service.NewToolUIDService(uids, true))
	env.svc.SetRunStarter(runtimeSvc)

	data := handoffRequest(t, handoffTenantA, "run-src", "agent-tgt", "Please review", map[string]any{
		"tool_uid": 20022,
		"metadata": map[string]string{"tenant_id": handoffTenantB, "tool_uid": "20022", "handoff_hop": "0"},
	})
	if err := env.svc.HandleHandoffRequest(context.Background(), data); err != nil {
		t.Fatalf("HandleHandoffRequest: %v", err)
	}
	msg, ok := queue.lastMessage(messagequeue.SubjectRunStart)
	if !ok {
		t.Fatal("no runs.start published for the handoff")
	}
	var start messagequeue.RunStartPayload
	if err := json.Unmarshal(msg.Data, &start); err != nil {
		t.Fatal(err)
	}
	if start.TenantID != handoffTenantA || start.ToolUID != 20011 {
		t.Fatalf("handoff run start in tenant %q with tool_uid %d, want tenant A and its UID 20011", start.TenantID, start.ToolUID)
	}
}

// TestDeadLetterHandlersNeverRepublishToolStarts: the Core's .dlq handlers
// only end or record work. A dead-letter copy is written by the worker, so
// republishing it onto a start subject would let the worker choose a tool
// UID. Recorded with real handlers, then checked over the sources.
func TestDeadLetterHandlersNeverRepublishToolStarts(t *testing.T) {
	ctx := context.Background()
	forged := []byte(`{"run_id":"11111111-2222-3333-4444-555555555555","task_id":"task-1","project_id":"proj-1",` +
		`"tenant_id":"` + lostWorkerTenant + `","tool_uid":29999,"conversation_id":"conv-1"}`)

	svc, store, queue, _ := newRuntimeTestEnv()
	setStoredRun(store, &run.Run{
		ID: "11111111-2222-3333-4444-555555555555", TenantID: lostWorkerTenant, TaskID: "task-1", AgentID: "agent-1",
		ProjectID: "proj-1", PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning,
	})
	if err := svc.HandleDeadLetteredRunStart(ctx, forged); err != nil {
		t.Fatalf("runtime: %v", err)
	}
	env := newConvStopEnv(t, nil, nil)
	if err := env.conv.HandleDeadLetteredRunStart(ctx, messagequeue.SubjectConversationRunStart+".dlq", forged); err != nil {
		t.Fatalf("conversation: %v", err)
	}
	for _, q := range []*runtimeMockQueue{queue, env.starts, env.responses} {
		q.mu.Lock()
		for _, msg := range q.messages {
			if strings.Contains(string(msg.Data), "29999") {
				t.Errorf("a dead-letter copy was republished on %s: %s", msg.Subject, msg.Data)
			}
		}
		q.mu.Unlock()
	}

	// No function of the Core that handles dead letters names a start subject.
	scanned := 0
	for _, dir := range []string{".", "../adapter/nats", "../../cmd/codeforge"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil || !strings.Contains(strings.ToLower(fn.Name.Name), "deadletter") {
					continue
				}
				scanned++
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || !strings.HasPrefix(sel.Sel.Name, "Publish") {
						return true
					}
					for _, arg := range call.Args {
						if s, ok := arg.(*ast.SelectorExpr); ok {
							for _, subject := range toolStartSubjects {
								if s.Sel.Name == subject {
									t.Errorf("%s: %s publishes %s", path, fn.Name.Name, subject)
								}
							}
						}
					}
					return true
				})
			}
		}
	}
	if scanned < 5 {
		t.Fatalf("scanned %d dead-letter handlers, want the Core's (at least 5)", scanned)
	}
}
