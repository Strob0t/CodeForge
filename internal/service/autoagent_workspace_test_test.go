package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-81: the auto-agent's post-verification ran `python -m pytest` in the Go
// Core, in the agent-written workspace and with the Go Core's secrets. The
// Go Core must not execute workspace code: the test runs in the worker
// (conversation.test.request / conversation.test.result).

// workspaceTestQueue records requests and answers them with reply (nil: no
// answer, the request times out).
type workspaceTestQueue struct {
	noopQueue
	svc   *AutoAgentService
	reply func(req *messagequeue.WorkspaceTestRequestPayload) *messagequeue.WorkspaceTestResultPayload

	mu       sync.Mutex
	requests []messagequeue.WorkspaceTestRequestPayload
}

func (q *workspaceTestQueue) Publish(ctx context.Context, subject string, data []byte) error {
	if subject != messagequeue.SubjectConversationTestRequest {
		return nil
	}
	var req messagequeue.WorkspaceTestRequestPayload
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}
	q.mu.Lock()
	q.requests = append(q.requests, req)
	q.mu.Unlock()
	if q.reply != nil {
		if res := q.reply(&req); res != nil {
			go func() {
				// Delivered at least once.
				_ = q.svc.HandleWorkspaceTestResult(context.WithoutCancel(ctx), res)
				_ = q.svc.HandleWorkspaceTestResult(context.WithoutCancel(ctx), res)
			}()
		}
	}
	return nil
}

func newWorkspaceTestEnv(t *testing.T, reply func(*messagequeue.WorkspaceTestRequestPayload) *messagequeue.WorkspaceTestResultPayload) (*AutoAgentService, *workspaceTestQueue, string) {
	t.Helper()
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "test_feature.py"), []byte("def test_x(): pass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newAutoAgentMockStore()
	store.projects["proj-1"] = &project.Project{ID: "proj-1", WorkspacePath: ws}
	q := &workspaceTestQueue{reply: reply}
	svc := NewAutoAgentService(store, &noopBroadcaster{}, q, nil)
	q.svc = svc
	return svc, q, ws
}

func passedPtr(b bool) *bool { return &b }

func TestRunWorkspaceTest_RunsInTheWorker(t *testing.T) {
	tests := []struct {
		name       string
		result     messagequeue.WorkspaceTestResultPayload
		want       testResult
		wantErrSub string
	}{
		{name: "passed", result: messagequeue.WorkspaceTestResultPayload{Passed: passedPtr(true), Output: "=== 3 passed in 0.1s ==="},
			want: testResult{Passed: 3, Total: 3, AllPassed: true}},
		{name: "failed", result: messagequeue.WorkspaceTestResultPayload{Passed: passedPtr(false), Output: "=== 1 failed, 2 passed ==="},
			want: testResult{Passed: 2, Failed: 1, Total: 3}},
		{name: "exit status fails although nothing failed", result: messagequeue.WorkspaceTestResultPayload{Passed: passedPtr(false), Output: "=== 2 passed, 1 error ==="},
			want: testResult{Passed: 2, Total: 2}},
		{name: "could not run", result: messagequeue.WorkspaceTestResultPayload{Error: "command timed out after 300s", Output: "command timed out after 300s"},
			wantErrSub: "timed out"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, q, ws := newWorkspaceTestEnv(t, func(req *messagequeue.WorkspaceTestRequestPayload) *messagequeue.WorkspaceTestResultPayload {
				res := tc.result
				res.RequestID, res.TenantID = req.RequestID, req.TenantID
				return &res
			})
			ctx := tenantctx.WithTenant(context.Background(), "tenant-1")

			got, err := svc.runWorkspaceTest(ctx, "proj-1", "conv-1", "test_feature.py")

			if tc.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("runWorkspaceTest = %+v, %v; want an error containing %q", got, err, tc.wantErrSub)
				}
			} else {
				if err != nil {
					t.Fatalf("runWorkspaceTest: %v", err)
				}
				got.Output = ""
				if got != tc.want {
					t.Fatalf("result = %+v, want %+v", got, tc.want)
				}
			}
			q.mu.Lock()
			defer q.mu.Unlock()
			if len(q.requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(q.requests))
			}
			req := q.requests[0]
			if req.RequestID == "" || req.TenantID != "tenant-1" || req.ProjectID != "proj-1" || req.ConversationID != "conv-1" ||
				req.WorkspacePath != ws || req.TestFile != "test_feature.py" || req.TimeoutSeconds <= 0 {
				t.Fatalf("request = %+v", req)
			}
		})
	}
}

func TestRunWorkspaceTest_NoResultTimesOut(t *testing.T) {
	svc, _, _ := newWorkspaceTestEnv(t, nil)
	svc.testWaitMargin = 10 * time.Millisecond
	svc.testTimeout = 10 * time.Millisecond

	_, err := svc.runWorkspaceTest(context.Background(), "proj-1", "conv-1", "test_feature.py")
	if err == nil || !strings.Contains(err.Error(), "no result") {
		t.Fatalf("runWorkspaceTest without a result = %v, want a timeout error", err)
	}
	svc.testMu.Lock()
	defer svc.testMu.Unlock()
	if len(svc.testWaiters) != 0 {
		t.Fatalf("waiters left behind: %d", len(svc.testWaiters))
	}
}

func TestRunWorkspaceTest_RefusesPathsOutsideTheWorkspace(t *testing.T) {
	svc, q, _ := newWorkspaceTestEnv(t, nil)
	for _, file := range []string{"../test_x.py", "test_missing.py", "/etc/test_x.py"} {
		if _, err := svc.runWorkspaceTest(context.Background(), "proj-1", "conv-1", file); err == nil {
			t.Errorf("runWorkspaceTest(%q) succeeded", file)
		}
	}
	if len(q.requests) != 0 {
		t.Fatalf("requests published for refused paths: %v", q.requests)
	}
}

func TestHandleWorkspaceTestResult_UnknownRequestIsIgnored(t *testing.T) {
	svc, _, _ := newWorkspaceTestEnv(t, nil)
	if err := svc.HandleWorkspaceTestResult(context.Background(), &messagequeue.WorkspaceTestResultPayload{RequestID: "unknown"}); err != nil {
		t.Fatalf("HandleWorkspaceTestResult: %v", err)
	}
}

// TestGoCoreServicesDoNotExecute: the services run no programs themselves;
// workspace code runs in the worker (KI-81). The sandbox's docker CLI is the
// one exception (it starts containers, it runs nothing from the workspace in
// the Go Core).
func TestGoCoreServicesDoNotExecute(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	execCall := regexp.MustCompile(`exec\.Command(Context)?\(|os\.StartProcess\(|syscall\.Exec\(`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "sandbox.go" {
			continue
		}
		src, err := os.ReadFile(f) //nolint:gosec // package source file
		if err != nil {
			t.Fatal(err)
		}
		if loc := execCall.FindIndex(src); loc != nil {
			t.Errorf("%s runs a program in the Go Core: %s", f, src[loc[0]:loc[1]])
		}
	}
}
